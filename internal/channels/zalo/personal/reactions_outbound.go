package personal

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
)

const (
	reactionDebounceMs           = 700 * time.Millisecond
	reactionTombstoneTTL         = 60 * time.Second
	defaultReactionTerminalMinMs = 800 * time.Millisecond
	defaultReactionTerminalMaxMs = 2000 * time.Millisecond
	reactionLengthBonusPerCharMs = 1 * time.Millisecond
	reactionLengthBonusCap       = 1500 * time.Millisecond
)

// statusReactionVariants maps agent lifecycle states to the picker emojis.
// Tone matches zalo-oa: a single working ack + warm/sad terminal. Angry is
// excluded — reacting angry on the user's own message reads as blaming them.
var statusReactionVariants = map[string][]string{
	"thinking": {protocol.ReactionLike, protocol.ReactionHeart},
	"done":     {protocol.ReactionHeart, protocol.ReactionLike},
	"error":    {protocol.ReactionCry},
}

func resolveOutboundReactionEmoji(status string) string {
	variants, ok := statusReactionVariants[status]
	if !ok {
		return ""
	}
	for _, v := range variants {
		if _, supported := reactionMetaTableLookup(v); supported {
			return v
		}
	}
	return ""
}

func reactionMetaTableLookup(code string) (struct{}, bool) {
	m := protocol.LookupReactionMeta(code)
	if m.RType < 0 {
		return struct{}{}, false
	}
	return struct{}{}, true
}

var _ channels.ReactionChannel = (*Channel)(nil)

type personalReactionController struct {
	ch              *Channel
	threadID        string
	sourceMessageID string

	mu            sync.Mutex
	currentIcon   string
	lastStatus    string
	terminal      bool
	debounceTimer *time.Timer
	tombstoneOnce sync.Once
}

func newPersonalReactionController(ch *Channel, threadID, sourceMessageID string) *personalReactionController {
	return &personalReactionController{ch: ch, threadID: threadID, sourceMessageID: sourceMessageID}
}

func (rc *personalReactionController) SetStatus(ctx context.Context, status string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if rc.terminal {
		return
	}
	rc.lastStatus = status

	if status == "done" || status == "error" {
		rc.terminal = true
		rc.cancelDebounceLocked()
		icon := resolveOutboundReactionEmoji(status)
		if icon == "" {
			return
		}
		select {
		case <-rc.ch.stopCh:
			return
		default:
		}
		rc.ch.reactionWG.Add(1)
		rc.debounceTimer = time.AfterFunc(rc.ch.terminalReactionDelay(rc.threadID), func() {
			defer rc.ch.reactionWG.Done()
			rc.mu.Lock()
			defer rc.mu.Unlock()
			rc.applyReactionLocked(rc.ch.reactionCtx, icon)
		})
		return
	}

	if _, mapped := statusReactionVariants[status]; !mapped {
		return
	}

	rc.cancelDebounceLocked()
	select {
	case <-rc.ch.stopCh:
		return
	default:
	}
	rc.ch.reactionWG.Add(1)
	rc.debounceTimer = time.AfterFunc(reactionDebounceMs, func() {
		defer rc.ch.reactionWG.Done()
		rc.mu.Lock()
		defer rc.mu.Unlock()
		if rc.terminal {
			return
		}
		if icon := resolveOutboundReactionEmoji(rc.lastStatus); icon != "" {
			rc.applyReactionLocked(rc.ch.reactionCtx, icon)
		}
	})
}

func (rc *personalReactionController) Stop() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.cancelDebounceLocked()
}

func (rc *personalReactionController) cancelDebounceLocked() {
	if rc.debounceTimer != nil {
		if rc.debounceTimer.Stop() {
			rc.ch.reactionWG.Done()
		}
		rc.debounceTimer = nil
	}
}

