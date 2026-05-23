package personal

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

const (
	outboundLogSourceType = "outbound_msg"
	outboundLogTTL        = 48 * time.Hour
	outboundPreviewMax    = 200
)

func outboundSourceID(msgID string) string {
	return "outbound:" + msgID
}

func (c *Channel) recordOutboundMessage(msgID, preview string) {
	if c.episodicStore == nil || msgID == "" || preview == "" {
		return
	}
	agentUUID := c.AgentUUID()
	if agentUUID == uuid.Nil {
		return
	}
	expiresAt := time.Now().Add(outboundLogTTL)
	ep := &store.EpisodicSummary{
		TenantID:   c.TenantID(),
		AgentID:    agentUUID,
		UserID:     "",
		SessionKey: "",
		Summary:    preview,
		L0Abstract: preview,
		SourceType: outboundLogSourceType,
		SourceID:   outboundSourceID(msgID),
		ExpiresAt:  &expiresAt,
	}
	ctx, cancel := context.WithTimeout(store.WithTenantID(context.Background(), c.TenantID()), 3*time.Second)
	defer cancel()
	if err := c.episodicStore.Create(ctx, ep); err != nil {
		slog.Warn("zalo_personal.outbound.persist_failed", "err", err, "msg_id", msgID)
		return
	}
	slog.Info("zalo_personal.outbound.cached", "msg_id", msgID, "preview_len", len(preview))
}

func (c *Channel) lookupOutboundPreview(ctx context.Context, msgID string) string {
	if c.episodicStore == nil || msgID == "" {
		return ""
	}
	agentUUID := c.AgentUUID()
	if agentUUID == uuid.Nil {
		return ""
	}
	tctx, cancel := context.WithTimeout(store.WithTenantID(ctx, c.TenantID()), 3*time.Second)
	defer cancel()
	ep, err := c.episodicStore.GetBySourceID(tctx, agentUUID.String(), "", outboundSourceID(msgID))
	if err != nil {
		slog.Warn("zalo_personal.outbound.lookup_failed", "err", err, "msg_id", msgID)
		return ""
	}
	if ep == nil {
		return ""
	}
	return ep.Summary
}

func previewText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	rs := []rune(s)
	if len(rs) <= max {
		return s
	}
	return string(rs[:max]) + "…"
}

func mediaPreview(kind, filePath, caption string) string {
	name := filePath
	for i := len(filePath) - 1; i >= 0; i-- {
		if filePath[i] == '/' {
			name = filePath[i+1:]
			break
		}
	}
	if caption != "" {
		return fmt.Sprintf("[%s: %s] %s", kind, name, previewText(caption, outboundPreviewMax-len(name)-12))
	}
	return fmt.Sprintf("[%s: %s]", kind, name)
}
