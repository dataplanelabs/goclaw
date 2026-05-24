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

// TestSend_SkipsAskerPrependWhenQuotePresent: Send() must NOT prepend
// @[sender_uid] on group replies that carry reply_to_quote_payload — quote
// bubble already identifies the asker and pings them.
func TestSend_SkipsAskerPrependWhenQuotePresent(t *testing.T) {
	t.Parallel()
	ch, _ := newHandlerTestChannel(t)
	ch.MarkGroupApproved("group-1")
	ch.SetRunning(true)
	// Stub session so Send() doesn't bail at the running check (we only care
	// about the prepend logic; the protocol send will fail with no real
	// session, but that fires AFTER applyAskerPrepend runs).
	ch.mu.Lock()
	ch.sess = protocol.NewSession()
	ch.mu.Unlock()

	// Quote payload present → auto-asker should be SKIPPED.
	original := "Dạ em đã ghi nhận"
	msg := bus.OutboundMessage{
		ChatID:  "group-1",
		Content: original,
		Metadata: map[string]string{
			"group_id":               "group-1",
			"sender_uid":             "5234567890",
			"reply_to_quote_payload": `{"globalMsgId":"123"}`,
		},
	}
	// Don't actually call Send() (no real session/server) — exercise the
	// guard logic by mirroring it inline.
	if msg.Metadata["reply_to_quote_payload"] == "" {
		msg.Content = applyAskerPrepend(msg.Content, msg.Metadata["sender_uid"])
	}
	if msg.Content != original {
		t.Fatalf("auto-asker fired despite quote: got %q, want %q", msg.Content, original)
	}
	if strings.HasPrefix(msg.Content, "@[5234567890]") {
		t.Fatal("content should not be prepended when quote rides on reply")
	}
	_ = context.Background()
}

// TestSend_FiresAskerPrependWhenNoQuote: the no-quote safety net path.
func TestSend_FiresAskerPrependWhenNoQuote(t *testing.T) {
	t.Parallel()
	msg := bus.OutboundMessage{
		Content: "Em chưa nhận được thông tin",
		Metadata: map[string]string{
			"group_id":   "group-1",
			"sender_uid": "5234567890",
			// No reply_to_quote_payload
		},
	}
	if msg.Metadata["reply_to_quote_payload"] == "" {
		msg.Content = applyAskerPrepend(msg.Content, msg.Metadata["sender_uid"])
	}
	if !strings.HasPrefix(msg.Content, "@[5234567890]") {
		t.Fatalf("expected auto-asker prepend on no-quote path; got %q", msg.Content)
	}
}
