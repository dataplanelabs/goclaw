package protocol

import (
	"context"
	"errors"
	"strings"
	"testing"

	pkgproto "github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

func TestSendMessageWithOptions_NoMentions_UsesSendmsgEndpoint(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "1001", 0)
	sess := newQuoteTestSession(t, srv)

	_, err := SendMessageWithOptions(context.Background(), sess, "group-abc", ThreadTypeGroup, SendOptions{Text: "plain"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !strings.HasSuffix((*cap)[0].path, apiPathGroup) {
		t.Errorf("path = %q, want suffix %q", (*cap)[0].path, apiPathGroup)
	}
}

func TestSendMessageWithOptions_GroupMention_RoutesToMentionEndpoint(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "1001", 0)
	sess := newQuoteTestSession(t, srv)

	mentions := []pkgproto.Mention{
		{UserID: "u_a", DisplayName: "Alice", Position: 0, Length: 6, Type: 0},
	}
	_, err := SendMessageWithOptions(context.Background(), sess, "group-abc", ThreadTypeGroup, SendOptions{
		Text:     "@Alice hi",
		Mentions: mentions,
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !strings.HasSuffix((*cap)[0].path, apiPathGroupMention) {
		t.Errorf("path = %q, want suffix %q", (*cap)[0].path, apiPathGroupMention)
	}
}

func TestSendMessageWithOptions_MentionInfoJSONExactMatch(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "1001", 0)
	sess := newQuoteTestSession(t, srv)

	mentions := []pkgproto.Mention{
		{UserID: "u_a", DisplayName: "Alice", Position: 7, Length: 6, Type: 0},
		{UserID: pkgproto.MentionAllUID, DisplayName: "all", Position: 18, Length: 4, Type: 1},
	}
	_, err := SendMessageWithOptions(context.Background(), sess, "group-abc", ThreadTypeGroup, SendOptions{
		Text:     "Cảm ơn @Alice và @all!",
		Mentions: mentions,
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	payload := decryptRequestParams(t, (*cap)[0].body)
	mentionInfo, ok := payload["mentionInfo"].(string)
	if !ok {
		t.Fatalf("mentionInfo missing or wrong type: %v", payload)
	}
	want := `[{"pos":7,"uid":"u_a","len":6,"type":0},{"pos":18,"uid":"-1","len":4,"type":1}]`
	if mentionInfo != want {
		t.Fatalf("mentionInfo wire shape drift:\n got  %s\n want %s", mentionInfo, want)
	}
}

func TestSendMessageWithOptions_AtAllOnly(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "1001", 0)
	sess := newQuoteTestSession(t, srv)

	mentions := []pkgproto.Mention{
		{UserID: pkgproto.MentionAllUID, DisplayName: "all", Position: 0, Length: 4, Type: 1},
	}
	_, err := SendMessageWithOptions(context.Background(), sess, "group-abc", ThreadTypeGroup, SendOptions{
		Text:     "@all meeting!",
		Mentions: mentions,
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !strings.HasSuffix((*cap)[0].path, apiPathGroupMention) {
		t.Errorf("path = %q, want %q", (*cap)[0].path, apiPathGroupMention)
	}
}

func TestSendMessageWithOptions_DM_DropsMentions(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "1001", 0)
	sess := newQuoteTestSession(t, srv)

	mentions := []pkgproto.Mention{
		{UserID: "u_a", DisplayName: "Alice", Position: 0, Length: 6, Type: 0},
	}
	_, err := SendMessageWithOptions(context.Background(), sess, "user-1", ThreadTypeUser, SendOptions{
		Text:     "@Alice hi",
		Mentions: mentions,
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !strings.HasSuffix((*cap)[0].path, apiPathDM) {
		t.Errorf("path = %q, want %q (mentions must NOT route DM to mention endpoint)", (*cap)[0].path, apiPathDM)
	}
	payload := decryptRequestParams(t, (*cap)[0].body)
	if _, present := payload["mentionInfo"]; present {
		t.Errorf("mentionInfo must NOT be present on DM payload: %v", payload)
	}
}

func TestSendMessageWithOptions_QuoteWithoutAttachment_AllowsMentions(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "1001", 0)
	sess := newQuoteTestSession(t, srv)

	q := &SendMessageQuote{OwnerID: "111", MsgID: "9876", MsgType: "chat.text", Msg: "orig"}
	mentions := []pkgproto.Mention{
		{UserID: "u_a", DisplayName: "Alice", Position: 0, Length: 6, Type: 0},
	}
	_, err := SendMessageWithOptions(context.Background(), sess, "group-abc", ThreadTypeGroup, SendOptions{
		Text:     "@Alice see above",
		Quote:    q,
		Mentions: mentions,
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	payload := decryptRequestParams(t, (*cap)[0].body)
	if _, ok := payload["mentionInfo"]; !ok {
		t.Errorf("expected mentionInfo retained when quote has no attachment, got payload %v", payload)
	}
	if _, ok := payload["qmsgId"]; !ok {
		t.Errorf("expected qmsgId still present alongside mentionInfo: %v", payload)
	}
	// Quote endpoint must WIN when both quote and mentions are set (zca-js parity).
	// Routing mention over quote drops the quote bubble on Zalo client.
	if !strings.HasSuffix((*cap)[0].path, apiPathGroupQuote) {
		t.Errorf("path = %q, want suffix %q (quote endpoint must win over mention when both set)", (*cap)[0].path, apiPathGroupQuote)
	}
}

// zca-js does NOT drop mentions on quote-with-attachment — both ride together
// (see sendMessage.ts:344-372 — mentionInfo set whenever isMentionsValid).
func TestSendMessageWithOptions_QuoteAttachment_KeepsMentions(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "1001", 0)
	sess := newQuoteTestSession(t, srv)

	q := &SendMessageQuote{
		OwnerID: "111", MsgID: "9876", MsgType: "chat.photo", Msg: "img",
		Attach: `{"hdUrl":"x"}`,
	}
	mentions := []pkgproto.Mention{
		{UserID: "u_a", DisplayName: "Alice", Position: 0, Length: 6, Type: 0},
	}
	_, err := SendMessageWithOptions(context.Background(), sess, "group-abc", ThreadTypeGroup, SendOptions{
		Text:     "@Alice look",
		Quote:    q,
		Mentions: mentions,
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	payload := decryptRequestParams(t, (*cap)[0].body)
	if _, present := payload["mentionInfo"]; !present {
		t.Errorf("mentionInfo MUST ride with quote-attachment per zca-js; payload=%v", payload)
	}
}

func TestSendMessageWithOptions_FiltersInvalidMentions(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "1001", 0)
	sess := newQuoteTestSession(t, srv)

	mentions := []pkgproto.Mention{
		{UserID: "u_a", DisplayName: "Alice", Position: 0, Length: 6, Type: 0},
		{UserID: "", DisplayName: "Bad", Position: 0, Length: 5, Type: 0},        // bad: empty UID
		{UserID: "u_b", DisplayName: "Bob", Position: -1, Length: 3, Type: 0},    // bad: negative pos
		{UserID: "u_c", DisplayName: "Cat", Position: 7, Length: 0, Type: 0},     // bad: zero len
	}
	_, err := SendMessageWithOptions(context.Background(), sess, "group-abc", ThreadTypeGroup, SendOptions{
		Text:     "@Alice and stuff",
		Mentions: mentions,
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	payload := decryptRequestParams(t, (*cap)[0].body)
	mi, _ := payload["mentionInfo"].(string)
	if !strings.Contains(mi, `"uid":"u_a"`) || strings.Contains(mi, `"uid":""`) ||
		strings.Contains(mi, `"uid":"u_b"`) || strings.Contains(mi, `"uid":"u_c"`) {
		t.Fatalf("filter did not strip invalid mentions; mentionInfo=%s", mi)
	}
}

func TestSendMessageWithOptions_MentionEndpointReject_WrapsErrMentionRejected(t *testing.T) {
	t.Parallel()
	srv, _ := captureServer(t, "1001", 1234) // non-zero error_code triggers wrap
	sess := newQuoteTestSession(t, srv)

	mentions := []pkgproto.Mention{
		{UserID: "u_a", DisplayName: "Alice", Position: 0, Length: 6, Type: 0},
	}
	_, err := SendMessageWithOptions(context.Background(), sess, "group-abc", ThreadTypeGroup, SendOptions{
		Text:     "@Alice hi",
		Mentions: mentions,
	})
	if !errors.Is(err, ErrMentionRejected) {
		t.Fatalf("expected ErrMentionRejected, got %v", err)
	}
}
