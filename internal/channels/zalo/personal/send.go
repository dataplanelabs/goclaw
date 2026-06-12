package personal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/channels/mentions"
	"github.com/nextlevelbuilder/goclaw/internal/channels/typing"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/common"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
	mediapkg "github.com/nextlevelbuilder/goclaw/internal/media"
	"github.com/nextlevelbuilder/goclaw/internal/store"
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
// Outbound pipeline position-mutation order (anyone adding a step MUST update
// style positions to match):
//  1. common.RenderStyles    — emits styles[] over post-strip text
//  2. applyAskerPrepend      — inserts "@[uid] " at head → shift styles right
//  3. wrapBareMentions       — SKIPPED when styles non-empty (would shift mid-text)
//  4. ParseMarkersWithStyles — removes @[uid] markers → adjusts remaining styles
//  5. sendChunkedText        — re-anchors mentions/styles to each chunk
func (c *Channel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	sess := c.session()
	if !c.IsRunning() || sess == nil {
		return fmt.Errorf("zalo_personal channel not running")
	}

	var outStyles []common.Style
	if c.enableNativeStyles {
		msg.Content, outStyles = common.RenderStyles(msg.Content)
	} else {
		msg.Content = common.StripMarkdown(msg.Content)
	}

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
		before := msg.Content
		msg.Content = applyAskerPrepend(msg.Content, msg.Metadata["sender_uid"])
		if len(outStyles) > 0 && msg.Content != before {
			shift := pkgproto.UTF16Len(msg.Content) - pkgproto.UTF16Len(before)
			for i := range outStyles {
				outStyles[i].Start += shift
			}
		}
	}
	if threadType == protocol.ThreadTypeGroup {
		if len(outStyles) == 0 {
			msg.Content = c.wrapBareMentions(ctx, msg.ChatID, msg.Content)
		} else {
			slog.Debug("zalo_personal.style.bare_wrap_skipped",
				"chat_id", msg.ChatID,
				"len_styles", len(outStyles))
		}
	}

	rendered, allMentions, adjustedStyles := c.parseOutboundMentionsWithStyles(ctx, msg.ChatID, threadType, msg.Content, outStyles)
	msg.Content = rendered
	outStyles = adjustedStyles

	// Defense-in-depth: drop invalid styles before they reach the wire.
	if len(outStyles) > 0 {
		filtered := outStyles[:0]
		for _, s := range outStyles {
			if s.Len > 0 && s.Start >= 0 {
				filtered = append(filtered, s)
			}
		}
		outStyles = filtered
	}

	quote := readQuoteFromMetadata(msg.Metadata)

	if quote != nil {
		// Quoted-reply order: text-with-quote FIRST so the quote bubble anchors
		// the reply, then media (always unquoted — Zalo's /quote endpoint takes
		// only text + qmsg* params, matching zca-js's two-request split).
		if msg.Content != "" {
			if err := c.sendChunkedText(ctx, sess, msg.ChatID, threadType, msg.Content, quote, allMentions, outStyles); err != nil {
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
		return c.sendChunkedText(ctx, sess, msg.ChatID, threadType, msg.Content, nil, allMentions, outStyles)
	}
	return nil
}

// parseOutboundMentionsWithStyles routes through ParseMarkersWithStyles when
// styles are present; falls back to ParseMarkers when not.
func (c *Channel) parseOutboundMentionsWithStyles(ctx context.Context, threadID string, threadType protocol.ThreadType, text string, inStyles []common.Style) (string, []pkgproto.Mention, []common.Style) {
	if text == "" || !strings.Contains(text, "@[") {
		return text, nil, inStyles
	}
	if len(inStyles) == 0 {
		rendered, ms := c.parseOutboundMentions(ctx, threadID, threadType, text)
		return rendered, ms, nil
	}
	resolve := c.mentionResolver(ctx, threadID)
	// Convert []common.Style ↔ []mentions.Style across the boundary.
	mStyles := make([]mentions.Style, len(inStyles))
	for i, s := range inStyles {
		mStyles[i] = mentions.Style{Start: s.Start, Len: s.Len, St: s.St}
	}
	rendered, ms, adjusted := mentions.ParseMarkersWithStyles(text, resolve, mStyles)
	out := make([]common.Style, len(adjusted))
	for i, s := range adjusted {
		out[i] = common.Style{Start: s.Start, Len: s.Len, St: s.St}
	}
	slog.Info("mention.parse",
		"channel", "zalo_personal",
		"thread_type", threadType,
		"resolved", len(ms),
		"styles", len(out),
	)
	return rendered, ms, out
}

// mentionResolver chains: group member cache (uid, then name) → rate-limited
// member fetch → channel_contacts. The contacts fallback covers cold caches
// after pod restart; a contacts hit may be a non-member, which still renders
// "@Name" (Zalo drops mention spans for non-members server-side).
func (c *Channel) mentionResolver(ctx context.Context, threadID string) mentions.Resolve {
	return func(marker string) (string, string, bool) {
		if name, ok := c.LookupGroupMember(ctx, threadID, marker); ok {
			return marker, name, true
		}
		if uid, name, ok := c.LookupGroupMemberByName(ctx, threadID, marker); ok {
			return uid, name, true
		}
		if name, ok := c.lookupContactDisplayName(ctx, marker); ok {
			slog.Info("zalo_personal.mention.contacts_fallback",
				"thread_id", threadID, "uid", marker, "name", name)
			return marker, name, true
		}
		return "", "", false
	}
}

func (c *Channel) lookupContactDisplayName(ctx context.Context, uid string) (string, bool) {
	cc := c.ContactCollector()
	if cc == nil {
		return "", false
	}
	return cc.DisplayNameBySenderID(store.WithTenantID(ctx, c.TenantID()), c.Type(), uid)
}

func (c *Channel) parseOutboundMentions(ctx context.Context, threadID string, threadType protocol.ThreadType, text string) (string, []pkgproto.Mention) {
	if text == "" || !strings.Contains(text, "@[") {
		return text, nil
	}
	rendered, ms := mentions.ParseMarkers(text, c.mentionResolver(ctx, threadID))
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
		if !c.config.DisableVoiceSend && mediapkg.IsAudioExt(m.URL) {
			if err := c.sendVoice(ctx, sess, chatID, threadType, m.URL); err == nil {
				continue
			} else {
				slog.Warn("zalo_personal: voice send failed, falling back to share.file", "path", m.URL, "error", err)
			}
		}
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

// sendVoice normalizes audio to ADTS AAC (Zalo's own voice format) then sends
// via Zalo's voice endpoint. Returns an error so the caller can fall back to
// share.file on failure.
func (c *Channel) sendVoice(ctx context.Context, sess *protocol.Session, chatID string, threadType protocol.ThreadType, filePath string) error {
	ln := c.getListener()
	if ln == nil {
		return fmt.Errorf("listener not available for voice upload")
	}
	normalized, err := mediapkg.NormalizeAudio(ctx, filePath, "aac")
	if err != nil {
		return fmt.Errorf("normalize audio: %w", err)
	}
	if normalized != filePath {
		defer func() { _ = os.Remove(normalized) }()
	}
	msgID, err := protocol.SendVoice(ctx, sess, ln, chatID, threadType, normalized, 0)
	if err != nil {
		return err
	}
	c.cacheOutboundMedia(msgID, "voice", filePath, "")
	return nil
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
// Mentions/styles are authored against the full rendered text, but Zalo expects
// offsets relative to each outgoing message body. For multi-chunk sends we map
// full-text UTF-16 offsets into chunk-local offsets and drop only spans that
// straddle a chunk boundary.
func (c *Channel) sendChunkedText(ctx context.Context, sess *protocol.Session, chatID string, threadType protocol.ThreadType, text string, quote *protocol.SendMessageQuote, allMentions []pkgproto.Mention, allStyles []common.Style) error {
	chunks := channels.ChunkMarkdown(text, maxTextLength)
	ranges := chunkRangesUTF16(text, chunks)
	chunkMentions, droppedMentions := mentionsByChunk(allMentions, ranges)
	chunkStyles, droppedStyles := stylesByChunk(allStyles, ranges)
	if len(chunks) > 1 && droppedMentions > 0 {
		slog.Warn("zalo_personal.mention.dropped_multichunk",
			"chat_id", chatID,
			"mentions_total", len(allMentions),
			"kept", countMentions(chunkMentions),
			"dropped", droppedMentions,
			"chunks", len(chunks))
	}
	if len(chunks) > 1 && droppedStyles > 0 {
		slog.Warn("zalo_personal.style.dropped_multichunk",
			"chat_id", chatID,
			"styles_total", len(allStyles),
			"kept", countStyles(chunkStyles),
			"dropped", droppedStyles,
			"chunks", len(chunks),
			"text_preview", previewText(text, 80))
	}
	for i, chunk := range chunks {
		var q *protocol.SendMessageQuote
		if i == 0 {
			q = quote
		}
		msgID, err := c.sendChunkWithFallbacks(ctx, sess, chatID, threadType, chunk, q, chunkMentions[i], chunkStyles[i])
		if err != nil {
			return err
		}
		c.recordOutboundMessage(msgID, previewText(chunk, outboundPreviewMax))
	}
	return nil
}

// sendChunkWithFallbacks: ErrMentionRejected → retry without mentions;
// ErrQuoteRejected → retry without quote.
func (c *Channel) sendChunkWithFallbacks(ctx context.Context, sess *protocol.Session, chatID string, threadType protocol.ThreadType, text string, quote *protocol.SendMessageQuote, ms []pkgproto.Mention, styles []common.Style) (string, error) {
	opts := protocol.SendOptions{Text: text, Quote: quote, Mentions: ms, Styles: styles}
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

type chunkRange struct {
	startUTF16 int
	lenUTF16   int
}

func chunkRangesUTF16(text string, chunks []string) []chunkRange {
	ranges := make([]chunkRange, len(chunks))
	searchFrom := 0
	fallbackStart := 0
	for i, chunk := range chunks {
		if chunk == "" {
			ranges[i] = chunkRange{startUTF16: fallbackStart}
			continue
		}
		if searchFrom <= len(text) {
			if rel := strings.Index(text[searchFrom:], chunk); rel >= 0 {
				byteStart := searchFrom + rel
				ranges[i] = chunkRange{
					startUTF16: pkgproto.UTF16Len(text[:byteStart]),
					lenUTF16:   pkgproto.UTF16Len(chunk),
				}
				searchFrom = byteStart + len(chunk)
				fallbackStart = ranges[i].startUTF16 + ranges[i].lenUTF16
				continue
			}
		}
		// Fallback for chunks that ChunkMarkdown repaired, e.g. force-split code
		// fences. Those chunks generally have no native styles, but keep progress
		// monotonic so later chunks can still be localized.
		chunkLen := pkgproto.UTF16Len(chunk)
		ranges[i] = chunkRange{startUTF16: fallbackStart, lenUTF16: chunkLen}
		fallbackStart += chunkLen
	}
	return ranges
}

func mentionsByChunk(all []pkgproto.Mention, ranges []chunkRange) ([][]pkgproto.Mention, int) {
	out := make([][]pkgproto.Mention, len(ranges))
	dropped := 0
	for _, m := range all {
		start := m.Position
		end := m.Position + m.Length
		if start < 0 || m.Length <= 0 {
			dropped++
			continue
		}
		chunkIdx := containingChunk(start, end, ranges)
		if chunkIdx < 0 {
			dropped++
			continue
		}
		local := m
		local.Position = start - ranges[chunkIdx].startUTF16
		out[chunkIdx] = append(out[chunkIdx], local)
	}
	return out, dropped
}

func stylesByChunk(all []common.Style, ranges []chunkRange) ([][]common.Style, int) {
	out := make([][]common.Style, len(ranges))
	dropped := 0
	for _, s := range all {
		start := s.Start
		end := s.Start + s.Len
		if start < 0 || s.Len <= 0 {
			dropped++
			continue
		}
		chunkIdx := containingChunk(start, end, ranges)
		if chunkIdx < 0 {
			dropped++
			continue
		}
		local := s
		local.Start = start - ranges[chunkIdx].startUTF16
		out[chunkIdx] = append(out[chunkIdx], local)
	}
	return out, dropped
}

func containingChunk(start, end int, ranges []chunkRange) int {
	for i, r := range ranges {
		rangeEnd := r.startUTF16 + r.lenUTF16
		if start >= r.startUTF16 && end <= rangeEnd {
			return i
		}
	}
	return -1
}

func countMentions(chunks [][]pkgproto.Mention) int {
	n := 0
	for _, chunk := range chunks {
		n += len(chunk)
	}
	return n
}

func countStyles(chunks [][]common.Style) int {
	n := 0
	for _, chunk := range chunks {
		n += len(chunk)
	}
	return n
}
