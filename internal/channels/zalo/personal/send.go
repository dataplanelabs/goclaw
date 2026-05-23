package personal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/channels/typing"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/common"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
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

	quote := readQuoteFromMetadata(msg.Metadata)

	if quote != nil {
		// Quoted-reply order: text-with-quote FIRST so the quote bubble anchors
		// the reply, then media (always unquoted — Zalo's /quote endpoint takes
		// only text + qmsg* params, matching zca-js's two-request split).
		if msg.Content != "" {
			if err := c.sendChunkedText(ctx, sess, msg.ChatID, threadType, msg.Content, quote); err != nil {
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
		return c.sendChunkedText(ctx, sess, msg.ChatID, threadType, msg.Content, nil)
	}
	return nil
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

	_, err = protocol.SendImage(ctx, sess, chatID, threadType, upload, caption)
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

	_, err = protocol.SendFile(ctx, sess, chatID, threadType, upload)
	return err
}

// sendChunkedText splits text and sends each chunk. When quote is non-nil it
// attaches to the FIRST chunk only — continuation chunks ride unquoted to
// match zca-js behavior and the zalo-oa convention. On ErrQuoteRejected
// (e.g. quoted source deleted) the helper retries once without the quote so
// the reply still lands.
func (c *Channel) sendChunkedText(ctx context.Context, sess *protocol.Session, chatID string, threadType protocol.ThreadType, text string, quote *protocol.SendMessageQuote) error {
	chunks := channels.ChunkMarkdown(text, maxTextLength)
	for i, chunk := range chunks {
		var q *protocol.SendMessageQuote
		if i == 0 {
			q = quote
		}
		msgID, err := c.sendChunkWithQuoteFallback(ctx, sess, chatID, threadType, chunk, q)
		if err != nil {
			return err
		}
		if c.outboundCache != nil && msgID != "" {
			c.outboundCache.set(msgID, previewText(chunk, outboundPreviewMaxChars))
		}
	}
	return nil
}

const outboundPreviewMaxChars = 200

// sendChunkWithQuoteFallback sends a single text chunk with an optional quote.
// On ErrQuoteRejected (quote source deleted, expired, cross-thread, etc.) it
// retries once without the quote and logs a structured warn so ops can
// observe quote-drop rate. Mirrors zalo-oa's FamilyPayload retry at
// internal/channels/zalo/oa/send.go::postCSWithQuoteFallback.
func (c *Channel) sendChunkWithQuoteFallback(ctx context.Context, sess *protocol.Session, chatID string, threadType protocol.ThreadType, text string, quote *protocol.SendMessageQuote) (string, error) {
	if quote == nil {
		return protocol.SendMessage(ctx, sess, chatID, threadType, text, nil)
	}
	id, err := protocol.SendMessage(ctx, sess, chatID, threadType, text, quote)
	if errors.Is(err, protocol.ErrQuoteRejected) {
		slog.Warn("zalo_personal.quote.fallback_no_quote",
			"chat_id", chatID,
			"quoted_msg_id", quote.MsgID,
			"err", err,
			"hint", "quoted source likely deleted/expired or wire format mismatch; retrying without quote")
		return protocol.SendMessage(ctx, sess, chatID, threadType, text, nil)
	}
	return id, err
}
