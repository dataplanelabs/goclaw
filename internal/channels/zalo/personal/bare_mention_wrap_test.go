package personal

import (
	"context"
	"strings"
	"testing"
)

func TestWrapBareMentions_SingleWordHit(t *testing.T) {
	t.Parallel()
	ch, _ := newHandlerTestChannel(t)
	ch.memberCache.Set("g1", "u_a", "Alice")
	got := ch.wrapBareMentions(context.Background(), "g1", "Hi @Alice please review")
	want := "Hi @[Alice] please review"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestWrapBareMentions_MultiWordHit(t *testing.T) {
	t.Parallel()
	ch, _ := newHandlerTestChannel(t)
	ch.memberCache.Set("g1", "u_n", "Ngoc Tran")
	got := ch.wrapBareMentions(context.Background(), "g1", "Chị Trang @Ngoc Tran cứ tạo group")
	want := "Chị Trang @[Ngoc Tran] cứ tạo group"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestWrapBareMentions_LongestPrefixWins(t *testing.T) {
	t.Parallel()
	ch, _ := newHandlerTestChannel(t)
	// Two members: short name + long name. Longest should match first.
	ch.memberCache.Set("g1", "u_short", "Ngoc")
	ch.memberCache.Set("g1", "u_long", "Ngoc Tran")
	got := ch.wrapBareMentions(context.Background(), "g1", "Hi @Ngoc Tran here")
	if !strings.Contains(got, "@[Ngoc Tran]") {
		t.Fatalf("got %q, expected longest-prefix @[Ngoc Tran]", got)
	}
}

func TestWrapBareMentions_ShrinkOnMiss(t *testing.T) {
	t.Parallel()
	ch, _ := newHandlerTestChannel(t)
	ch.memberCache.Set("g1", "u_a", "Alice")
	// "Alice said" — only "Alice" matches; "said" is not a member.
	got := ch.wrapBareMentions(context.Background(), "g1", "Hello @Alice said something")
	want := "Hello @[Alice] said something"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestWrapBareMentions_NoMatch_LeftAlone(t *testing.T) {
	t.Parallel()
	ch, _ := newHandlerTestChannel(t)
	ch.memberCache.Set("g1", "u_a", "Alice")
	got := ch.wrapBareMentions(context.Background(), "g1", "ping @Bob")
	if got != "ping @Bob" {
		t.Fatalf("unknown name should pass through; got %q", got)
	}
}

func TestWrapBareMentions_ReservedSkipped(t *testing.T) {
	t.Parallel()
	ch, _ := newHandlerTestChannel(t)
	for _, in := range []string{"@all check this", "@everyone listen up", "@All hello"} {
		got := ch.wrapBareMentions(context.Background(), "g1", in)
		if strings.Contains(got, "@[") {
			t.Errorf("%q got wrapped: %q", in, got)
		}
	}
}

func TestWrapBareMentions_InsideMarkerNoDoubleWrap(t *testing.T) {
	t.Parallel()
	ch, _ := newHandlerTestChannel(t)
	ch.memberCache.Set("g1", "u_a", "Alice")
	// Already a marker — must not be re-wrapped.
	got := ch.wrapBareMentions(context.Background(), "g1", "Hi @[Alice] there")
	if got != "Hi @[Alice] there" {
		t.Fatalf("marker double-wrapped: %q", got)
	}
}

func TestWrapBareMentions_EmailNotMatched(t *testing.T) {
	t.Parallel()
	ch, _ := newHandlerTestChannel(t)
	ch.memberCache.Set("g1", "u_a", "Alice")
	// alice@example.com — @example must NOT match a member named Alice.
	got := ch.wrapBareMentions(context.Background(), "g1", "send to alice@example.com")
	if strings.Contains(got, "@[") {
		t.Fatalf("email matched: %q", got)
	}
}

func TestWrapBareMentions_DiacriticsRoundTrip(t *testing.T) {
	t.Parallel()
	ch, _ := newHandlerTestChannel(t)
	ch.memberCache.Set("g1", "u_d", "Đức")
	got := ch.wrapBareMentions(context.Background(), "g1", "Cảm ơn @Đức nhé")
	want := "Cảm ơn @[Đức] nhé"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestWrapBareMentions_StartOfText(t *testing.T) {
	t.Parallel()
	ch, _ := newHandlerTestChannel(t)
	ch.memberCache.Set("g1", "u_a", "Alice")
	got := ch.wrapBareMentions(context.Background(), "g1", "@Alice good morning")
	if got != "@[Alice] good morning" {
		t.Fatalf("start-of-text not wrapped: %q", got)
	}
}

func TestWrapBareMentions_AmbiguousLeftAlone(t *testing.T) {
	t.Parallel()
	ch, _ := newHandlerTestChannel(t)
	ch.memberCache.Set("g1", "u_1", "Hà Lan")
	ch.memberCache.Set("g1", "u_2", "hà lan") // same normalized name
	got := ch.wrapBareMentions(context.Background(), "g1", "ping @Hà Lan now")
	if strings.Contains(got, "@[") {
		t.Fatalf("ambiguous match was wrapped: %q", got)
	}
}
