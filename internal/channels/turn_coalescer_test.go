package channels

import (
	"testing"
	"time"
)

func TestTurnCoalescerMergesAndFlushesLatestGeneration(t *testing.T) {
	t.Parallel()

	flushed := make(chan string, 1)
	c := NewTurnCoalescer[string](
		30*time.Millisecond,
		func(existing, next string) string { return existing + "\n" + next },
		func(value string) { flushed <- value },
	)

	c.Enqueue("k", "text")
	time.Sleep(10 * time.Millisecond)
	c.Enqueue("k", "media")

	select {
	case value := <-flushed:
		t.Fatalf("flushed before reset delay: %q", value)
	case <-time.After(15 * time.Millisecond):
	}

	select {
	case value := <-flushed:
		if value != "text\nmedia" {
			t.Fatalf("flushed value = %q, want merged value", value)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for coalesced value")
	}
}

func TestTurnCoalescerDisabledFlushesImmediately(t *testing.T) {
	t.Parallel()

	flushed := make(chan string, 1)
	c := NewTurnCoalescer[string](0, nil, func(value string) { flushed <- value })
	c.Enqueue("k", "text")

	select {
	case value := <-flushed:
		if value != "text" {
			t.Fatalf("flushed value = %q, want text", value)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for immediate flush")
	}
}

func TestTurnCoalescerFlushAll(t *testing.T) {
	t.Parallel()

	flushed := make(chan string, 2)
	c := NewTurnCoalescer[string](time.Hour, nil, func(value string) { flushed <- value })
	c.Enqueue("a", "one")
	c.Enqueue("b", "two")

	c.FlushAll()

	got := map[string]bool{}
	for range 2 {
		select {
		case value := <-flushed:
			got[value] = true
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timed out waiting for FlushAll")
		}
	}
	if !got["one"] || !got["two"] {
		t.Fatalf("flushed values = %v, want one and two", got)
	}
}
