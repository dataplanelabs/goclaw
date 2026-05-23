package personal

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

const reactionCoalesceWindow = 30 * time.Second

type ReactionEvent = protocol.ReactionEvent

// reactionCoalescer dedupes rapid-fire reactions per (thread, user, msg).
// Latest-wins: Submit cancels the prior sleeper. Per-pending context
// cancellation (not time.AfterFunc) avoids deadlocks across Cancel/Flush.
type reactionCoalescer struct {
	mu      sync.Mutex
	pending map[string]*pendingReaction
	window  time.Duration
	emit    func(ReactionEvent)
}

type pendingReaction struct {
	event  ReactionEvent
	cancel context.CancelFunc
}

func newReactionCoalescer(window time.Duration, emit func(ReactionEvent)) *reactionCoalescer {
	return &reactionCoalescer{
		pending: make(map[string]*pendingReaction),
		window:  window,
		emit:    emit,
	}
}

// Submit registers an event, cancelling any prior sleeper for the same key.
func (rc *reactionCoalescer) Submit(ev ReactionEvent) {
	key := coalesceKey(ev)
	ctx, cancel := context.WithCancel(context.Background())
	pr := &pendingReaction{event: ev, cancel: cancel}

	rc.mu.Lock()
	if prev, ok := rc.pending[key]; ok {
		prev.cancel()
	}
	rc.pending[key] = pr
	rc.mu.Unlock()

	go rc.runSleeper(ctx, key, pr)
}

func (rc *reactionCoalescer) runSleeper(ctx context.Context, key string, pr *pendingReaction) {
	t := time.NewTimer(rc.window)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return
	case <-t.C:
		rc.mu.Lock()
		if cur, ok := rc.pending[key]; ok && cur == pr {
			delete(rc.pending, key)
			rc.mu.Unlock()
			rc.emit(pr.event)
			return
		}
		rc.mu.Unlock()
	}
}

// Flush emits all pending events immediately. Used in tests; production paths
// use Cancel to avoid emitting into a torn-down agent.
func (rc *reactionCoalescer) Flush() {
	rc.mu.Lock()
	items := make([]*pendingReaction, 0, len(rc.pending))
	for _, pr := range rc.pending {
		items = append(items, pr)
		pr.cancel()
	}
	rc.pending = make(map[string]*pendingReaction)
	rc.mu.Unlock()
	for _, pr := range items {
		rc.emit(pr.event)
	}
}

// Cancel drops every pending sleeper without emitting. Used on Channel.Stop.
func (rc *reactionCoalescer) Cancel() {
	rc.mu.Lock()
	for _, pr := range rc.pending {
		pr.cancel()
	}
	rc.pending = make(map[string]*pendingReaction)
	rc.mu.Unlock()
}

func (rc *reactionCoalescer) pendingCount() int {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return len(rc.pending)
}

func coalesceKey(ev ReactionEvent) string {
	return ev.ThreadID + "|" + ev.UIDFrom + "|" + ev.MsgID
}

func (c *Channel) onReactionEvent(ev ReactionEvent) {
	if ev.IsHistoric {
		return
	}
	if c.config.DisableReactions {
		return
	}
	if ev.IsSelf && !c.config.ListenSelfReactions {
		return
	}
	if c.reactionCoalescer == nil {
		c.emitCoalescedReaction(ev)
		return
	}
	c.reactionCoalescer.Submit(ev)
}

func (c *Channel) emitCoalescedReaction(ev ReactionEvent) {
	if !c.IsRunning() {
		return
	}

	metadata := map[string]string{
		"platform":          channels.TypeZaloPersonal,
		"zalo_event":        "reaction_added",
		"reaction_code":     ev.Code,
		"target_msg_id":     ev.MsgID,
		"target_cli_msg_id": ev.CliMsgID,
		"display_name":      channels.SanitizeDisplayName(ev.DName),
		"synthetic":         "true",
	}
	if ev.Code == protocol.ReactionNone {
		metadata["zalo_event"] = "reaction_removed"
	}
	peerKind := "direct"
	if ev.ThreadType == protocol.ThreadTypeGroup {
		peerKind = "group"
		metadata["group_id"] = ev.ThreadID
	}

	_ = store.WithTenantID(context.Background(), c.TenantID())
	c.HandleMessage(ev.UIDFrom, ev.ThreadID, formatReactionLine(ev), nil, metadata, peerKind)
}

func formatReactionLine(ev ReactionEvent) string {
	name := ev.DName
	if name == "" {
		name = ev.UIDFrom
	}
	if ev.Code == protocol.ReactionNone {
		return fmt.Sprintf("[reaction] %s removed their reaction from message %s", name, ev.MsgID)
	}
	icon := ev.Code
	if u := protocol.ReactionCodeToUnicode(ev.Code); u != "" {
		icon = u
	}
	return fmt.Sprintf("[reaction] %s reacted %s to message %s", name, icon, ev.MsgID)
}
