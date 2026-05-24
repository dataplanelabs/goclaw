package personal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/channels/mentions"
	"github.com/nextlevelbuilder/goclaw/internal/channels/typing"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/common"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
	pkgproto "github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

const maxTextLength = 2000

// readQuoteFromMetadata deserializes the quote payload stamped on inbound
// (Phase 2's buildQuoteMetadata) back into a *protocol.SendMessageQuote
// suitable for the /quote endpoint. Returns nil when the key is absent or
// unparseable — callers fall through to non-quoted send.
func readQuoteFromMetadata(meta map[string]string) *protocol.SendMessageQuote {
	raw, ok := meta["reply_to_quote_payload"]
	if !ok || raw == "" {
		return nil
	}
	var q protocol.TQuote
	if err := json.Unmarshal([]byte(raw), &q); err != nil {
		slog.Warn("zalo_personal.quote.parse_metadata_failed", "err", err)
		return nil
	}
	return protocol.FromInboundQuote(&q)
}

// Send delivers an outbound message to a Zalo chat.
//
// When metadata carries a quote payload (Phase 2 stamped reply_to_quote_payload
// from an inbound TQuote), the first text chunk routes to Zalo's /quote
// endpoint so the message renders as a native quote bubble in the Zalo client.
// Subsequent text chunks and any media ride unquoted. The order also inverts
// when quoting (text-with-quote first, media after) so the quote bubble
// anchors the reply rather than getting buried under attachments.
func (c *Channel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	sess := c.session()
	if !c.IsRunning() || sess == nil {
		return fmt.Errorf("zalo_personal channel not running")
	}

	// Strip markdown — Zalo does not support any markup rendering.
	msg.Content = common.StripMarkdown(msg.Content)

	// Stop typing indicator before sending response
	if ctrl, ok := c.typingCtrls.LoadAndDelete(msg.ChatID); ok {
		ctrl.(*typing.Controller).Stop()
	}

	threadType := protocol.ThreadTypeUser
	if c.IsGroupApproved(msg.ChatID) {
		threadType = protocol.ThreadTypeGroup
	} else if msg.Metadata != nil {
		if _, ok := msg.Metadata["group_id"]; ok {
			threadType = protocol.ThreadTypeGroup
			c.MarkGroupApproved(msg.ChatID)
		}
	}

	if threadType == protocol.ThreadTypeGroup && msg.Metadata != nil {
		// Always prepend the asker mention — quote bubble alone doesn't reliably
		// notify on Android, and the explicit @ matches human-conversational style.
		msg.Content = applyAskerPrepend(msg.Content, msg.Metadata["sender_uid"])
	}
	if threadType == protocol.ThreadTypeGroup {
		msg.Content = c.wrapBareMentions(ctx, msg.ChatID, msg.Content)
	}

	rendered, allMentions := c.parseOutboundMentions(ctx, msg.ChatID, threadType, msg.Content)
	msg.Content = rendered

	quote := readQuoteFromMetadata(msg.Metadata)

	if quote != nil {
		// Quoted-reply order: text-with-quote FIRST so the quote bubble anchors
		// the reply, then media (always unquoted — Zalo's /quote endpoint takes
		// only text + qmsg* params, matching zca-js's two-request split).
		if msg.Content != "" {
			if err := c.sendChunkedText(ctx, sess, msg.ChatID, threadType, msg.Content, quote, allMentions); err != nil {
				return err
			}
		}
		c.sendMediaBestEffort(ctx, sess, msg.ChatID, threadType, msg.Media)
		return nil
	}

	// Non-quoted path: preserve historical media-first order so existing
	// zalo-personal users see no regression.
	c.sendMediaBestEffort(ctx, sess, msg.ChatID, threadType, msg.Media)
	if msg.Content != "" {
		return c.sendChunkedText(ctx, sess, msg.ChatID, threadType, msg.Content, nil, allMentions)
	}
	return nil
}

func (c *Channel) parseOutboundMentions(ctx context.Context, threadID string, threadType protocol.ThreadType, text string) (string, []pkgproto.Mention) {
	if text == "" || !strings.Contains(text, "@[") {
		return text, nil
	}
	resolve := func(marker string) (string, string, bool) {
		if name, ok := c.LookupGroupMember(ctx, threadID, marker); ok {
			return marker, name, true
		}
		if uid, name, ok := c.LookupGroupMemberByName(ctx, threadID, marker); ok {
			return uid, name, true
		}
		return "", "", false
	}
	rendered, ms := mentions.ParseMarkers(text, resolve)
	slog.Info("mention.parse",
		"channel", "zalo_personal",
		"thread_type", threadType,
		"resolved", len(ms),
		"unresolved", strings.Count(rendered, "@["),
	)
	return rendered, ms
}

