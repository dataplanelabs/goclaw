package protocol

import (
	"context"
	"encoding/json"
	"testing"

	zcommon "github.com/nextlevelbuilder/goclaw/internal/channels/zalo/common"
	pkgproto "github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

func TestSendMessageWithOptions_TextProperties_Group(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "1001", 0)
	sess := newQuoteTestSession(t, srv)

	_, err := SendMessageWithOptions(context.Background(), sess, "group-1", ThreadTypeGroup, SendOptions{
		Text:   "hello world",
		Styles: []zcommon.Style{{Start: 0, Len: 5, St: zcommon.StyleBold}},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	payload := decryptRequestParams(t, (*cap)[0].body)
	raw, ok := payload["textProperties"].(string)
	if !ok {
		t.Fatalf("textProperties missing or not string: %v", payload["textProperties"])
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("decode textProperties: %v", err)
	}
	if got["ver"] != float64(0) {
		t.Errorf("ver = %v, want 0", got["ver"])
	}
	styles, _ := got["styles"].([]any)
	if len(styles) != 1 {
		t.Fatalf("styles len = %d, want 1", len(styles))
	}
	first, _ := styles[0].(map[string]any)
	if first["start"] != float64(0) || first["len"] != float64(5) || first["st"] != "b" {
		t.Errorf("style = %v, want {start:0,len:5,st:b}", first)
	}
}

func TestSendMessageWithOptions_TextProperties_DM(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "1001", 0)
	sess := newQuoteTestSession(t, srv)
	_, err := SendMessageWithOptions(context.Background(), sess, "user-1", ThreadTypeUser, SendOptions{
		Text:   "hi",
		Styles: []zcommon.Style{{Start: 0, Len: 2, St: zcommon.StyleItalic}},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	payload := decryptRequestParams(t, (*cap)[0].body)
	if _, ok := payload["textProperties"].(string); !ok {
		t.Errorf("DM should also carry textProperties; got %v", payload)
	}
}

func TestSendMessageWithOptions_TextProperties_WithMentions(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "1001", 0)
	sess := newQuoteTestSession(t, srv)
	_, err := SendMessageWithOptions(context.Background(), sess, "group-1", ThreadTypeGroup, SendOptions{
		Text:     "@Alice hello",
		Mentions: []pkgproto.Mention{{Position: 0, Length: 6, UserID: "u1"}},
		Styles:   []zcommon.Style{{Start: 7, Len: 5, St: "b"}},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	payload := decryptRequestParams(t, (*cap)[0].body)
	if _, ok := payload["mentionInfo"].(string); !ok {
		t.Error("mentionInfo missing")
	}
	if _, ok := payload["textProperties"].(string); !ok {
		t.Error("textProperties missing")
	}
}

func TestSendMessageWithOptions_TextProperties_WithQuote(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "1001", 0)
	sess := newQuoteTestSession(t, srv)
	q := &SendMessageQuote{
		OwnerID: "111", MsgID: "9876543210", CliMsgID: "1709300000",
		MsgType: "chat.text", Msg: "original", TS: "1709300000",
	}
	_, err := SendMessageWithOptions(context.Background(), sess, "group-1", ThreadTypeGroup, SendOptions{
		Text:   "reply",
		Quote:  q,
		Styles: []zcommon.Style{{Start: 0, Len: 5, St: "b"}},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	payload := decryptRequestParams(t, (*cap)[0].body)
	if _, ok := payload["qmsgOwner"]; !ok {
		t.Error("qmsg* fields missing on quote path")
	}
	if _, ok := payload["textProperties"].(string); !ok {
		t.Error("textProperties missing on quote path — Zalo /quote may not honor styles; verify in dogfood")
	}
}

func TestSendMessageWithOptions_NoStyles_OmitsTextProperties(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "1001", 0)
	sess := newQuoteTestSession(t, srv)
	for _, styles := range [][]zcommon.Style{nil, {}} {
		_, err := SendMessageWithOptions(context.Background(), sess, "user-1", ThreadTypeUser, SendOptions{
			Text:   "plain",
			Styles: styles,
		})
		if err != nil {
			t.Fatalf("send: %v", err)
		}
	}
	for i, c := range *cap {
		payload := decryptRequestParams(t, c.body)
		if _, present := payload["textProperties"]; present {
			t.Errorf("call %d: textProperties should be ABSENT when styles empty/nil; got %v", i, payload["textProperties"])
		}
	}
}

func TestSendMessageWithOptions_MultipleStyles_ArrayOrder(t *testing.T) {
	t.Parallel()
	srv, cap := captureServer(t, "1001", 0)
	sess := newQuoteTestSession(t, srv)
	_, err := SendMessageWithOptions(context.Background(), sess, "group-1", ThreadTypeGroup, SendOptions{
		Text: "abcd efghij klmnopqrst",
		Styles: []zcommon.Style{
			{Start: 0, Len: 4, St: "b"},
			{Start: 5, Len: 5, St: "i"},
			{Start: 11, Len: 8, St: "s"},
		},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	payload := decryptRequestParams(t, (*cap)[0].body)
	raw := payload["textProperties"].(string)
	var got map[string]any
	_ = json.Unmarshal([]byte(raw), &got)
	styles, _ := got["styles"].([]any)
	if len(styles) != 3 {
		t.Fatalf("got %d styles, want 3", len(styles))
	}
	for i, want := range []string{"b", "i", "s"} {
		s, _ := styles[i].(map[string]any)
		if s["st"] != want {
			t.Errorf("styles[%d].st = %v, want %v", i, s["st"], want)
		}
	}
}

func TestStyle_ConstantValues(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"Bold":          zcommon.StyleBold,
		"Italic":        zcommon.StyleItalic,
		"Underline":     zcommon.StyleUnderline,
		"Strikethrough": zcommon.StyleStrikethrough,
		"ListUnordered": zcommon.StyleListUnordered,
		"ListOrdered":   zcommon.StyleListOrdered,
	}
	want := map[string]string{
		"Bold":          "b",
		"Italic":        "i",
		"Underline":     "u",
		"Strikethrough": "s",
		"ListUnordered": "lst_1",
		"ListOrdered":   "lst_2",
	}
	for k, v := range cases {
		if v != want[k] {
			t.Errorf("Style%s = %q, want %q (zca-js wire literal)", k, v, want[k])
		}
	}
}
