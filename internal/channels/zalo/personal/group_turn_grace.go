package personal

import (
	"context"
	"log/slog"
	"maps"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type inboundTurn struct {
	senderID      string
	threadID      string
	content       string
	media         []string
	metadata      map[string]string
	peerKind      string
	threadType    protocol.ThreadType
	messageCount  int
	firstQueuedAt time.Time
}

func (c *Channel) enqueueInboundTurn(turn inboundTurn) {
	if c.turnCoalescer == nil || c.turnCoalescer.Delay() <= 0 {
		c.dispatchInboundTurn(turn)
		return
	}

	now := time.Now()
	turn = cloneInboundTurn(turn)
	if turn.firstQueuedAt.IsZero() {
		turn.firstQueuedAt = now
	}

	delay := c.turnCoalescer.Delay()
	messageCount := turn.messageCount
	mediaCount := len(turn.media)
	c.turnCoalescer.Enqueue(inboundTurnKey(turn), turn)

	slog.Debug("zalo_personal inbound turn buffered",
		"thread_id", turn.threadID,
		"peer_kind", turn.peerKind,
		"wait_ms", delay.Milliseconds(),
		"message_count", messageCount,
		"media_count", mediaCount)
}

func (c *Channel) flushPendingInboundTurns() {
	if c.turnCoalescer != nil {
		c.turnCoalescer.FlushAll()
	}
}

func (c *Channel) dispatchInboundTurn(turn inboundTurn) {
	// #255: re-check policy at dequeue. A turn accepted under an old policy may have
	// sat in the coalesce/grace window while the sender was blocked. Re-evaluate just
	// before running so a now-disallowed sender's queued turn is dropped, not processed.
	if !c.policyAllowsAtDequeue(turn) {
		slog.Warn("inbound: dropped — sender no longer allowed by policy at dequeue",
			"thread_id", turn.threadID,
			"sender_id", turn.senderID,
			"peer_kind", turn.peerKind)
		if turn.peerKind == "group" {
			if gh := c.GroupHistory(); gh != nil {
				gh.Clear(turn.threadID)
			}
		}
		return
	}

	finalContent := turn.content
	var histMedia []string
	if turn.peerKind == "group" {
		if gh := c.GroupHistory(); gh != nil && c.HistoryLimit() > 0 {
			finalContent, histMedia = gh.BuildContextAndCollectMedia(turn.threadID, turn.content, c.HistoryLimit())
		}
	}

	allMedia := make([]string, 0, len(histMedia)+len(turn.media))
	allMedia = append(allMedia, histMedia...)
	allMedia = append(allMedia, turn.media...)

	c.startTyping(turn.threadID, turn.threadType)
	c.HandleMessage(turn.senderID, turn.threadID, finalContent, allMedia, turn.metadata, turn.peerKind)

	if turn.peerKind == "group" {
		if gh := c.GroupHistory(); gh != nil {
			gh.Clear(turn.threadID)
		}
	}

	waitMs := int64(0)
	if !turn.firstQueuedAt.IsZero() {
		waitMs = time.Since(turn.firstQueuedAt).Milliseconds()
	}
	slog.Info("zalo_personal inbound turn flushed",
		"thread_id", turn.threadID,
		"peer_kind", turn.peerKind,
		"wait_ms", waitMs,
		"message_count", max(turn.messageCount, 1),
		"history_media_count", len(histMedia),
		"current_media_count", len(turn.media))
}

// policyAllowsAtDequeue re-evaluates the sender's access policy at the moment a
// queued turn is about to run. Returns false only on an explicit PolicyDeny (sender
// removed from allowlist / channel disabled). PolicyNeedsPairing is treated as
// still-allowed here — pairing was already handled at enqueue and we must not
// re-trigger a pairing reply from the dequeue path.
func (c *Channel) policyAllowsAtDequeue(turn inboundTurn) bool {
	ctx := store.WithTenantID(context.Background(), c.TenantID())
	var result channels.PolicyResult
	if turn.peerKind == "group" {
		result = c.CheckGroupPolicy(ctx, turn.senderID, turn.threadID, c.config.GroupPolicy)
	} else {
		result = c.CheckDMPolicy(ctx, turn.senderID, c.config.DMPolicy)
	}
	return result != channels.PolicyDeny
}

func cloneInboundTurn(turn inboundTurn) inboundTurn {
	if turn.messageCount <= 0 {
		turn.messageCount = 1
	}
	turn.media = append([]string(nil), turn.media...)
	turn.metadata = maps.Clone(turn.metadata)
	return turn
}

func mergeInboundTurns(existing, next inboundTurn) inboundTurn {
	merged := cloneInboundTurn(existing)
	next = cloneInboundTurn(next)

	merged.content = joinNonEmpty("\n\n", merged.content, next.content)
	merged.media = append(merged.media, next.media...)
	merged.metadata = next.metadata
	merged.senderID = next.senderID
	merged.peerKind = next.peerKind
	merged.threadType = next.threadType
	merged.messageCount += next.messageCount
	if merged.firstQueuedAt.IsZero() {
		merged.firstQueuedAt = next.firstQueuedAt
	}
	return merged
}

func inboundTurnKey(turn inboundTurn) string {
	return turn.peerKind + ":" + turn.threadID + ":" + turn.senderID
}

func joinNonEmpty(sep string, parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, sep)
}
