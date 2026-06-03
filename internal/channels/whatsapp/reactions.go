package whatsapp

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/sessions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

const (
	whatsappMessageLogSourceType = "whatsapp_msg"
	whatsappReactionSourceType   = "reaction_feedback"
	whatsappPreviewMax           = 200
	whatsappMessageLogTTL        = 48 * time.Hour
	whatsappReactionTTL          = 7 * 24 * time.Hour
)

type whatsappReaction struct {
	ReactionMsgID     string
	TargetMsgID       string
	TargetChatID      string
	TargetParticipant string
	TargetFromMe      bool
	ReactorID         string
	ReactorName       string
	ChatID            string
	PeerKind          string
	Emoji             string
	Sentiment         string
	Timestamp         time.Time
	Removed           bool
}

func (c *Channel) handleReactionMessage(ctx context.Context, evt *events.Message, senderID, chatID, peerKind string) bool {
	if evt == nil || evt.Message == nil {
		return false
	}

	reaction := evt.Message.GetReactionMessage()
	if reaction == nil && evt.Message.GetEncReactionMessage() != nil {
		if c.client == nil {
			slog.Warn("whatsapp.reaction.decrypt_skipped", "reason", "client nil")
			return true
		}
		decrypted, err := c.client.DecryptReaction(ctx, evt)
		if err != nil {
			slog.Warn("whatsapp.reaction.decrypt_failed", "err", err, "message_id", evt.Info.ID)
			return true
		}
		reaction = decrypted
	}
	if reaction == nil {
		return false
	}
	if c.config.DisableReactions {
		return true
	}

	ev, ok := extractWhatsAppReaction(evt, reaction, senderID, chatID, peerKind)
	if !ok {
		slog.Warn("whatsapp.reaction.malformed", "message_id", evt.Info.ID)
		return true
	}

	slog.Info("whatsapp.reaction.feedback",
		"channel", c.Name(),
		"chat_id", ev.ChatID,
		"peer_kind", ev.PeerKind,
		"reactor_id", ev.ReactorID,
		"reactor_name", ev.ReactorName,
		"target_msg_id", ev.TargetMsgID,
		"target_chat_id", ev.TargetChatID,
		"target_participant", ev.TargetParticipant,
		"emoji", ev.Emoji,
		"sentiment", ev.Sentiment,
		"removed", ev.Removed,
		"reaction_at", ev.Timestamp,
	)
	c.recordReactionFeedback(ctx, ev)
	return true
}

func extractWhatsAppReaction(
	evt *events.Message,
	reaction *waE2E.ReactionMessage,
	senderID string,
	chatID string,
	peerKind string,
) (whatsappReaction, bool) {
	if evt == nil || reaction == nil || reaction.GetKey() == nil {
		return whatsappReaction{}, false
	}

	key := reaction.GetKey()
	targetMsgID := key.GetID()
	if targetMsgID == "" {
		return whatsappReaction{}, false
	}

	reactorName := channels.SanitizeDisplayName(evt.Info.PushName)
	if reactorName == "" {
		reactorName = senderID
	}

	emoji := reaction.GetText()
	ts := reactionTimestamp(reaction.GetSenderTimestampMS(), evt.Info.Timestamp)
	return whatsappReaction{
		ReactionMsgID:     string(evt.Info.ID),
		TargetMsgID:       targetMsgID,
		TargetChatID:      firstNonEmpty(key.GetRemoteJID(), chatID),
		TargetParticipant: key.GetParticipant(),
		TargetFromMe:      key.GetFromMe(),
		ReactorID:         senderID,
		ReactorName:       reactorName,
		ChatID:            chatID,
		PeerKind:          peerKind,
		Emoji:             emoji,
		Sentiment:         whatsappReactionSentiment(emoji),
		Timestamp:         ts,
		Removed:           emoji == "",
	}, true
}

func reactionTimestamp(senderTimestampMS int64, fallback time.Time) time.Time {
	if senderTimestampMS > 0 {
		return time.UnixMilli(senderTimestampMS).UTC()
	}
	if !fallback.IsZero() {
		return fallback.UTC()
	}
	return time.Now().UTC()
}

func (c *Channel) recordReactionFeedback(ctx context.Context, ev whatsappReaction) {
	if c.episodicStore == nil {
		return
	}
	agentUUID := c.AgentUUID()
	if agentUUID == uuid.Nil {
		return
	}

	preview := c.lookupMessagePreview(ctx, ev.TargetMsgID)
	slog.Info("whatsapp.reaction.preview_lookup",
		"target_msg_id", ev.TargetMsgID,
		"hit", preview != "",
		"preview_len", len(preview),
	)

	expiresAt := time.Now().Add(whatsappReactionTTL)
	ep := &store.EpisodicSummary{
		TenantID:   c.TenantID(),
		AgentID:    agentUUID,
		UserID:     c.reactionMemoryUserID(ev),
		SessionKey: sessions.BuildSessionKey(c.AgentID(), c.Name(), sessions.PeerKindFromGroup(ev.PeerKind == string(sessions.PeerGroup)), ev.ChatID),
		Summary:    buildWhatsAppReactionSummary(ev, preview),
		SourceType: whatsappReactionSourceType,
		SourceID:   whatsappReactionSourceID(ev),
		CreatedAt:  ev.Timestamp,
		ExpiresAt:  &expiresAt,
	}
	ep.L0Abstract = ep.Summary

	tctx, cancel := context.WithTimeout(store.WithTenantID(ctx, c.TenantID()), 5*time.Second)
	defer cancel()
	if err := c.episodicStore.Create(tctx, ep); err != nil {
		slog.Warn("whatsapp.reaction.persist_failed", "err", err, "target_msg_id", ev.TargetMsgID)
	}
}

