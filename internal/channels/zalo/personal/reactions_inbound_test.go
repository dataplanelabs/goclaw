package personal

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/channels/zalo/personal/protocol"
	"github.com/nextlevelbuilder/goclaw/internal/config"
)

func makeEvent(threadID, uidFrom, msgID, code string, opts ...func(*ReactionEvent)) ReactionEvent {
	ev := ReactionEvent{
		MsgID:    msgID,
		CliMsgID: "c" + msgID,
		ThreadID: threadID,
		UIDFrom:  uidFrom,
		Code:     code,
	}
	for _, o := range opts {
		o(&ev)
	}
	return ev
}

// --- Coalescer ---

func TestCoalescer_SingleEvent(t *testing.T) {
	t.Parallel()
	var got []ReactionEvent
	var mu sync.Mutex
	rc := newReactionCoalescer(20*time.Millisecond, func(ev ReactionEvent) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, ev)
	})
	rc.Submit(makeEvent("t1", "u1", "m1", protocol.ReactionHeart))
	time.Sleep(80 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(got), got)
	}
}

func TestCoalescer_RapidUpdatesLatestWins(t *testing.T) {
	t.Parallel()
	var got []ReactionEvent
	var mu sync.Mutex
	rc := newReactionCoalescer(40*time.Millisecond, func(ev ReactionEvent) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, ev)
	})
	codes := []string{protocol.ReactionHeart, protocol.ReactionLike, protocol.ReactionWow}
	for _, c := range codes {
		rc.Submit(makeEvent("t", "u", "m", c))
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1 (coalesced): %+v", len(got), got)
	}
	if got[0].Code != protocol.ReactionWow {
		t.Errorf("got %q, want %q (latest)", got[0].Code, protocol.ReactionWow)
	}
}

func TestCoalescer_DifferentKeysIndependent(t *testing.T) {
	t.Parallel()
	var n int32
	rc := newReactionCoalescer(20*time.Millisecond, func(ReactionEvent) {
		atomic.AddInt32(&n, 1)
	})
	rc.Submit(makeEvent("t1", "u1", "m1", protocol.ReactionHeart))
	rc.Submit(makeEvent("t2", "u1", "m1", protocol.ReactionHeart)) // different thread
	rc.Submit(makeEvent("t1", "u2", "m1", protocol.ReactionHeart)) // different user
	rc.Submit(makeEvent("t1", "u1", "m2", protocol.ReactionHeart)) // different message
	time.Sleep(100 * time.Millisecond)
	if got := atomic.LoadInt32(&n); got != 4 {
		t.Errorf("got %d emits, want 4", got)
	}
}

func TestCoalescer_Flush(t *testing.T) {
	t.Parallel()
	var got []ReactionEvent
	var mu sync.Mutex
	rc := newReactionCoalescer(1*time.Minute, func(ev ReactionEvent) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, ev)
	})
	rc.Submit(makeEvent("t1", "u1", "m1", protocol.ReactionHeart))
	rc.Submit(makeEvent("t2", "u1", "m1", protocol.ReactionLike))
	if got := rc.pendingCount(); got != 2 {
		t.Fatalf("pendingCount before flush=%d", got)
	}
	rc.Flush()
	// Brief wait so any cancelled sleepers exit cleanly without panicking.
	time.Sleep(10 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Errorf("flush emitted %d events, want 2", len(got))
	}
}

func TestCoalescer_Cancel(t *testing.T) {
	t.Parallel()
	var n int32
	rc := newReactionCoalescer(50*time.Millisecond, func(ReactionEvent) {
		atomic.AddInt32(&n, 1)
	})
	rc.Submit(makeEvent("t", "u", "m1", protocol.ReactionHeart))
	rc.Submit(makeEvent("t", "u", "m2", protocol.ReactionHeart))
	rc.Cancel()
	time.Sleep(120 * time.Millisecond)
	if got := atomic.LoadInt32(&n); got != 0 {
		t.Errorf("cancelled events should not emit, got %d", got)
	}
}

// --- onReactionEvent filters ---

func newTestChannel(t *testing.T, cfg config.ZaloPersonalConfig) (*Channel, *[]ReactionEvent, *sync.Mutex) {
	t.Helper()
	var got []ReactionEvent
	var mu sync.Mutex
	ch := &Channel{
		BaseChannel: channels.NewBaseChannel(channels.TypeZaloPersonal, nil, nil),
		config:      cfg,
	}
	ch.reactionCoalescer = newReactionCoalescer(10*time.Millisecond, func(ev ReactionEvent) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, ev)
	})
	return ch, &got, &mu
}

func TestOnReactionEvent_SuppressSelf(t *testing.T) {
	t.Parallel()
	ch, _, _ := newTestChannel(t, config.ZaloPersonalConfig{ReactionsMode: reactionsModeInbound})
	ev := makeEvent("t", "u", "m", protocol.ReactionHeart, func(e *ReactionEvent) {
		e.IsSelf = true
	})
	ch.onReactionEvent(ev)
	if got := ch.reactionCoalescer.pendingCount(); got != 0 {
		t.Errorf("self reaction must not enter coalescer, pending=%d", got)
	}
}

