package personal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/channels/media"
	"github.com/nextlevelbuilder/goclaw/internal/channels/typing"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// First match wins; Zalo's quote-reply frame puts the user's new text in `title`.
var quotedReplyTextFields = []string{"title", "text", "msg", "content", "body", "description"}

func extractTextFromRawContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	for _, key := range quotedReplyTextFields {
		val, ok := obj[key]
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(val, &s); err != nil {
			continue
		}
		s = strings.TrimSpace(s)
		if s != "" {
			return s
		}
	}
	return ""
}

// buildQuoteMetadata returns the two metadata keys that carry an inbound
// TQuote downstream: reply_to_message_id (the quoted message's global ID, for
// gateway routing) and reply_to_quote_payload (the full JSON-serialized TQuote
type quoteOwnerCtx struct {
	senderUID   string
	botUID      string
	resolveName func(uid string) string
}

// quoteOwnerCtxFor builds the attribution context for a given inbound sender.
// botUID resolves from the live session (empty during pre-auth — acceptable).
func (c *Channel) quoteOwnerCtxFor(senderUID string, resolveName func(uid string) string) quoteOwnerCtx {
	var botUID string
	if sess := c.session(); sess != nil {
		botUID = sess.UID
	}
	return quoteOwnerCtx{senderUID: senderUID, botUID: botUID, resolveName: resolveName}
}

// groupNameResolver returns a func that maps a UID to a display name using the
// channel's recent group-history entries. Returns nil when no history is
// available so the caller falls back to a generic "another participant" label.
func (c *Channel) groupNameResolver(threadID string) func(uid string) string {
	gh := c.GroupHistory()
	if gh == nil {
		return nil
	}
	return func(uid string) string {
		for _, entry := range gh.GetEntries(threadID) {
			if entry.SenderID == uid && entry.Sender != "" {
				return entry.Sender
			}
		}
		return ""
	}
}

// quoteContextPrefix returns a "[Replying to ...]" line attributing the quoted
// message's author so the agent knows who originated it (bot / sender's own
// earlier message / third party in a group). Falls through q.Msg →
// caption-from-attach → media-type placeholder for body. When mediaAttached is
// true, the body (caption) is rendered on a separate line as quoted-message
// text so the LLM doesn't conflate it with the attached image's content.
func quoteContextPrefix(raw json.RawMessage, owner quoteOwnerCtx, mediaAttached bool) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var q protocol.TQuote
	if err := json.Unmarshal(raw, &q); err != nil {
		preview := string(raw)
		if len(preview) > 300 {
			preview = preview[:300]
		}
		slog.Warn("zalo_personal.quote.parse_failed", "err", err, "raw_preview", preview)
		return ""
	}
	body := strings.TrimSpace(q.Msg)
	if body == "" {
		body = extractAttachBody(q.Attach)
	}
	noun := mediaNoun(q.CliMsgType)
	who := whoAuthored(q.OwnerID.String(), owner)
	return formatReplyingTo(who, noun, body, mediaAttached)
}

// attachMediaURLFields lists the JSON keys Zalo uses inside TQuote.Attach to
// point at the original media file. Order matters — prefer the highest-quality
// available so the agent's vision/regen uses the best source.
var attachMediaURLFields = []string{"hdUrl", "oriUrl", "href", "normalUrl", "thumbUrl", "thumb"}

