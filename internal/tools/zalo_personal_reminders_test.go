package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
)

func futureRFC3339(t *testing.T, offset time.Duration) string {
	t.Helper()
	return time.Now().Add(offset).UTC().Format(time.RFC3339)
}

func TestParseReminderStartTime(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		wantErr bool
		check   func(t *testing.T, got int64)
	}{
		{"", true, nil},
		{"not-a-time", true, nil},
		{"1700000000000", false, func(t *testing.T, got int64) {
			if got != 1700000000000 {
				t.Errorf("ms input round-trip: %d", got)
			}
		}},
		{"1700000000", false, func(t *testing.T, got int64) {
			if got != 1700000000*1000 {
				t.Errorf("sec→ms promotion: %d", got)
			}
		}},
		{"-1700000000", true, nil},
		{"0", true, nil},
		{"999999999999", false, func(t *testing.T, got int64) {
			if got != 999999999999*1000 {
				t.Errorf("just-below-1e12 should be sec→ms: %d", got)
			}
		}},
		{"1000000000000", false, func(t *testing.T, got int64) {
			if got != 1000000000000 {
				t.Errorf("at-1e12 should be raw ms: %d", got)
			}
		}},
		{"2026-05-25T09:00:00Z", false, func(t *testing.T, got int64) {
			wanted := time.Date(2026, 5, 25, 9, 0, 0, 0, time.UTC).UnixMilli()
			if got != wanted {
				t.Errorf("RFC3339 parse: got %d, want %d", got, wanted)
			}
		}},
	}
	for _, c := range cases {
		got, err := parseReminderStartTime(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("parseReminderStartTime(%q) err=%v want_err=%v", c.in, err, c.wantErr)
		}
		if err == nil && c.check != nil {
			c.check(t, got)
		}
	}
}

func TestCreateReminder_HappyPath_Group(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloPersonalAction{createReminderReturn: "rem-77"}
	tool := NewZaloPersonalCreateReminderTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(fake))

	res := tool.Execute(zpCtx(t), map[string]any{
		"thread_id":   "group-abc",
		"thread_type": "group",
		"title":       "team standup",
		"start_time":  futureRFC3339(t, 24*time.Hour),
		"repeat":      "daily",
		"pin_to_top":  true,
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.ForLLM)
	}
	if !fake.createReminderCall.isGroup {
		t.Errorf("thread_type=group must dispatch isGroup=true")
	}
	if fake.createReminderCall.threadID != "group-abc" {
		t.Errorf("threadID=%q", fake.createReminderCall.threadID)
	}
	if fake.createReminderCall.settings.Repeat != "daily" {
		t.Errorf("repeat=%q", fake.createReminderCall.settings.Repeat)
	}
	var out map[string]any
	_ = json.Unmarshal([]byte(res.ForLLM), &out)
	if out["reminder_id"] != "rem-77" {
		t.Errorf("reminder_id=%v", out["reminder_id"])
	}
}

func TestCreateReminder_DispatchesByThreadType(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloPersonalAction{createReminderReturn: "x"}
	tool := NewZaloPersonalCreateReminderTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(fake))
	tool.Execute(zpCtx(t), map[string]any{
		"thread_id": "user-1", "thread_type": "dm", "title": "ping",
		"start_time": futureRFC3339(t, time.Hour),
	})
	if fake.createReminderCall.isGroup {
		t.Errorf("thread_type=dm must dispatch isGroup=false")
	}
}

func TestCreateReminder_RejectsPastStartTime(t *testing.T) {
	t.Parallel()
	tool := NewZaloPersonalCreateReminderTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(&fakeZaloPersonalAction{}))
	res := tool.Execute(zpCtx(t), map[string]any{
		"thread_id": "g1", "thread_type": "group", "title": "x",
		"start_time": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
	})
	if !res.IsError || !strings.Contains(res.ForLLM, "60s in the future") {
		t.Errorf("want past-time error, got: %s", res.ForLLM)
	}
}

