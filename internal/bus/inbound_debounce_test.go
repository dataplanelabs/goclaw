package bus

import (
	"testing"
	"time"
)

func TestInboundDebouncerMergesTextThenMedia(t *testing.T) {
	t.Parallel()

	flushed := make(chan InboundMessage, 1)
	debouncer := NewInboundDebouncer(20*time.Millisecond, func(msg InboundMessage) {
		flushed <- msg
	})
	defer debouncer.Stop()

	debouncer.Push(InboundMessage{
		Channel:  "zalo",
		ChatID:   "chat-1",
		SenderID: "sender-1",
		Content:  "please use the image",
		Metadata: map[string]string{"message_id": "m1"},
	})
	debouncer.Push(InboundMessage{
		Channel:  "zalo",
		ChatID:   "chat-1",
		SenderID: "sender-1",
		Media:    []MediaFile{{Path: "/tmp/example.jpg", Filename: "example.jpg"}},
		Metadata: map[string]string{"message_id": "m2"},
	})

	got := requireDebouncedMessage(t, flushed)
	if got.Content != "please use the image" {
		t.Fatalf("content = %q, want original text", got.Content)
	}
	if len(got.Media) != 1 || got.Media[0].Path != "/tmp/example.jpg" {
		t.Fatalf("media = %+v, want one merged media file", got.Media)
	}
	if got.Metadata["message_id"] != "m2" {
		t.Fatalf("message_id = %q, want latest metadata", got.Metadata["message_id"])
	}
}

func TestInboundDebouncerWaitsForMediaOnly(t *testing.T) {
	t.Parallel()

	flushed := make(chan InboundMessage, 1)
	debouncer := NewInboundDebouncer(30*time.Millisecond, func(msg InboundMessage) {
		flushed <- msg
	})
	defer debouncer.Stop()

	debouncer.Push(InboundMessage{
		Channel:  "zalo",
		ChatID:   "chat-1",
		SenderID: "sender-1",
		Media:    []MediaFile{{Path: "/tmp/example.jpg"}},
	})

	select {
	case msg := <-flushed:
		t.Fatalf("media-only message flushed before debounce window: %+v", msg)
	case <-time.After(10 * time.Millisecond):
	}

	got := requireDebouncedMessage(t, flushed)
	if len(got.Media) != 1 {
		t.Fatalf("media count = %d, want 1", len(got.Media))
	}
}

func TestInboundDebouncerDisabledPassesThroughImmediately(t *testing.T) {
	t.Parallel()

	flushed := make(chan InboundMessage, 1)
	debouncer := NewInboundDebouncer(0, func(msg InboundMessage) {
		flushed <- msg
	})

	debouncer.Push(InboundMessage{
		Channel:  "zalo",
		ChatID:   "chat-1",
		SenderID: "sender-1",
		Content:  "hello",
	})

	got := requireDebouncedMessage(t, flushed)
	if got.Content != "hello" {
		t.Fatalf("content = %q, want hello", got.Content)
	}
}

func requireDebouncedMessage(t *testing.T, ch <-chan InboundMessage) InboundMessage {
	t.Helper()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for debounced message")
		return InboundMessage{}
	}
}