func (rc *personalReactionController) applyReactionLocked(ctx context.Context, icon string) {
	if icon == rc.currentIcon {
		return
	}
	sess := rc.ch.session()
	if sess == nil {
		return
	}
	threadType := protocol.ThreadTypeUser
	if rc.ch.IsGroupApproved(rc.threadID) {
		threadType = protocol.ThreadTypeGroup
	}
	dest := protocol.ReactionDest{
		MsgID:    rc.sourceMessageID,
		CliMsgID: "0",
		ThreadID: rc.threadID,
		Type:     threadType,
	}
	if _, err := protocol.AddReaction(ctx, sess, dest, icon); err != nil {
		slog.Debug("zalo_personal.reaction.set_failed",
			"thread_id", rc.threadID,
			"source_message_id", rc.sourceMessageID,
			"icon", icon,
			"error", err)
		return
	}
	rc.currentIcon = icon
}

func (c *Channel) terminalReactionDelay(chatID string) time.Duration {
	minD := defaultReactionTerminalMinMs
	maxD := defaultReactionTerminalMaxMs
	if c.config.ReactionTerminalDelayMinMs > 0 {
		minD = time.Duration(c.config.ReactionTerminalDelayMinMs) * time.Millisecond
	}
	if c.config.ReactionTerminalDelayMaxMs > 0 {
		maxD = time.Duration(c.config.ReactionTerminalDelayMaxMs) * time.Millisecond
	}
	if maxD < minD {
		maxD = minD
	}
	d := minD
	if maxD > minD {
		d += time.Duration(rand.Int64N(int64(maxD-minD) + 1))
	}
	if v, ok := c.lastReplyChars.Load(chatID); ok {
		if n, ok := v.(int); ok && n > 0 {
			bonus := time.Duration(n) * reactionLengthBonusPerCharMs
			if bonus > reactionLengthBonusCap {
				bonus = reactionLengthBonusCap
			}
			d += bonus
		}
	}
	return d
}

func (c *Channel) recordReactionReplyLen(chatID string, n int) {
	if chatID == "" || n <= 0 {
		return
	}
	c.lastReplyChars.Store(chatID, n)
}

// OnReactionEvent is invoked by the agent lifecycle bus with the agent's
// status for the user's source message. Deterministic — no LLM involvement.
func (c *Channel) OnReactionEvent(ctx context.Context, chatID, messageID, status string) error {
	if c.config.ReactionLevel == "" || c.config.ReactionLevel == "off" {
		return nil
	}
	if c.config.ReactionLevel == "minimal" && status != "done" && status != "error" {
		return nil
	}
	if chatID == "" || messageID == "" {
		return nil
	}
	select {
	case <-c.stopCh:
		return nil
	default:
	}

	key := chatID + ":" + messageID
	val, _ := c.reactions.LoadOrStore(key, newPersonalReactionController(c, chatID, messageID))
	rc, ok := val.(*personalReactionController)
	if !ok {
		return nil
	}
	rc.SetStatus(ctx, status)

	if status == "done" || status == "error" {
		rc.tombstoneOnce.Do(func() {
			select {
			case <-c.stopCh:
				return
			default:
			}
			c.reactionWG.Add(1)
			go func() {
				defer c.reactionWG.Done()
				t := time.NewTimer(reactionTombstoneTTL)
				defer t.Stop()
				select {
				case <-t.C:
					c.reactions.CompareAndDelete(key, rc)
				case <-c.stopCh:
				}
			}()
		})
	}
	return nil
}

// ClearReaction removes any outstanding reaction on a user message — used by
// the agent's cancel/abort path. Maps to a NONE reaction on Zalo.
func (c *Channel) ClearReaction(ctx context.Context, chatID, messageID string) error {
	if chatID == "" || messageID == "" {
		return nil
	}
	key := chatID + ":" + messageID
	if val, ok := c.reactions.LoadAndDelete(key); ok {
		if rc, ok := val.(*personalReactionController); ok {
			rc.Stop()
		}
	}
	sess := c.session()
	if sess == nil {
		return nil
	}
	threadType := protocol.ThreadTypeUser
	if c.IsGroupApproved(chatID) {
		threadType = protocol.ThreadTypeGroup
	}
	_, err := protocol.AddReaction(ctx, sess, protocol.ReactionDest{
		MsgID:    messageID,
		CliMsgID: "0",
		ThreadID: chatID,
		Type:     threadType,
	}, protocol.ReactionNone)
	return err
}
