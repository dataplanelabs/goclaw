package personal

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
	"github.com/nextlevelbuilder/goclaw/internal/config"
)

var _ channels.DMQuoteChannel = (*Channel)(nil)

func TestExtractTextFromRawContent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"plain object with title", `{"title":"review quote nay","params":{"id":"x"}}`, "review quote nay"},
		{"text field", `{"text":"hello there"}`, "hello there"},
		{"msg field", `{"msg":"hi","other":1}`, "hi"},
		{"first match wins (title beats text)", `{"text":"second","title":"first"}`, "first"},
		{"whitespace trimmed", `{"title":"   spaced   "}`, "spaced"},
		{"no text field", `{"href":"https://x.com/y.jpg","width":100}`, ""},
		{"invalid json", `not-json`, ""},
		{"null", `null`, ""},
		{"non-object array", `[1,2,3]`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := extractTextFromRawContent(json.RawMessage(tc.raw))
			if got != tc.want {
				t.Errorf("extractTextFromRawContent(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestExtractContentAndMedia_QuotedReplyShape(t *testing.T) {
	t.Parallel()
	var c protocol.Content
	if err := json.Unmarshal([]byte(`{"title":"review quote nay","params":{"id":"x"}}`), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, media := extractContentAndMedia(c)
	if got != "review quote nay" {
		t.Errorf("content = %q, want 'review quote nay'", got)
	}
	if media != nil {
		t.Errorf("media = %v, want nil", media)
	}
}

// Regression for PR #14: title-probe must not pre-empt attachment when href is present.
// Uses TEST-NET-1 to force downloadFile to fail; fallback "[User sent an image: ...]"
// proves the attachment branch was taken (buggy probe would return bare title).
func TestExtractContentAndMedia_ImageWithCaption_PrefersAttachment(t *testing.T) {
	t.Parallel()
	var c protocol.Content
	raw := `{"title":"tao poster voi buc hinh nay","href":"https://192.0.2.1/photo.jpg","thumb":"https://192.0.2.1/thumb.jpg"}`
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, _ := extractContentAndMedia(c)
	// Buggy behavior would return just "tao poster voi buc hinh nay" via the title
	// probe. Fixed behavior routes through extractAttachment → on download fail,
	// AttachmentText returns "[User sent an image: <title>]".
	want := "[User sent an image: tao poster voi buc hinh nay]"
	if got != want {
		t.Errorf("content = %q, want %q (ordering regression: title probe won over attachment path)", got, want)
	}
}

func TestChannel_QuoteInboundOnDM_HonorsConfig(t *testing.T) {
	t.Parallel()
	off, on := false, true
	cases := []struct {
		name string
		ptr  *bool
		want bool
	}{
		{"unset_defaults_off", nil, false},
		{"explicit_true", &on, true},
		{"explicit_false", &off, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := &Channel{config: config.ZaloPersonalConfig{QuoteUserMessage: tc.ptr}}
			if got := c.QuoteInboundOnDM(); got != tc.want {
				t.Errorf("QuoteInboundOnDM = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBuildQuoteMetadata_NilReturnsNil(t *testing.T) {
	t.Parallel()
	if got := buildQuoteMetadata(nil); got != nil {
		t.Errorf("buildQuoteMetadata(nil) = %v, want nil", got)
	}
}

// TestBuildQuoteMetadata_InvalidPropertyExtReturnsNil: when PropertyExt
// contains invalid JSON, json.Marshal fails — the helper must return nil
// emptyOwner pretends the quote has no owner UID — exercises the no-attribution path.
var emptyOwner = quoteOwnerCtx{}

func TestExtractQuoteMedia_FieldProbe(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		attach  string
		wantURL string
	}{
		{"hdUrl wins", `{"hdUrl":"https://x/a.jpg","thumbUrl":"https://x/t.jpg"}`, "https://x/a.jpg"},
		{"oriUrl fallback", `{"oriUrl":"https://x/o.jpg"}`, "https://x/o.jpg"},
		{"href fallback", `{"href":"https://x/h.jpg"}`, "https://x/h.jpg"},
		{"thumbUrl last", `{"thumbUrl":"https://x/t.jpg"}`, "https://x/t.jpg"},
		{"no URL field", `{"width":1080,"height":720}`, ""},
		{"empty attach", "", ""},
		{"invalid json", "{not-json}", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw, _ := json.Marshal(&protocol.TQuote{Attach: tc.attach})
			path, tag := extractQuoteMedia(raw)
			// We can't actually download in unit tests; verify that "no URL" cases
			// return empty without attempting download. URL-bearing cases would
			// hit the network → not asserted here, covered by the live smoke test.
			if tc.wantURL == "" {
				if path != "" || tag != "" {
					t.Errorf("expected no extraction, got path=%q tag=%q", path, tag)
				}
			}
		})
	}
}

func TestQuoteContextPrefix_FallbackToAttach(t *testing.T) {
	t.Parallel()
	q := &protocol.TQuote{Msg: "", Attach: `{"title":"recovered from attach"}`}
	raw, _ := json.Marshal(q)
	if got := quoteContextPrefix(raw, emptyOwner, false); got != "[Replying to message: \"recovered from attach\"]\n" {
		t.Errorf("got %q", got)
	}
}

func TestQuoteContextPrefix_MediaQuotes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		cliMsgType int
		msg        string
		attach     string
		want       string
	}{
		{"image with caption", 2, "", `{"title":"holiday photo"}`, "[Replying to an image: \"holiday photo\"]\n"},
		{"image no caption", 2, "", "", "[Replying to an image]\n"},
		{"sticker no caption", 3, "", "", "[Replying to a sticker]\n"},
		{"voice no caption", 5, "", "", "[Replying to a voice message]\n"},
		{"checklist no caption", 19, "", "", "[Replying to a checklist]\n"},
		{"unknown media type", 99, "", "", "[Replying to a media message]\n"},
		{"text quote on image type", 2, "actual text body", "", "[Replying to an image: \"actual text body\"]\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw, _ := json.Marshal(&protocol.TQuote{
				CliMsgType: tc.cliMsgType, Msg: tc.msg, Attach: tc.attach,
			})
			if got := quoteContextPrefix(raw, emptyOwner, false); got != tc.want {
				t.Errorf("got %q\nwant %q", got, tc.want)
			}
		})
	}
}

