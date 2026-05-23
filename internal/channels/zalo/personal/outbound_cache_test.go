package personal

import (
	"testing"
	"time"
)

func TestOutboundCache_SetGetRoundtrip(t *testing.T) {
	t.Parallel()
	c := newOutboundCache(time.Minute)
	c.set("msg-1", "hello world")
	if got := c.get("msg-1"); got != "hello world" {
		t.Errorf("get=%q, want hello world", got)
	}
}

func TestOutboundCache_EmptyInputsIgnored(t *testing.T) {
	t.Parallel()
	c := newOutboundCache(time.Minute)
	c.set("", "hello")
	c.set("msg-1", "")
	if got := c.get(""); got != "" {
		t.Errorf("empty msgID lookup must return empty, got %q", got)
	}
	if got := c.get("msg-1"); got != "" {
		t.Errorf("set with empty preview should not store, got %q", got)
	}
}

func TestOutboundCache_Expires(t *testing.T) {
	t.Parallel()
	c := newOutboundCache(20 * time.Millisecond)
	c.set("msg-1", "hi")
	time.Sleep(40 * time.Millisecond)
	if got := c.get("msg-1"); got != "" {
		t.Errorf("expired entry returned %q", got)
	}
}

func TestOutboundCache_SweepDropsExpired(t *testing.T) {
	t.Parallel()
	c := newOutboundCache(20 * time.Millisecond)
	c.set("old", "x")
	time.Sleep(30 * time.Millisecond)
	c.set("new", "y")
	if _, ok := c.entries["old"]; ok {
		t.Errorf("sweep should have dropped expired entry")
	}
	if got := c.get("new"); got != "y" {
		t.Errorf("fresh entry missing: %q", got)
	}
}

func TestPreviewText(t *testing.T) {
	t.Parallel()
	if got := previewText("hi", 10); got != "hi" {
		t.Errorf("short string should pass through, got %q", got)
	}
	if got := previewText("0123456789", 5); got != "01234…" {
		t.Errorf("long string should truncate + ellipsis, got %q", got)
	}
}

func TestBuildReactionSummary(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                 string
		reactor, icon, sent  string
		msgID, preview       string
		removed              bool
		wantSub              string
	}{
		{"with preview added", "Alice", "❤", "positive", "100", "hi there", false, `Alice reacted ❤ (positive) on your reply: "hi there"`},
		{"with preview removed", "Alice", "", "unknown", "100", "hi there", true, `Alice removed their reaction on your reply: "hi there"`},
		{"no preview added", "Alice", "❤", "positive", "100", "", false, `Alice reacted ❤ (positive) on message 100`},
		{"no preview removed", "Alice", "", "unknown", "100", "", true, `Alice removed their reaction on message 100`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildReactionSummary(tc.reactor, tc.icon, tc.sent, tc.msgID, tc.preview, tc.removed)
			if got != tc.wantSub {
				t.Errorf("buildReactionSummary = %q, want %q", got, tc.wantSub)
			}
		})
	}
}