// sendMediaBestEffort sends each attachment, logging (not returning) per-item
// failures so one bad attachment doesn't drop the rest of the reply. Existing
// behavior preserved from pre-quote code; extracted so both branches in Send
// share the same media loop.
func (c *Channel) sendMediaBestEffort(ctx context.Context, sess *protocol.Session, chatID string, threadType protocol.ThreadType, media []bus.MediaAttachment) {
	for _, m := range media {
		if protocol.IsImageFile(m.URL) {
			if err := c.sendImage(ctx, sess, chatID, threadType, m.URL, m.Caption); err != nil {
				slog.Warn("zalo_personal: failed to send image", "path", m.URL, "error", err)
			}
		} else {
			if err := c.sendFile(ctx, sess, chatID, threadType, m.URL); err != nil {
				slog.Warn("zalo_personal: failed to send file", "path", m.URL, "error", err)
			}
		}
	}
}

// sendImage uploads and sends an image file to a Zalo thread.
func (c *Channel) sendImage(ctx context.Context, sess *protocol.Session, chatID string, threadType protocol.ThreadType, filePath, caption string) error {
	upload, err := protocol.UploadImage(ctx, sess, chatID, threadType, filePath)
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}

	msgID, err := protocol.SendImage(ctx, sess, chatID, threadType, upload, caption)
	if err == nil {
		c.cacheOutboundMedia(msgID, "image", filePath, caption)
	}
	return err
}

// sendFile uploads and sends a file to a Zalo thread.
func (c *Channel) sendFile(ctx context.Context, sess *protocol.Session, chatID string, threadType protocol.ThreadType, filePath string) error {
	ln := c.getListener()
	if ln == nil {
		return fmt.Errorf("listener not available for file upload")
	}
	upload, err := protocol.UploadFile(ctx, sess, ln, chatID, threadType, filePath)
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}

	msgID, err := protocol.SendFile(ctx, sess, chatID, threadType, upload)
	if err == nil {
		c.cacheOutboundMedia(msgID, "file", filePath, "")
	}
	return err
}

func (c *Channel) cacheOutboundMedia(msgID, kind, filePath, caption string) {
	c.recordOutboundMessage(msgID, mediaPreview(kind, filePath, caption))
}

// sendChunkedText sends one chunk per request. Quote attaches to chunk 0 only.
// Mentions only ride on single-chunk sends — ChunkMarkdown's whitespace trim
// + fence injection drift the offsets for chunks 1..N, so multi-chunk drops
// mentions (text still renders as @DisplayName).
func (c *Channel) sendChunkedText(ctx context.Context, sess *protocol.Session, chatID string, threadType protocol.ThreadType, text string, quote *protocol.SendMessageQuote, allMentions []pkgproto.Mention) error {
	chunks := channels.ChunkMarkdown(text, maxTextLength)
	for i, chunk := range chunks {
		var q *protocol.SendMessageQuote
		if i == 0 {
			q = quote
		}
		var chunkMentions []pkgproto.Mention
		if len(allMentions) > 0 {
			if len(chunks) == 1 {
				chunkMentions = allMentions
			} else if i == 0 {
				slog.Warn("zalo_personal.mention.dropped_multichunk",
					"chat_id", chatID,
					"mentions", len(allMentions),
					"chunks", len(chunks),
					"hint", "multi-chunk send: highlight dropped, @DisplayName text intact")
			}
		}
		msgID, err := c.sendChunkWithFallbacks(ctx, sess, chatID, threadType, chunk, q, chunkMentions)
		if err != nil {
			return err
		}
		c.recordOutboundMessage(msgID, previewText(chunk, outboundPreviewMax))
	}
	return nil
}

// sendChunkWithFallbacks: ErrMentionRejected → retry without mentions;
// ErrQuoteRejected → retry without quote.
func (c *Channel) sendChunkWithFallbacks(ctx context.Context, sess *protocol.Session, chatID string, threadType protocol.ThreadType, text string, quote *protocol.SendMessageQuote, ms []pkgproto.Mention) (string, error) {
	opts := protocol.SendOptions{Text: text, Quote: quote, Mentions: ms}
	id, err := protocol.SendMessageWithOptions(ctx, sess, chatID, threadType, opts)
	if errors.Is(err, protocol.ErrMentionRejected) {
		slog.Warn("zalo_personal.mention.fallback_no_mention",
			"chat_id", chatID,
			"err", err,
			"hint", "mention endpoint rejected payload; retrying via /sendmsg with rewritten text")
		opts.Mentions = nil
		id, err = protocol.SendMessageWithOptions(ctx, sess, chatID, threadType, opts)
	}
	if errors.Is(err, protocol.ErrQuoteRejected) {
		slog.Warn("zalo_personal.quote.fallback_no_quote",
			"chat_id", chatID,
			"quoted_msg_id", quoteIDOrEmpty(quote),
			"err", err,
			"hint", "quoted source likely deleted/expired or wire format mismatch; retrying without quote")
		opts.Quote = nil
		id, err = protocol.SendMessageWithOptions(ctx, sess, chatID, threadType, opts)
	}
	return id, err
}

func quoteIDOrEmpty(q *protocol.SendMessageQuote) string {
	if q == nil {
		return ""
	}
	return q.MsgID
}