func (c *Channel) reactionMemoryUserID(ev whatsappReaction) string {
	if ev.PeerKind == string(sessions.PeerGroup) && ev.ChatID != "" {
		return fmt.Sprintf("group:%s:%s", c.Name(), ev.ChatID)
	}
	return ev.ReactorID
}

func buildWhatsAppReactionSummary(ev whatsappReaction, preview string) string {
	at := ev.Timestamp.Format(time.RFC3339)
	if ev.Removed {
		if preview != "" {
			return fmt.Sprintf(`%s removed their reaction on %s: %q at %s (target message %s)`,
				ev.ReactorName, reactionTargetLabel(ev.TargetFromMe), preview, at, ev.TargetMsgID)
		}
		return fmt.Sprintf("%s removed their reaction on message %s at %s", ev.ReactorName, ev.TargetMsgID, at)
	}
	if preview != "" {
		return fmt.Sprintf(`%s reacted %s (%s) on %s: %q at %s (target message %s)`,
			ev.ReactorName, ev.Emoji, ev.Sentiment, reactionTargetLabel(ev.TargetFromMe), preview, at, ev.TargetMsgID)
	}
	return fmt.Sprintf("%s reacted %s (%s) on message %s at %s", ev.ReactorName, ev.Emoji, ev.Sentiment, ev.TargetMsgID, at)
}

func reactionTargetLabel(targetFromMe bool) string {
	if targetFromMe {
		return "your reply"
	}
	return "message"
}

func whatsappReactionSourceID(ev whatsappReaction) string {
	dedupe := ev.ReactionMsgID
	if dedupe == "" {
		code := ev.Emoji
		if ev.Removed {
			code = "removed"
		}
		dedupe = fmt.Sprintf("%s:%d", code, ev.Timestamp.UnixMilli())
	}
	return fmt.Sprintf("react:%s:%s:%s", ev.TargetMsgID, ev.ReactorID, dedupe)
}

func whatsappMessageSourceID(msgID string) string {
	return "whatsapp:msg:" + msgID
}

func (c *Channel) recordMessagePreview(ctx context.Context, msgID, preview string, createdAt time.Time) {
	if c.episodicStore == nil || msgID == "" || strings.TrimSpace(preview) == "" {
		return
	}
	agentUUID := c.AgentUUID()
	if agentUUID == uuid.Nil {
		return
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	expiresAt := time.Now().Add(whatsappMessageLogTTL)
	preview = truncatePreview(preview, whatsappPreviewMax)
	ep := &store.EpisodicSummary{
		TenantID:   c.TenantID(),
		AgentID:    agentUUID,
		UserID:     "",
		SessionKey: "",
		Summary:    preview,
		L0Abstract: preview,
		SourceType: whatsappMessageLogSourceType,
		SourceID:   whatsappMessageSourceID(msgID),
		CreatedAt:  createdAt.UTC(),
		ExpiresAt:  &expiresAt,
	}
	tctx, cancel := context.WithTimeout(store.WithTenantID(ctx, c.TenantID()), 3*time.Second)
	defer cancel()
	if err := c.episodicStore.Create(tctx, ep); err != nil {
		slog.Warn("whatsapp.message_preview.persist_failed", "err", err, "msg_id", msgID)
	}
}

func (c *Channel) lookupMessagePreview(ctx context.Context, msgID string) string {
	if c.episodicStore == nil || msgID == "" {
		return ""
	}
	agentUUID := c.AgentUUID()
	if agentUUID == uuid.Nil {
		return ""
	}
	tctx, cancel := context.WithTimeout(store.WithTenantID(ctx, c.TenantID()), 3*time.Second)
	defer cancel()
	ep, err := c.episodicStore.GetBySourceID(tctx, agentUUID.String(), "", whatsappMessageSourceID(msgID))
	if err != nil {
		slog.Warn("whatsapp.message_preview.lookup_failed", "err", err, "msg_id", msgID)
		return ""
	}
	if ep == nil {
		return ""
	}
	return ep.Summary
}

func whatsappReactionSentiment(emoji string) string {
	switch emoji {
	case "❤", "❤️", "👍", "😍", "😂", "😄", "😆", "👌", "👏", "💪":
		return "positive"
	case "😢", "😭", "😡", "😠", "👎":
		return "negative"
	case "😮", "😯", "😲", "😳":
		return "surprise"
	case "":
		return "unknown"
	default:
		return "unknown"
	}
}

func mediaAttachmentPreview(kind, filePath, caption string) string {
	name := filepath.Base(filePath)
	if caption != "" {
		return fmt.Sprintf("[%s: %s] %s", kind, name, truncatePreview(caption, whatsappPreviewMax-len(name)-12))
	}
	return fmt.Sprintf("[%s: %s]", kind, name)
}

func mediaKindFromMIME(mime string) string {
	switch {
	case strings.HasPrefix(mime, "image/"):
		return "image"
	case strings.HasPrefix(mime, "video/"):
		return "video"
	case strings.HasPrefix(mime, "audio/"):
		return "audio"
	default:
		return "file"
	}
}

func truncatePreview(s string, max int) string {
	if max <= 0 {
		max = whatsappPreviewMax
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	rs := []rune(s)
	return string(rs[:max]) + "..."
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
