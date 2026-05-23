package personal

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
	"github.com/nextlevelbuilder/goclaw/internal/sessions"
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

const (
	reactionsModeSilent   = "silent"
	reactionsModeFeedback = "feedback"
	reactionsModeInbound  = "inbound"
)

func (c *Channel) reactionsMode() string {
	if c.config.DisableReactions {
		return reactionsModeSilent
	}
	switch c.config.ReactionsMode {
	case reactionsModeSilent, reactionsModeFeedback, reactionsModeInbound:
		return c.config.ReactionsMode
	}
	return reactionsModeFeedback
}

func (c *Channel) onReactionEvent(ev ReactionEvent) {
	if ev.IsHistoric {
		return
	}
	if ev.IsSelf && !c.config.ListenSelfReactions {
		return
	}
	switch c.reactionsMode() {
	case reactionsModeSilent:
		return
	case reactionsModeFeedback:
		c.recordReactionFeedback(ev)
		return
	}
	if c.reactionCoalescer == nil {
		c.emitCoalescedReaction(ev)
		return
	}
	c.reactionCoalescer.Submit(ev)
}

func (c *Channel) recordReactionFeedback(ev ReactionEvent) {
	icon := ev.Code
	if u := protocol.ReactionCodeToUnicode(ev.Code); u != "" {
		icon = u
	}
	sentiment := reactionSentiment(ev.Code)
	reactorName := channels.SanitizeDisplayName(ev.DName)
	if reactorName == "" {
		reactorName = ev.UIDFrom
	}

	slog.Info("zalo_personal.reaction.feedback",
		"channel", c.Name(),
		"thread_id", ev.ThreadID,
		"thread_type", reactionThreadTypeName(ev.ThreadType),
		"reactor_uid", ev.UIDFrom,
		"reactor_name", reactorName,
		"target_msg_id", ev.MsgID,
		"target_cli_msg_id", ev.CliMsgID,
		"code", ev.Code,
		"icon", icon,
		"sentiment", sentiment,
	)

	if c.episodicStore == nil {
		return
	}
	agentUUID := c.AgentUUID()
	if agentUUID == uuid.Nil {
		return
	}

	preview := ""
	if c.outboundCache != nil {
		preview = c.outboundCache.get(ev.MsgID)
	}
	summary := buildReactionSummary(reactorName, icon, sentiment, ev.MsgID, preview, ev.Code == protocol.ReactionNone)

	sessionKey := sessions.BuildSessionKey(c.AgentID(), c.Type(), sessions.PeerKindFromGroup(ev.ThreadType == protocol.ThreadTypeGroup), ev.ThreadID)
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	ep := &store.EpisodicSummary{
		TenantID:   c.TenantID(),
		AgentID:    agentUUID,
		UserID:     ev.UIDFrom,
		SessionKey: sessionKey,
		Summary:    summary,
		L0Abstract: summary,
		SourceType: "reaction_feedback",
		SourceID:   fmt.Sprintf("react:%s:%s:%s", ev.MsgID, ev.UIDFrom, ev.Code),
		ExpiresAt:  &expiresAt,
	}
	ctx, cancel := context.WithTimeout(store.WithTenantID(context.Background(), c.TenantID()), 5*time.Second)
	defer cancel()
	if err := c.episodicStore.Create(ctx, ep); err != nil {
		slog.Warn("zalo_personal.reaction.persist_failed", "err", err, "target_msg_id", ev.MsgID)
	}
}

func reactionThreadTypeName(t protocol.ThreadType) string {
	if t == protocol.ThreadTypeGroup {
		return "group"
	}
	return "direct"
}

func buildReactionSummary(reactorName, icon, sentiment, msgID, preview string, removed bool) string {
	if removed {
		if preview != "" {
			return fmt.Sprintf(`%s removed their reaction on your reply: %q`, reactorName, preview)
		}
		return fmt.Sprintf("%s removed their reaction on message %s", reactorName, msgID)
	}
	if preview != "" {
		return fmt.Sprintf(`%s reacted %s (%s) on your reply: %q`, reactorName, icon, sentiment, preview)
	}
	return fmt.Sprintf("%s reacted %s (%s) on message %s", reactorName, icon, sentiment, msgID)
}

func reactionSentiment(code string) string {
	switch code {
	case protocol.ReactionHeart, protocol.ReactionLike, protocol.ReactionHaha:
		return "positive"
	case protocol.ReactionAngry, protocol.ReactionCry, protocol.ReactionWorry:
		return "negative"
	case protocol.ReactionWow:
		return "surprise"
	}
	return "unknown"
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