func TestOnReactionEvent_AllowSelfOptIn(t *testing.T) {
	t.Parallel()
	ch, _, _ := newTestChannel(t, config.ZaloPersonalConfig{ListenSelfReactions: true, ReactionsMode: reactionsModeInbound})
	ev := makeEvent("t", "u", "m", protocol.ReactionHeart, func(e *ReactionEvent) {
		e.IsSelf = true
	})
	ch.onReactionEvent(ev)
	if got := ch.reactionCoalescer.pendingCount(); got != 1 {
		t.Errorf("with opt-in, self reaction must enter coalescer, pending=%d", got)
	}
}

func TestOnReactionEvent_SuppressHistoric(t *testing.T) {
	t.Parallel()
	ch, _, _ := newTestChannel(t, config.ZaloPersonalConfig{ReactionsMode: reactionsModeInbound})
	ev := makeEvent("t", "u", "m", protocol.ReactionHeart, func(e *ReactionEvent) {
		e.IsHistoric = true
	})
	ch.onReactionEvent(ev)
	if got := ch.reactionCoalescer.pendingCount(); got != 0 {
		t.Errorf("historic reaction must be dropped, pending=%d", got)
	}
}

func TestOnReactionEvent_KillSwitch(t *testing.T) {
	t.Parallel()
	ch, _, _ := newTestChannel(t, config.ZaloPersonalConfig{DisableReactions: true})
	ev := makeEvent("t", "u", "m", protocol.ReactionHeart)
	ch.onReactionEvent(ev)
	if got := ch.reactionCoalescer.pendingCount(); got != 0 {
		t.Errorf("disabled reactions must be dropped, pending=%d", got)
	}
}

func TestOnReactionEvent_DefaultModeIsFeedback(t *testing.T) {
	t.Parallel()
	ch, _, _ := newTestChannel(t, config.ZaloPersonalConfig{})
	ev := makeEvent("t", "u", "m", protocol.ReactionHeart)
	ch.onReactionEvent(ev)
	if got := ch.reactionCoalescer.pendingCount(); got != 0 {
		t.Errorf("feedback mode must not enter coalescer, pending=%d", got)
	}
}

func TestOnReactionEvent_SilentMode(t *testing.T) {
	t.Parallel()
	ch, _, _ := newTestChannel(t, config.ZaloPersonalConfig{ReactionsMode: reactionsModeSilent})
	ev := makeEvent("t", "u", "m", protocol.ReactionHeart)
	ch.onReactionEvent(ev)
	if got := ch.reactionCoalescer.pendingCount(); got != 0 {
		t.Errorf("silent mode must drop, pending=%d", got)
	}
}

func TestReactionSentiment(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		protocol.ReactionHeart: "positive",
		protocol.ReactionLike:  "positive",
		protocol.ReactionHaha:  "positive",
		protocol.ReactionAngry: "negative",
		protocol.ReactionCry:   "negative",
		protocol.ReactionWorry: "negative",
		protocol.ReactionWow:   "neutral",
		protocol.ReactionNone:  "unknown",
	}
	for code, want := range cases {
		if got := reactionSentiment(code); got != want {
			t.Errorf("reactionSentiment(%q)=%q, want %q", code, got, want)
		}
	}
}

// --- formatReactionLine ---

func TestFormatReactionLine_AddedHeart(t *testing.T) {
	t.Parallel()
	ev := makeEvent("t", "u", "100", protocol.ReactionHeart, func(e *ReactionEvent) {
		e.DName = "Alice"
	})
	got := formatReactionLine(ev)
	if !strings.Contains(got, "Alice") {
		t.Errorf("missing name: %q", got)
	}
	if !strings.Contains(got, "100") {
		t.Errorf("missing msgID: %q", got)
	}
	if !strings.Contains(got, "❤") {
		t.Errorf("missing heart emoji: %q", got)
	}
}

func TestFormatReactionLine_Removed(t *testing.T) {
	t.Parallel()
	ev := makeEvent("t", "u", "100", protocol.ReactionNone, func(e *ReactionEvent) {
		e.DName = "Bob"
	})
	got := formatReactionLine(ev)
	if !strings.Contains(got, "Bob") || !strings.Contains(got, "removed") {
		t.Errorf("missing name/removed in: %q", got)
	}
}

func TestFormatReactionLine_FallbackToUID(t *testing.T) {
	t.Parallel()
	ev := makeEvent("t", "user-xyz", "100", protocol.ReactionHeart) // empty DName
	got := formatReactionLine(ev)
	if !strings.Contains(got, "user-xyz") {
		t.Errorf("must fall back to UID when DName is empty: %q", got)
	}
}

func TestCoalesceKey_Stable(t *testing.T) {
	t.Parallel()
	a := makeEvent("t", "u", "m", protocol.ReactionHeart)
	b := makeEvent("t", "u", "m", protocol.ReactionLike)
	if coalesceKey(a) != coalesceKey(b) {
		t.Errorf("same thread+user+msg must share key, got %q vs %q", coalesceKey(a), coalesceKey(b))
	}
}
