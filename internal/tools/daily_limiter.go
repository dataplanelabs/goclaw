package tools

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// DailyToolLimiter enforces a per-day cap on specific tools (e.g. create_image),
// scoped per channel-instance / agent / thread / globally. In-memory and reset on
// UTC day rollover (prior days' counters are discarded). A soft guardrail for
// cost/abuse — a process restart clears counts, which is acceptable here (not billing).
type DailyToolLimiter struct {
	mu     sync.Mutex
	day    string
	counts map[string]int
}

func NewDailyToolLimiter() *DailyToolLimiter {
	return &DailyToolLimiter{counts: make(map[string]int)}
}

// rollover clears all counters when the UTC day changes. Caller holds the lock.
func (d *DailyToolLimiter) rollover() {
	today := time.Now().UTC().Format("2006-01-02")
	if today != d.day {
		d.day = today
		d.counts = make(map[string]int)
	}
}

// Count returns today's count for key.
func (d *DailyToolLimiter) Count(key string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.rollover()
	return d.counts[key]
}

// Incr increments today's count for key.
func (d *DailyToolLimiter) Incr(key string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.rollover()
	d.counts[key]++
}

// dailyLimitCfg is the subset of a tool's builtin settings JSON used for limiting.
// Lives alongside any other tool-specific settings (e.g. provider chain).
type dailyLimitCfg struct {
	DailyLimit int    `json:"daily_limit"`
	Scope      string `json:"daily_limit_scope"` // channel (default) | agent | thread | tool
}

// dailyLimitFor reads the per-tool daily_limit from the merged builtin tool
// settings (tenant override layered over global). Returns 0 when unconfigured.
func dailyLimitFor(ctx context.Context, tool string) (limit int, scope string) {
	settings := BuiltinToolSettingsFromCtx(ctx)
	if settings == nil {
		return 0, ""
	}
	raw, ok := settings[tool]
	if !ok || len(raw) == 0 {
		return 0, ""
	}
	var cfg dailyLimitCfg
	if err := json.Unmarshal(raw, &cfg); err != nil || cfg.DailyLimit <= 0 {
		return 0, ""
	}
	scope = cfg.Scope
	if scope == "" {
		scope = "channel"
	}
	return cfg.DailyLimit, scope
}

// dailyScopeKey builds the counter key for the requested scope. "channel" =
// per channel-instance (the bot deployment); "thread" = per chat/group;
// "agent" = per agent; "tool" = global-per-tenant.
func dailyScopeKey(ctx context.Context, tool, scope, channel, chatID string) string {
	tenant := store.TenantIDFromContext(ctx).String()
	switch scope {
	case "agent":
		return tool + "|agent|" + tenant + "|" + store.AgentIDFromContext(ctx).String()
	case "thread":
		if channel == "" {
			channel = ToolChannelFromCtx(ctx)
		}
		if chatID == "" {
			chatID = ToolChatIDFromCtx(ctx)
		}
		return tool + "|thread|" + tenant + "|" + channel + "|" + chatID
	case "tool":
		return tool + "|tool|" + tenant
	default: // "channel" — per channel-instance
		if channel == "" {
			channel = ToolChannelFromCtx(ctx)
		}
		return tool + "|channel|" + tenant + "|" + channel
	}
}
