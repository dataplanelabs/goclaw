package tools

import (
	"context"
	"strings"
	"testing"
)

func TestDailyToolLimiter_CountIncrRollover(t *testing.T) {
	d := NewDailyToolLimiter()
	if d.Count("k") != 0 {
		t.Fatal("fresh count should be 0")
	}
	d.Incr("k")
	d.Incr("k")
	if got := d.Count("k"); got != 2 {
		t.Fatalf("count = %d, want 2", got)
	}
	if d.Count("other") != 0 {
		t.Fatal("unrelated key should be 0")
	}
	// simulate a stale day → next access resets all counters
	d.day = "2000-01-01"
	if got := d.Count("k"); got != 0 {
		t.Fatalf("after day rollover count = %d, want 0", got)
	}
}

func TestDailyLimitFor(t *testing.T) {
	mk := func(j string) context.Context {
		return WithTenantToolSettings(context.Background(), BuiltinToolSettings{"create_image": []byte(j)})
	}
	tests := []struct {
		name      string
		ctx       context.Context
		tool      string
		wantLimit int
		wantScope string
	}{
		{"configured channel", mk(`{"daily_limit":10,"daily_limit_scope":"channel"}`), "create_image", 10, "channel"},
		{"default scope", mk(`{"daily_limit":5}`), "create_image", 5, "channel"},
		{"agent scope + coexists with chain", mk(`{"daily_limit":3,"daily_limit_scope":"agent","providers":["gemini"]}`), "create_image", 3, "agent"},
		{"zero disables", mk(`{"daily_limit":0}`), "create_image", 0, ""},
		{"absent field", mk(`{"providers":["gemini"]}`), "create_image", 0, ""},
		{"tool not configured", mk(`{"daily_limit":9}`), "create_video", 0, ""},
		{"no settings", context.Background(), "create_image", 0, ""},
		{"bad json", mk(`{not json`), "create_image", 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, scope := dailyLimitFor(tt.ctx, tt.tool)
			if limit != tt.wantLimit || scope != tt.wantScope {
				t.Errorf("dailyLimitFor = (%d,%q), want (%d,%q)", limit, scope, tt.wantLimit, tt.wantScope)
			}
		})
	}
}

func TestDailyScopeKey(t *testing.T) {
	ctx := context.Background()
	cases := map[string][]string{ // scope → substrings the key must contain
		"channel": {"create_image|channel|", "zalo-shtp"},
		"agent":   {"create_image|agent|"},
		"thread":  {"create_image|thread|", "zalo-shtp", "g123"},
		"tool":    {"create_image|tool|"},
		"":        {"create_image|channel|"}, // default
	}
	for scope, wantParts := range cases {
		key := dailyScopeKey(ctx, "create_image", scope, "zalo-shtp", "g123")
		for _, p := range wantParts {
			if !strings.Contains(key, p) {
				t.Errorf("scope %q key %q missing %q", scope, key, p)
			}
		}
	}
	// channel scope must NOT include the chatID (per-instance, all groups combined)
	if k := dailyScopeKey(ctx, "create_image", "channel", "zalo-shtp", "g123"); strings.Contains(k, "g123") {
		t.Errorf("channel scope must not be per-thread: %q", k)
	}
}
