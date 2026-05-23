package personal

import (
	"context"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
	"github.com/nextlevelbuilder/goclaw/internal/config"
)

func TestResolveOutboundReactionEmoji(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"thinking": protocol.ReactionLike,
		"done":     protocol.ReactionHeart,
		"error":    protocol.ReactionCry,
		"unknown":  "",
		"":         "",
	}
	for status, want := range cases {
		if got := resolveOutboundReactionEmoji(status); got != want {
			t.Errorf("resolveOutboundReactionEmoji(%q)=%q, want %q", status, got, want)
		}
	}
}

func newOutboundTestChannel(t *testing.T, level string) *Channel {
	t.Helper()
	cfg := config.ZaloPersonalConfig{ReactionLevel: level}
	ch := &Channel{
		BaseChannel: channels.NewBaseChannel(channels.TypeZaloPersonal, nil, nil),
		config:      cfg,
		stopCh:      make(chan struct{}),
	}
	ch.reactionCtx, ch.reactionCancel = context.WithCancel(context.Background())
	return ch
}

func TestOnReactionEvent_OffShortCircuits(t *testing.T) {
	t.Parallel()
	for _, level := range []string{"", "off"} {
		ch := newOutboundTestChannel(t, level)
		if err := ch.OnReactionEvent(context.Background(), "u", "m", "done"); err != nil {
			t.Fatalf("OnReactionEvent: %v", err)
		}
		count := 0
		ch.reactions.Range(func(_, _ any) bool { count++; return true })
		if count != 0 {
			t.Errorf("level=%q must not store controller, got %d entries", level, count)
		}
	}
}

func TestOnReactionEvent_MinimalSkipsIntermediate(t *testing.T) {
	t.Parallel()
	ch := newOutboundTestChannel(t, "minimal")
	_ = ch.OnReactionEvent(context.Background(), "u", "m", "thinking")
	count := 0
	ch.reactions.Range(func(_, _ any) bool { count++; return true })
	if count != 0 {
		t.Errorf("minimal must skip 'thinking', got %d entries", count)
	}
}

func TestOnReactionEvent_EmptyIDsShortCircuit(t *testing.T) {
	t.Parallel()
	ch := newOutboundTestChannel(t, "full")
	_ = ch.OnReactionEvent(context.Background(), "", "msg", "done")
	_ = ch.OnReactionEvent(context.Background(), "user", "", "done")
	count := 0
	ch.reactions.Range(func(_, _ any) bool { count++; return true })
	if count != 0 {
		t.Errorf("empty IDs must short-circuit, got %d entries", count)
	}
}

func TestOnReactionEvent_StoresControllerWhenEnabled(t *testing.T) {
	t.Parallel()
	ch := newOutboundTestChannel(t, "full")
	_ = ch.OnReactionEvent(context.Background(), "u", "m", "thinking")
	count := 0
	ch.reactions.Range(func(_, _ any) bool { count++; return true })
	if count != 1 {
		t.Errorf("expected 1 controller, got %d", count)
	}
	ch.reactionCancel()
	ch.reactionWG.Wait()
}

func TestRecordReplyLen_StoresLength(t *testing.T) {
	t.Parallel()
	ch := newOutboundTestChannel(t, "full")
	ch.recordReactionReplyLen("u1", 42)
	v, ok := ch.lastReplyChars.Load("u1")
	if !ok || v.(int) != 42 {
		t.Errorf("expected lastReplyChars[u1]=42, got %v ok=%v", v, ok)
	}
	ch.recordReactionReplyLen("", 10)
	ch.recordReactionReplyLen("u2", 0)
	if _, ok := ch.lastReplyChars.Load(""); ok {
		t.Errorf("empty chatID should not be stored")
	}
	if _, ok := ch.lastReplyChars.Load("u2"); ok {
		t.Errorf("zero length should not be stored")
	}
}

func TestTerminalReactionDelay_RespectsBounds(t *testing.T) {
	t.Parallel()
	ch := newOutboundTestChannel(t, "full")
	ch.config.ReactionTerminalDelayMinMs = 100
	ch.config.ReactionTerminalDelayMaxMs = 200
	d := ch.terminalReactionDelay("u")
	if d.Milliseconds() < 100 || d.Milliseconds() > 1700 {
		t.Errorf("delay %v out of expected window [100ms, 1700ms]", d)
	}
}