// extractQuoteMedia downloads the media file referenced inside TQuote.Attach
// when the quoted message has one (image / video / file). Returns the local
// file path + agent-facing <media:*> tag, or empty when no media URL extractable.
//
// Without this, when a user quotes one of the bot's earlier images saying
// "fix this", the agent only sees "[Replying to your image]" and has to guess
// the reference from session memory — frequently hallucinating an old image_id
// that no longer resolves.
func extractQuoteMedia(rawQuote json.RawMessage) (string, string) {
	if len(rawQuote) == 0 || string(rawQuote) == "null" {
		return "", ""
	}
	var q protocol.TQuote
	if json.Unmarshal(rawQuote, &q) != nil {
		return "", ""
	}
	attach := strings.TrimSpace(q.Attach)
	if attach == "" || attach == "null" {
		return "", ""
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal([]byte(attach), &obj) != nil {
		return "", ""
	}
	var url string
	for _, key := range attachMediaURLFields {
		val, ok := obj[key]
		if !ok {
			continue
		}
		var s string
		if json.Unmarshal(val, &s) != nil {
			continue
		}
		if s = strings.TrimSpace(s); s != "" {
			url = s
			break
		}
	}
	if url == "" {
		return "", ""
	}
	filePath, err := downloadFile(context.Background(), url)
	if err != nil {
		slog.Warn("zalo_personal.quote.media_download_failed", "url", url, "err", err)
		return "", ""
	}
	mimeType := media.DetectMIMEType(filePath)
	mediaKind := media.MediaKindFromMime(mimeType)
	// Force image kind for known image-type quotes when MIME sniff fails.
	if mediaKind != media.TypeImage && q.CliMsgType == 2 {
		mediaKind = media.TypeImage
	}
	tag := media.BuildMediaTags([]media.MediaInfo{{
		Type:        mediaKind,
		FilePath:    filePath,
		ContentType: mimeType,
	}})
	return filePath, tag
}

// whoAuthored returns a human-readable phrase for who wrote the quoted message:
//   - "your"                              → bot's earlier message (agent is "you")
//   - "their own"                         → current sender quoted themselves
//   - "<name>'s"                          → third party in a group (name resolved)
//   - "another participant's"             → third party, name unresolvable
func whoAuthored(ownerUID string, ctx quoteOwnerCtx) string {
	if ownerUID == "" || (ctx.botUID == "" && ctx.senderUID == "" && ctx.resolveName == nil) {
		return ""
	}
	switch ownerUID {
	case ctx.botUID:
		return "your"
	case ctx.senderUID:
		return "their own"
	}
	if ctx.resolveName != nil {
		if name := strings.TrimSpace(ctx.resolveName(ownerUID)); name != "" {
			return name + "'s"
		}
	}
	return "another participant's"
}

// formatReplyingTo composes the prefix line.
//   - who:  "your" | "their own" | "<name>'s" | "another participant's" | ""
//   - noun: bare media noun ("image", "sticker", ...) or ""
//   - body: actual text body or caption from attach
//   - mediaAttached: true when the quoted media file is also attached to the
//     current turn. In that case caption is emitted on a separate "[Quoted
//     caption: ...]" line so the LLM treats it as the sender's text rather
//     than as a description of the image content (prevents hallucinations
//     like "running in rain" when the image is a static portrait).
func formatReplyingTo(who, noun, body string, mediaAttached bool) string {
	if mediaAttached && noun != "" {
		var head string
		if who != "" {
			head = fmt.Sprintf("[Replying to %s %s]\n", who, noun)
		} else {
			head = fmt.Sprintf("[Replying to %s %s]\n", mediaArticle(noun), noun)
		}
		if body != "" {
			head += fmt.Sprintf("[Quoted caption: %q]\n", body)
		}
		return head
	}
	switch {
	case body != "" && noun != "" && who != "":
		return fmt.Sprintf("[Replying to %s %s: %q]\n", who, noun, body)
	case body != "" && noun != "":
		return fmt.Sprintf("[Replying to %s %s: %q]\n", mediaArticle(noun), noun, body)
	case body != "" && who != "":
		return fmt.Sprintf("[Replying to %s message: %q]\n", who, body)
	case body != "":
		return fmt.Sprintf("[Replying to message: %q]\n", body)
	case noun != "" && who != "":
		return fmt.Sprintf("[Replying to %s %s]\n", who, noun)
	case noun != "":
		return fmt.Sprintf("[Replying to %s %s]\n", mediaArticle(noun), noun)
	}
	return ""
}

// mediaNoun returns the bare noun for a quoted message's media type so the
// caller can compose with an article or a possessive. Empty for text/unknown.
func mediaNoun(cliMsgType int) string {
	switch cliMsgType {
	case 2:
		return "image"
	case 3:
		return "sticker"
	case 5:
		return "voice message"
	case 19:
		return "checklist"
	case 1:
		return ""
	default:
		if cliMsgType > 0 {
			return "media message"
		}
		return ""
	}
}

func mediaArticle(noun string) string {
	if noun == "" {
		return ""
	}
	switch noun[0] {
	case 'a', 'e', 'i', 'o', 'u':
		return "an"
	}
	return "a"
}

func extractAttachBody(attach string) string {
	attach = strings.TrimSpace(attach)
	if attach == "" || attach == "null" {
		return ""
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(attach), &obj); err != nil {
		return ""
	}
	for _, key := range quotedReplyTextFields {
		val, ok := obj[key]
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(val, &s); err != nil {
			continue
		}
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}

// Returns nil when no quote, when the quote raw JSON fails to parse against
// our TQuote shape, or when re-marshal fails. Parse failures are warn-logged
// with the raw payload preview so the schema mismatch is visible — the rest
// of the inbound message (text, mentions, media) still flows through.
func buildQuoteMetadata(raw json.RawMessage) map[string]string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var q protocol.TQuote
	if err := json.Unmarshal(raw, &q); err != nil {
		preview := string(raw)
		if len(preview) > 300 {
			preview = preview[:300]
		}
		slog.Warn("zalo_personal.quote.parse_failed", "err", err, "raw_preview", preview)
		return nil
	}
	payload, err := json.Marshal(&q)
	if err != nil {
		slog.Warn("zalo_personal.quote.marshal_failed", "err", err, "global_msg_id", q.GlobalMsgIDString())
		return nil
	}
	return map[string]string{
		"reply_to_message_id":    q.GlobalMsgIDString(),
		"reply_to_quote_payload": string(payload),
	}
}

// inboundMsgTypeToCli inverts classifyQuoteMsgType so we can synthesize a
// TQuote.CliMsgType from the inbound TMessage.MsgType string. Unknown types
// default to text (1) since classifyQuoteMsgType's text branch is the most
// permissive on the Zalo /quote endpoint.
func inboundMsgTypeToCli(msgType string) int {
	switch strings.ToLower(msgType) {
	case "chat.photo", "photo":
		return 2
	case "chat.sticker", "sticker":
		return 3
	case "chat.voice", "voice":
		return 5
	case "chat.todo", "todo":
		return 19
	}
	return 1
}

// buildSelfQuoteMetadata synthesizes reply_to_quote_payload from the inbound
// MESSAGE ITSELF (not from any quote it carries). Used when QuoteUserMessage
// is enabled and the user sent a plain message — without this the bot's
// outbound Send sees no quote payload and falls through to plain send, so
// the "Quote user message" toggle silently no-ops for normal conversation.
func buildSelfQuoteMetadata(data *protocol.TMessage, senderID string) map[string]string {
	if data == nil || data.MsgID == "" {
		return nil
	}
	msgBody := data.Content.Text()
	attach := ""
	if msgBody == "" && data.Content.Raw != nil {
		attach = string(data.Content.Raw)
	}
	q := protocol.TQuote{
		OwnerID:     json.Number(senderID),
		CliMsgID:    data.CliMsgID,
		GlobalMsgID: json.Number(data.MsgID),
		CliMsgType:  inboundMsgTypeToCli(data.MsgType),
		TS:          json.Number(data.TS),
		Msg:         msgBody,
		Attach:      attach,
	}
	payload, err := json.Marshal(&q)
	if err != nil {
		slog.Warn("zalo_personal.quote.self_marshal_failed", "err", err, "msg_id", data.MsgID)
		return nil
	}
	return map[string]string{
		"reply_to_message_id":    data.MsgID,
		"reply_to_quote_payload": string(payload),
	}
}

func (c *Channel) handleMessage(msg protocol.Message) {
	if msg.IsSelf() {
		return
	}

	switch m := msg.(type) {
	case protocol.UserMessage:
		c.handleDM(m)
	case protocol.GroupMessage:
		c.handleGroupMessage(m)
	}
}

func (c *Channel) handleDM(msg protocol.UserMessage) {
	ctx := context.Background()
	ctx = store.WithTenantID(ctx, c.TenantID())
	senderID := msg.Data.UIDFrom
	threadID := msg.ThreadID()

	content, media := extractContentAndMedia(msg.Data.Content)
	if content == "" {
		return
	}

	if !c.checkDMPolicy(ctx, senderID, threadID) {
		return
	}

	quotedPath, quotedTag := extractQuoteMedia(msg.Data.Quote)
	if prefix := quoteContextPrefix(msg.Data.Quote, c.quoteOwnerCtxFor(senderID, nil), quotedPath != ""); prefix != "" {
		content = prefix + content
	}
	if quotedPath != "" {
		content = content + "\n" + quotedTag
		media = append(media, quotedPath)
	}
	senderName := msg.Data.DName
	if senderName != "" {
		content = fmt.Sprintf("[From: %s]\n%s", senderName, content)
	}

	slog.Debug("zalo_personal DM received",
		"sender", senderID,
		"dname", senderName,
		"thread", threadID,
		"preview", channels.Truncate(content, 50),
	)

	c.startTyping(threadID, protocol.ThreadTypeUser)

	// Collect contact for DM messages.
	if cc := c.ContactCollector(); cc != nil {
		cc.EnsureContact(ctx, c.Type(), c.Name(), senderID, senderID, senderName, "", "direct", "user", "", "")
	}

	metadata := map[string]string{
		"message_id":   msg.Data.MsgID,
		"cli_msg_id":   msg.Data.CliMsgID.String(),
		"platform":     channels.TypeZaloPersonal,
		"display_name": channels.SanitizeDisplayName(senderName),
	}
	if c.quoteUserMessageEnabled() {
		if qm := buildSelfQuoteMetadata(&msg.Data, senderID); qm != nil {
			maps.Copy(metadata, qm)
		}
	}
	c.HandleMessage(senderID, threadID, content, media, metadata, "direct")
}

func (c *Channel) handleGroupMessage(msg protocol.GroupMessage) {
	ctx := context.Background()
	ctx = store.WithTenantID(ctx, c.TenantID())
	senderID := msg.Data.UIDFrom
	threadID := msg.ThreadID()

	content, media := extractContentAndMedia(msg.Data.Content)
	if content == "" {
		return
	}

	// Step 1: enforce access policy (allowlist/pairing). Hard reject — don't record history.
	if !c.checkGroupPolicy(ctx, senderID, threadID) {
		return
	}

	senderName := msg.Data.DName
	if senderName == "" {
		senderName = senderID
	}

	// Step 2: @mention gating — record non-mentioned messages in history and return.
	if c.RequireMention() {
		wasMentioned := c.checkBotMentioned(msg.Data.Mentions)
		if !wasMentioned {
			c.GroupHistory().Record(threadID, channels.HistoryEntry{
				Sender:    senderName,
				SenderID:  senderID,
				Body:      content,
				Media:     media,
				Timestamp: time.Now(),
				MessageID: msg.Data.MsgID,
			}, c.HistoryLimit())

			// Collect contact even when bot is not mentioned (cache prevents DB spam).
			if cc := c.ContactCollector(); cc != nil {
				cc.EnsureContact(ctx, c.Type(), c.Name(), senderID, senderID, senderName, "", "group", "user", "", "")
			}

			slog.Debug("zalo_personal group message recorded (no mention)",
				"group_id", threadID,
				"sender", senderName,
			)
			return
		}
	}

	slog.Debug("zalo_personal group message received",
		"sender", senderID,
		"group", threadID,
		"preview", channels.Truncate(content, 50),
	)

	quotedPath, quotedTag := extractQuoteMedia(msg.Data.Quote)
	if prefix := quoteContextPrefix(msg.Data.Quote, c.quoteOwnerCtxFor(senderID, c.groupNameResolver(threadID)), quotedPath != ""); prefix != "" {
		content = prefix + content
	}
	if quotedPath != "" {
		content = content + "\n" + quotedTag
		media = append(media, quotedPath)
	}
	annotated := fmt.Sprintf("[From: %s]\n%s", senderName, content)
	finalContent := annotated
	if c.HistoryLimit() > 0 {
		finalContent = c.GroupHistory().BuildContext(threadID, annotated, c.HistoryLimit())
	}

	c.startTyping(threadID, protocol.ThreadTypeGroup)

	// Collect media from pending history entries (images sent before this @mention).
	// Must come after BuildContext — CollectMedia nulls out Media fields to prevent double-cleanup.
	histMedia := c.GroupHistory().CollectMedia(threadID)
	allMedia := append(histMedia, media...)

	// Collect contact for group-mentioned messages.
	if cc := c.ContactCollector(); cc != nil {
		cc.EnsureContact(ctx, c.Type(), c.Name(), senderID, senderID, senderName, "", "group", "user", "", "")
	}

	metadata := map[string]string{
		"message_id":   msg.Data.MsgID,
		"cli_msg_id":   msg.Data.CliMsgID.String(),
		"platform":     channels.TypeZaloPersonal,
		"group_id":     threadID,
		"display_name": channels.SanitizeDisplayName(senderName),
	}
	if c.quoteUserMessageEnabled() {
		if qm := buildSelfQuoteMetadata(&msg.Data.TMessage, senderID); qm != nil {
			maps.Copy(metadata, qm)
		}
	}
	c.HandleMessage(senderID, threadID, finalContent, allMedia, metadata, "group")

	// Clear pending history after sending to agent (matches Telegram/Discord/Slack/Feishu pattern).
	c.GroupHistory().Clear(threadID)
}

// startTyping starts a typing indicator with keepalive for the given thread.
// Zalo typing expires after ~5s, so keepalive fires every 3s.
func (c *Channel) startTyping(threadID string, threadType protocol.ThreadType) {
	sess := c.session()
	if sess == nil {
		// No authenticated session (e.g. brief window during reconnect, or in tests
		// that exercise handler logic without a live Zalo connection). Typing is
		// best-effort UX — skip rather than panic in the goroutine started below.
		return
	}
	ctrl := typing.New(typing.Options{
		MaxDuration:       60 * time.Second,
		KeepaliveInterval: 4 * time.Second,
		StartFn: func() error {
			return protocol.SendTypingEvent(context.Background(), sess, threadID, threadType)
		},
	})
	if prev, ok := c.typingCtrls.Load(threadID); ok {
		if ctrl, ok := prev.(*typing.Controller); ok {
			ctrl.Stop()
		}
	}
	c.typingCtrls.Store(threadID, ctrl)
	ctrl.Start()
}

// Ordering: any href attachment (image/video/audio/file) must beat the title-text
// probe — Zalo's media-with-caption shape `{title, href}` collides with quote-reply
// shape `{title, params}` on `title`.
func extractContentAndMedia(content protocol.Content) (string, []string) {
	if text := content.Text(); text != "" {
		return text, nil
	}
	if att := content.ParseAttachment(); att != nil && att.Href != "" {
		return extractAttachment(content, att)
	}
	if text := extractTextFromRawContent(content.Raw); text != "" {
		return text, nil
	}
	if len(content.Raw) > 0 {
		slog.Warn("zalo_personal: unparseable content shape (message dropped)",
			"content_raw", string(content.Raw))
	}
	return "", nil
}

func extractAttachment(content protocol.Content, att *protocol.Attachment) (string, []string) {
	filePath, err := downloadFile(context.Background(), att.Href)
	if err != nil {
		slog.Warn("zalo_personal: failed to download attachment", "url", att.Href, "error", err)
		if text := content.AttachmentText(); text != "" {
			return text, nil
		}
		return "", nil
	}

	mimeType := media.DetectMIMEType(filePath)
	mediaKind := media.MediaKindFromMime(mimeType)
	if mediaKind != media.TypeImage && att.IsImage() {
		mediaKind = media.TypeImage
	}

	tag := media.BuildMediaTags([]media.MediaInfo{{
		Type:        mediaKind,
		FilePath:    filePath,
		ContentType: mimeType,
		FileName:    att.Title,
	}})

	// att.Title carries the user's caption (not a filename) in image+caption frames.
	if caption := strings.TrimSpace(att.Title); caption != "" {
		return caption + "\n" + tag, []string{filePath}
	}
	return tag, []string{filePath}
}

const maxMediaBytes = 20 * 1024 * 1024 // 20MB (matches Telegram default)

// downloadFile downloads a URL to a temp file and returns the local path.
// Validates against SSRF and enforces timeout and size limits.
func downloadFile(ctx context.Context, fileURL string) (string, error) {
	if err := tools.CheckSSRF(fileURL); err != nil {
		return "", fmt.Errorf("ssrf check: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download status %d", resp.StatusCode)
	}

	// Strip query params before extracting extension.
	path := fileURL
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	ext := filepath.Ext(path)
	if ext == "" || len(ext) > 5 {
		ext = ".bin"
	}

	tmpFile, err := os.CreateTemp("", "goclaw_zca_*"+ext)
	if err != nil {
		return "", fmt.Errorf("create temp: %w", err)
	}
	defer tmpFile.Close()

	written, err := io.Copy(tmpFile, io.LimitReader(resp.Body, maxMediaBytes+1))
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("save: %w", err)
	}
	if written > maxMediaBytes {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("file too large: %d bytes (max %d)", written, maxMediaBytes)
	}

	return tmpFile.Name(), nil
}