// When the quoted media file is also attached as <media:image>, the caption
// must be emitted on a separate "[Quoted caption: ...]" line — otherwise the
// LLM treats the caption as a description of the image and hallucinates
// attributes not present in the actual picture (trace 019e5666 regression).
func TestQuoteContextPrefix_MediaAttached_SeparatesCaption(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		quote *protocol.TQuote
		ctx   quoteOwnerCtx
		want  string
	}{
		{
			name:  "their own image with caption",
			quote: &protocol.TQuote{OwnerID: json.Number("200"), CliMsgType: 2, Msg: "tạo ảnh buồn kiểu chạy"},
			ctx:   quoteOwnerCtx{senderUID: "200", botUID: "100"},
			want:  "[Replying to their own image]\n[Quoted caption: \"tạo ảnh buồn kiểu chạy\"]\n",
		},
		{
			name:  "your image no caption (no quoted-caption line)",
			quote: &protocol.TQuote{OwnerID: json.Number("100"), CliMsgType: 2},
			ctx:   quoteOwnerCtx{senderUID: "200", botUID: "100"},
			want:  "[Replying to your image]\n",
		},
		{
			name:  "no owner ctx image with caption from attach",
			quote: &protocol.TQuote{CliMsgType: 2, Attach: `{"title":"holiday photo"}`},
			ctx:   emptyOwner,
			want:  "[Replying to an image]\n[Quoted caption: \"holiday photo\"]\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw, _ := json.Marshal(tc.quote)
			if got := quoteContextPrefix(raw, tc.ctx, true); got != tc.want {
				t.Errorf("got %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestQuoteContextPrefix_OwnerAttribution(t *testing.T) {
	t.Parallel()
	mk := func(ownerUID, msg string) json.RawMessage {
		b, _ := json.Marshal(&protocol.TQuote{OwnerID: json.Number(ownerUID), Msg: msg})
		return b
	}
	resolver := func(uid string) string {
		if uid == "999" {
			return "Mai Hà Lan"
		}
		return ""
	}
	cases := []struct {
		name string
		raw  json.RawMessage
		ctx  quoteOwnerCtx
		want string
	}{
		{
			name: "bot's own message",
			raw:  mk("100", "I helped earlier"),
			ctx:  quoteOwnerCtx{senderUID: "200", botUID: "100"},
			want: "[Replying to your message: \"I helped earlier\"]\n",
		},
		{
			name: "sender replied to their own message",
			raw:  mk("200", "earlier user line"),
			ctx:  quoteOwnerCtx{senderUID: "200", botUID: "100"},
			want: "[Replying to their own message: \"earlier user line\"]\n",
		},
		{
			name: "third party with resolved name",
			raw:  mk("999", "hello from group member"),
			ctx:  quoteOwnerCtx{senderUID: "200", botUID: "100", resolveName: resolver},
			want: "[Replying to Mai Hà Lan's message: \"hello from group member\"]\n",
		},
		{
			name: "third party without resolver",
			raw:  mk("888", "anonymous group line"),
			ctx:  quoteOwnerCtx{senderUID: "200", botUID: "100"},
			want: "[Replying to another participant's message: \"anonymous group line\"]\n",
		},
		{
			name: "bot quoted an image (no caption)",
			raw: func() json.RawMessage {
				b, _ := json.Marshal(&protocol.TQuote{OwnerID: json.Number("100"), CliMsgType: 2})
				return b
			}(),
			ctx:  quoteOwnerCtx{senderUID: "200", botUID: "100"},
			want: "[Replying to your image]\n",
		},
		{
			name: "third party image with caption",
			raw: func() json.RawMessage {
				b, _ := json.Marshal(&protocol.TQuote{
					OwnerID: json.Number("999"), CliMsgType: 2,
					Attach: `{"title":"holiday photo"}`,
				})
				return b
			}(),
			ctx:  quoteOwnerCtx{senderUID: "200", botUID: "100", resolveName: resolver},
			want: "[Replying to Mai Hà Lan's image: \"holiday photo\"]\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := quoteContextPrefix(tc.raw, tc.ctx, false); got != tc.want {
				t.Errorf("got %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestExtractAttachBody(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, attach, want string }{
		{"empty", "", ""},
		{"null", "null", ""},
		{"title", `{"title":"hello"}`, "hello"},
		{"msg", `{"msg":"hi"}`, "hi"},
		{"invalid json", "{not-json}", ""},
		{"no text key", `{"width":100,"height":200}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := extractAttachBody(tc.attach); got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

// (no half-stamp) so the outbound reply gracefully degrades to a plain send
// rather than carrying a stale reply_to_message_id with no payload.
func TestQuoteContextPrefix(t *testing.T) {
	t.Parallel()
	mk := func(msg string) json.RawMessage {
		b, _ := json.Marshal(&protocol.TQuote{Msg: msg})
		return b
	}
	long := strings.TrimSpace(strings.Repeat("lorem ipsum ", 50))
	cases := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{"empty", nil, ""},
		{"null", json.RawMessage("null"), ""},
		{"blank msg", mk("   "), ""},
		{"short", mk("hello world"), "[Replying to message: \"hello world\"]\n"},
		{"full body preserved (no truncation)", mk(long), fmt.Sprintf("[Replying to message: %q]\n", long)},
		{"invalid json", json.RawMessage("not-json"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := quoteContextPrefix(tc.raw, emptyOwner, false); got != tc.want {
				t.Errorf("got %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestHandleDM_QuoteContextInjectedIntoAgentInput(t *testing.T) {
	t.Parallel()
	ch, mb := newHandlerTestChannel(t)
	text := "follow-up question"
	quoted := "earlier bot reply being referenced"
	quote, _ := json.Marshal(&protocol.TQuote{
		OwnerID: json.Number("123"), GlobalMsgID: json.Number("9876543210"),
		Msg: quoted,
	})
	ch.handleDM(protocol.NewUserMessage("self-uid", protocol.TMessage{
		MsgID: "m-current", UIDFrom: "456", IDTo: "self-uid", DName: "Alice",
		Content: protocol.Content{String: &text},
		Quote:   quote,
	}))
	got := drainInbound(t, mb)
	if !strings.Contains(got.Content, "[Replying to") {
		t.Errorf("agent input missing quote prefix:\n%s", got.Content)
	}
	if !strings.Contains(got.Content, quoted) {
		t.Errorf("agent input missing quoted body:\n%s", got.Content)
	}
	if !strings.Contains(got.Content, text) {
		t.Errorf("agent input missing user's new text:\n%s", got.Content)
	}
}

func TestBuildQuoteMetadata_UnparseableQuoteReturnsNil(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"ownerId":"111","cliMsgId":"not-a-number"}`)
	if got := buildQuoteMetadata(raw); got != nil {
		t.Errorf("buildQuoteMetadata with unparseable quote should return nil, got %v", got)
	}
}

func TestBuildQuoteMetadata_RoundTrip(t *testing.T) {
	t.Parallel()
	propertyExt := json.RawMessage(`{"color":-16777216,"size":18}`)
	original := &protocol.TQuote{
		OwnerID:     json.Number("111"),
		CliMsgID:    json.Number("1709300000123"),
		GlobalMsgID: json.Number("9876543210"),
		CliMsgType:  1,
		TS:          json.Number("1709300000"),
		Msg:         "hello world",
		Attach:      `{"hdUrl":"x"}`,
		FromD:       "789",
		TTL:         0,
		PropertyExt: propertyExt,
	}
	rawQuote, _ := json.Marshal(original)

	meta := buildQuoteMetadata(rawQuote)
	if meta["reply_to_message_id"] != "9876543210" {
		t.Errorf("reply_to_message_id = %q, want 9876543210", meta["reply_to_message_id"])
	}
	if meta["reply_to_quote_payload"] == "" {
		t.Fatal("reply_to_quote_payload missing")
	}

	// Roundtrip: unmarshal the payload back into a TQuote, then check fields
	// match (including byte-equal PropertyExt).
	var back protocol.TQuote
	if err := json.Unmarshal([]byte(meta["reply_to_quote_payload"]), &back); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if back.OwnerID.String() != original.OwnerID.String() || back.Msg != original.Msg ||
		back.GlobalMsgID.String() != original.GlobalMsgID.String() ||
		back.CliMsgID.String() != original.CliMsgID.String() ||
		back.Attach != original.Attach || back.FromD != original.FromD {
		t.Errorf("roundtrip mismatch:\n got=%+v\nwant=%+v", back, original)
	}
	// PropertyExt is RawMessage — re-encode to canonical form to compare without
	// being fooled by whitespace differences (none expected here but safe).
	var origExt, backExt any
	_ = json.Unmarshal(original.PropertyExt, &origExt)
	_ = json.Unmarshal(back.PropertyExt, &backExt)
	origCanon, _ := json.Marshal(origExt)
	backCanon, _ := json.Marshal(backExt)
	if string(origCanon) != string(backCanon) {
		t.Errorf("PropertyExt roundtrip mismatch:\n got=%s\nwant=%s", backCanon, origCanon)
	}
}

// newHandlerTestChannel builds a Channel suitable for invoking handleDM/
// handleGroupMessage without an authenticated Zalo session. The typing
// indicator path runs in a goroutine and silently logs when no session is
// present — acceptable for handler-level metadata tests.
func newHandlerTestChannel(t *testing.T) (*Channel, *bus.MessageBus) {
	t.Helper()
	mb := bus.New()
	enabled := true
	ch, err := New(config.ZaloPersonalConfig{
		Enabled:          true,
		DMPolicy:         "open",
		GroupPolicy:      "open",
		QuoteUserMessage: &enabled,
	}, mb, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return ch, mb
}

// drainInbound waits up to 1s for a message to land on the bus.
func drainInbound(t *testing.T, mb *bus.MessageBus) bus.InboundMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	msg, ok := mb.ConsumeInbound(ctx)
	if !ok {
		t.Fatal("expected inbound message on bus, got none")
	}
	return msg
}

// Even when the inbound is itself a quote-reply, the bot's reply must quote
// the user's CURRENT message (not the message the user was quoting). Matches
// the "Quote user message" toggle's natural intent.
func TestHandleDM_StampsQuoteMetadata(t *testing.T) {
	t.Parallel()
	ch, mb := newHandlerTestChannel(t)
	text := "thanks!"
	quote, _ := json.Marshal(&protocol.TQuote{
		OwnerID:     json.Number("111"),
		GlobalMsgID: json.Number("9876543210"),
		CliMsgID:    json.Number("1709300000"),
		Msg:         "hello there",
		FromD:       "789",
	})

	ch.handleDM(protocol.NewUserMessage("self-uid", protocol.TMessage{
		MsgID:    "7858722000099",
		CliMsgID: json.Number("1700000000999"),
		UIDFrom:  "456",
		IDTo:     "self-uid",
		DName:    "Replier",
		TS:       "1700000001",
		Content:  protocol.Content{String: &text},
		Quote:    quote,
	}))

	got := drainInbound(t, mb)
	if got.Metadata["reply_to_message_id"] != "7858722000099" {
		t.Errorf("reply_to_message_id = %q, want 7858722000099 (inbound's own MsgID)", got.Metadata["reply_to_message_id"])
	}
	payload := got.Metadata["reply_to_quote_payload"]
	if payload == "" {
		t.Fatal("reply_to_quote_payload should be stamped")
	}
	var q protocol.TQuote
	if err := json.Unmarshal([]byte(payload), &q); err != nil {
		t.Fatalf("payload not valid TQuote JSON: %v", err)
	}
	if q.OwnerID.String() != "456" {
		t.Errorf("OwnerID = %q, want 456 (sender), not the quoted msg owner 111", q.OwnerID.String())
	}
	if q.Msg != "thanks!" {
		t.Errorf("Msg = %q, want 'thanks!' (inbound's text, not the quoted msg's 'hello there')", q.Msg)
	}
	if got.Metadata["message_id"] != "7858722000099" {
		t.Errorf("message_id = %q, want 7858722000099", got.Metadata["message_id"])
	}
}

// TestHandleDM_QuoteDisabledByConfig: when QuoteUserMessage is false (default),
// the handler must NOT stamp quote metadata even when the inbound carries a
// TQuote — bot replies stay plain regardless of user's quote action.
func TestHandleDM_QuoteDisabledByConfig(t *testing.T) {
	t.Parallel()
	mb := bus.New()
	disabled := false
	ch, err := New(config.ZaloPersonalConfig{
		Enabled: true, DMPolicy: "open", GroupPolicy: "open",
		QuoteUserMessage: &disabled,
	}, mb, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	text := "hi"
	quote, _ := json.Marshal(&protocol.TQuote{
		OwnerID: json.Number("111"), GlobalMsgID: json.Number("9876543210"), Msg: "x",
	})
	ch.handleDM(protocol.NewUserMessage("self-uid", protocol.TMessage{
		MsgID: "m1", UIDFrom: "456", IDTo: "self-uid",
		Content: protocol.Content{String: &text},
		Quote:   quote,
	}))

	got := drainInbound(t, mb)
	if _, ok := got.Metadata["reply_to_quote_payload"]; ok {
		t.Errorf("reply_to_quote_payload must not be stamped when QuoteUserMessage=false, got %v", got.Metadata)
	}
	if _, ok := got.Metadata["reply_to_message_id"]; ok {
		t.Errorf("reply_to_message_id must not be stamped when QuoteUserMessage=false, got %v", got.Metadata)
	}
}

// When QuoteUserMessage is enabled and the inbound has no quote, the channel
// synthesizes reply_to_quote_payload from the inbound itself so the bot's
// outbound reply can quote-bubble the user's plain message. Without this the
// toggle silently no-oped for normal chat (trace 019e56c8).
func TestHandleDM_NoQuoteSynthesizesSelfQuote(t *testing.T) {
	t.Parallel()
	ch, mb := newHandlerTestChannel(t)
	text := "plain hi"

	ch.handleDM(protocol.NewUserMessage("self-uid", protocol.TMessage{
		MsgID:    "7858722000001",
		CliMsgID: json.Number("1700000000001"),
		UIDFrom:  "456",
		IDTo:     "self-uid",
		TS:       "1700000000",
		MsgType:  "chat.text",
		Content:  protocol.Content{String: &text},
	}))

	got := drainInbound(t, mb)
	if got.Metadata["reply_to_message_id"] != "7858722000001" {
		t.Errorf("reply_to_message_id = %q, want 7858722000001 (inbound MsgID)", got.Metadata["reply_to_message_id"])
	}
	payload := got.Metadata["reply_to_quote_payload"]
	if payload == "" {
		t.Fatal("reply_to_quote_payload must be synthesized from inbound")
	}
	var q protocol.TQuote
	if err := json.Unmarshal([]byte(payload), &q); err != nil {
		t.Fatalf("payload not valid TQuote JSON: %v", err)
	}
	if q.OwnerID.String() != "456" {
		t.Errorf("OwnerID = %q, want 456 (sender)", q.OwnerID.String())
	}
	if q.GlobalMsgID.String() != "7858722000001" {
		t.Errorf("GlobalMsgID = %q, want 7858722000001", q.GlobalMsgID.String())
	}
	if q.Msg != "plain hi" {
		t.Errorf("Msg = %q, want plain hi", q.Msg)
	}
	if q.CliMsgType != 1 {
		t.Errorf("CliMsgType = %d, want 1 (text)", q.CliMsgType)
	}
}

// Same but when QuoteUserMessage is DISABLED — nothing should be stamped
// even for plain messages.
func TestHandleDM_NoQuoteQuoteDisabledStampsNothing(t *testing.T) {
	t.Parallel()
	mb := bus.New()
	disabled := false
	ch, err := New(config.ZaloPersonalConfig{
		Enabled:          true,
		DMPolicy:         "open",
		QuoteUserMessage: &disabled,
	}, mb, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	text := "plain hi"

	ch.handleDM(protocol.NewUserMessage("self-uid", protocol.TMessage{
		MsgID:   "7858722000001",
		UIDFrom: "456",
		IDTo:    "self-uid",
		Content: protocol.Content{String: &text},
	}))

	got := drainInbound(t, mb)
	if _, ok := got.Metadata["reply_to_message_id"]; ok {
		t.Errorf("reply_to_message_id must not be stamped when QuoteUserMessage=false, got %v", got.Metadata)
	}
	if _, ok := got.Metadata["reply_to_quote_payload"]; ok {
		t.Errorf("reply_to_quote_payload must not be stamped when QuoteUserMessage=false, got %v", got.Metadata)
	}
}

func TestHandleGroupMessage_StampsQuoteMetadata(t *testing.T) {
	t.Parallel()
	ch, mb := newHandlerTestChannel(t)
	// Group policy = open; require_mention defaults to true so we must mention
	// the bot for the message to reach the agent. Disable mention gating to
	// keep the test focused on quote-metadata propagation.
	ch.SetRequireMention(false)

	text := "group reply"
	quote, _ := json.Marshal(&protocol.TQuote{
		OwnerID:     json.Number("111"),
		GlobalMsgID: json.Number("9876543299"),
		Msg:         "original group msg",
	})

	ch.handleGroupMessage(protocol.NewGroupMessage("self-uid", protocol.TGroupMessage{
		TMessage: protocol.TMessage{
			MsgID:    "7858722000200",
			CliMsgID: json.Number("1700000002000"),
			UIDFrom:  "789",
			IDTo:     "group-abc",
			DName:    "GroupReplier",
			TS:       "1700000002",
			Content:  protocol.Content{String: &text},
			Quote:    quote,
		},
	}))

	got := drainInbound(t, mb)
	if got.Metadata["reply_to_message_id"] != "7858722000200" {
		t.Errorf("group reply_to_message_id = %q, want 7858722000200 (inbound's own MsgID)", got.Metadata["reply_to_message_id"])
	}
	if got.Metadata["reply_to_quote_payload"] == "" {
		t.Error("group reply_to_quote_payload should be stamped")
	}
	if got.Metadata["group_id"] != "group-abc" {
		t.Errorf("group_id = %q, want group-abc", got.Metadata["group_id"])
	}
}
