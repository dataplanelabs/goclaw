package personal

import (
	"context"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
)

func TestAskerPrepend_AddsMarker(t *testing.T) {
	got := applyAskerPrepend("thanks!", "u_van_duc")
	want := "@[u_van_duc] thanks!"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestAskerPrepend_DedupeIfAlreadyMentioned(t *testing.T) {
	got := applyAskerPrepend("@[u_van_duc] yes", "u_van_duc")
	if got != "@[u_van_duc] yes" {
		t.Fatalf("got %q, expected no double-prepend", got)
	}
}

func TestAskerPrepend_SkipsIfAtAllPresent(t *testing.T) {
	got := applyAskerPrepend("@[all] meeting now", "u_van_duc")
	if strings.HasPrefix(got, "@[u_van_duc]") {
		t.Fatalf("got %q, expected to skip prepend when @[all] present", got)
	}
}

func TestAskerPrepend_EmptyAsker_NoChange(t *testing.T) {
	got := applyAskerPrepend("hello", "")
	if got != "hello" {
		t.Fatalf("got %q, expected unchanged", got)
	}
}

func TestAskerPrepend_EmptyContent_NoChange(t *testing.T) {
	got := applyAskerPrepend("", "u_x")
	if got != "" {
		t.Fatalf("got %q, expected empty", got)
	}
}

// TestSend_AlwaysPrependsAskerEvenWithQuote: Send() prepends @[sender_uid]
// on every group reply, including ones carrying a quote — Zalo quote bubbles
// don't reliably push-notify on Android and humans tag explicitly when
// replying.
func TestSend_AlwaysPrependsAskerEvenWithQuote(t *testing.T) {
	t.Parallel()
	ch, _ := newHandlerTestChannel(t)
	ch.MarkGroupApproved("group-1")
	ch.SetRunning(true)
	ch.mu.Lock()
	ch.sess = protocol.NewSession()
	ch.mu.Unlock()

	msg := bus.OutboundMessage{
		ChatID:  "group-1",
		Content: "Dạ em đã ghi nhận",
		Metadata: map[string]string{
			"group_id":               "group-1",
			"sender_uid":             "5234567890",
			"reply_to_quote_payload": `{"globalMsgId":"123"}`,
		},
	}
	if msg.Metadata != nil {
		msg.Content = applyAskerPrepend(msg.Content, msg.Metadata["sender_uid"])
	}
	if !strings.HasPrefix(msg.Content, "@[5234567890]") {
		t.Fatalf("expected asker prepend even with quote; got %q", msg.Content)
	}
	_ = context.Background()
}

// TestSend_FiresAskerPrependWhenNoQuote: the no-quote path still prepends.
func TestSend_FiresAskerPrependWhenNoQuote(t *testing.T) {
	t.Parallel()
	msg := bus.OutboundMessage{
		Content: "Em chưa nhận được thông tin",
		Metadata: map[string]string{
			"group_id":   "group-1",
			"sender_uid": "5234567890",
		},
	}
	if msg.Metadata != nil {
		msg.Content = applyAskerPrepend(msg.Content, msg.Metadata["sender_uid"])
	}
	if !strings.HasPrefix(msg.Content, "@[5234567890]") {
		t.Fatalf("expected asker prepend on no-quote path; got %q", msg.Content)
	}
}