func TestCreateReminder_RejectsBadRepeat(t *testing.T) {
	t.Parallel()
	tool := NewZaloPersonalCreateReminderTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(&fakeZaloPersonalAction{}))
	res := tool.Execute(zpCtx(t), map[string]any{
		"thread_id": "g1", "thread_type": "group", "title": "x",
		"start_time": futureRFC3339(t, time.Hour),
		"repeat":     "yearly",
	})
	if !res.IsError || !strings.Contains(res.ForLLM, "repeat") {
		t.Errorf("want bad-repeat error, got: %s", res.ForLLM)
	}
}

func TestCreateReminder_RejectsBadThreadType(t *testing.T) {
	t.Parallel()
	tool := NewZaloPersonalCreateReminderTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(&fakeZaloPersonalAction{}))
	res := tool.Execute(zpCtx(t), map[string]any{
		"thread_id": "g1", "thread_type": "channel", "title": "x",
		"start_time": futureRFC3339(t, time.Hour),
	})
	if !res.IsError || !strings.Contains(res.ForLLM, "thread_type") {
		t.Errorf("want thread_type error, got: %s", res.ForLLM)
	}
}

func TestCreateReminder_RequiresTitle(t *testing.T) {
	t.Parallel()
	tool := NewZaloPersonalCreateReminderTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(&fakeZaloPersonalAction{}))
	res := tool.Execute(zpCtx(t), map[string]any{
		"thread_id": "g1", "thread_type": "group", "title": "  ",
		"start_time": futureRFC3339(t, time.Hour),
	})
	if !res.IsError || !strings.Contains(res.ForLLM, "title") {
		t.Errorf("want missing-title error, got: %s", res.ForLLM)
	}
}

func TestCreateReminder_WrongChannelType(t *testing.T) {
	t.Parallel()
	tool := NewZaloPersonalCreateReminderTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(&fakeZaloPersonalAction{}))
	ctx := context.Background()
	ctx = WithToolChannelType(ctx, "telegram")
	ctx = WithToolChannel(ctx, "tg-bot")
	res := tool.Execute(ctx, map[string]any{
		"thread_id": "g1", "thread_type": "group", "title": "x",
		"start_time": futureRFC3339(t, time.Hour),
	})
	if !res.IsError || !strings.Contains(res.ForLLM, "zalo_personal") {
		t.Errorf("want channel-type error, got: %s", res.ForLLM)
	}
}

func TestRemoveReminder_HappyPath(t *testing.T) {
	t.Parallel()
	fake := &fakeZaloPersonalAction{}
	tool := NewZaloPersonalRemoveReminderTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(fake))
	res := tool.Execute(zpCtx(t), map[string]any{
		"reminder_id": "rem-9",
		"group_id":    "g1",
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.ForLLM)
	}
	if fake.removeReminderCall.reminderID != "rem-9" || fake.removeReminderCall.groupID != "g1" {
		t.Errorf("call mismatch: %+v", fake.removeReminderCall)
	}
}

func TestRemoveReminder_MissingReminderID(t *testing.T) {
	t.Parallel()
	tool := NewZaloPersonalRemoveReminderTool()
	tool.SetZaloPersonalActionFn(zpFakeFn(&fakeZaloPersonalAction{}))
	res := tool.Execute(zpCtx(t), map[string]any{"group_id": "g1"})
	if !res.IsError || !strings.Contains(res.ForLLM, "reminder_id") {
		t.Errorf("want missing-reminder_id error, got: %s", res.ForLLM)
	}
}

func TestReminderToolsRequireZaloPersonalChannel(t *testing.T) {
	t.Parallel()
	tools := []ChannelAware{
		NewZaloPersonalCreateReminderTool(),
		NewZaloPersonalRemoveReminderTool(),
	}
	for _, tl := range tools {
		types := tl.RequiredChannelTypes()
		if len(types) != 1 || types[0] != channels.TypeZaloPersonal {
			t.Errorf("%T: RequiredChannelTypes()=%v", tl, types)
		}
	}
}

var _ = func() bool {
	var _ ZaloPersonalActionAware = (*ZaloPersonalCreateReminderTool)(nil)
	var _ ZaloPersonalActionAware = (*ZaloPersonalRemoveReminderTool)(nil)
	var _ ChannelAware = (*ZaloPersonalCreateReminderTool)(nil)
	var _ ChannelAware = (*ZaloPersonalRemoveReminderTool)(nil)
	return true
}()
