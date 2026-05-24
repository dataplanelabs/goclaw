package tools

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/channels"
)

// parseReminderStartTime accepts ISO-8601 / RFC3339 or Unix epoch (sec or ms).
// Returns Unix milliseconds. Boundary is value-based, not digit-count:
// anything below 1e12 is treated as seconds (year 2001-32668 in sec → ~1969-2001 in ms),
// at-or-above 1e12 is treated as ms.
func parseReminderStartTime(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("start_time is required")
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n <= 0 {
			return 0, fmt.Errorf("start_time must be positive epoch")
		}
		if n < 1_000_000_000_000 {
			return n * 1000, nil
		}
		return n, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0, fmt.Errorf("start_time must be RFC3339 or Unix epoch: %w", err)
	}
	return t.UnixMilli(), nil
}

// --- create_reminder ---

type ZaloPersonalCreateReminderTool struct{ actionFn ZaloPersonalActionFn }

func NewZaloPersonalCreateReminderTool() *ZaloPersonalCreateReminderTool {
	return &ZaloPersonalCreateReminderTool{}
}

func (t *ZaloPersonalCreateReminderTool) SetZaloPersonalActionFn(fn ZaloPersonalActionFn) {
	t.actionFn = fn
}

func (t *ZaloPersonalCreateReminderTool) RequiredChannelTypes() []string {
	return []string{channels.TypeZaloPersonal}
}

func (t *ZaloPersonalCreateReminderTool) Name() string { return "zalo_personal_create_reminder" }

func (t *ZaloPersonalCreateReminderTool) Description() string {
	return "Schedule a Zalo Personal reminder in a group or DM. start_time accepts RFC3339 (2026-05-25T09:00:00+07:00) or Unix epoch seconds/ms. Returns the reminder ID."
}

func (t *ZaloPersonalCreateReminderTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"thread_id":   map[string]any{"type": "string", "description": "Group ID for groups, peer UID for DMs"},
			"thread_type": map[string]any{"type": "string", "enum": []string{"group", "dm"}, "description": "Required — Zalo uses different endpoints for group vs DM reminders"},
			"title":       map[string]any{"type": "string", "description": "What to remind about"},
			"start_time":  map[string]any{"type": "string", "description": "RFC3339 timestamp or Unix epoch (seconds or ms)"},
			"repeat":      map[string]any{"type": "string", "enum": []string{"none", "daily", "weekly", "monthly"}, "description": "Repeat cadence (default none)"},
			"pin_to_top":  map[string]any{"type": "boolean", "description": "Group-only: also pin reminder to group board"},
			"emoji":       map[string]any{"type": "string", "description": "Optional emoji (default ⏰)"},
		},
		"required": []string{"thread_id", "thread_type", "title", "start_time"},
	}
}

func (t *ZaloPersonalCreateReminderTool) Execute(ctx context.Context, args map[string]any) *Result {
	handle, errRes := resolveZaloPersonalAction(ctx, t.actionFn)
	if errRes != nil {
		return errRes
	}
	threadID := strings.TrimSpace(argString(args, "thread_id"))
	if threadID == "" {
		return ErrorResult("thread_id is required")
	}
	threadType := strings.ToLower(strings.TrimSpace(argString(args, "thread_type")))
	if threadType != "group" && threadType != "dm" {
		return ErrorResult(`thread_type must be "group" or "dm"`)
	}
	title := strings.TrimSpace(argString(args, "title"))
	if title == "" {
		return ErrorResult("title is required")
	}
	if len(title) > 1000 {
		return ErrorResult("title exceeds 1000 chars")
	}
	startMs, err := parseReminderStartTime(argString(args, "start_time"))
	if err != nil {
		return ErrorResult(err.Error())
	}
	if startMs < time.Now().Add(60*time.Second).UnixMilli() {
		return ErrorResult("start_time must be at least 60s in the future")
	}
	repeat := strings.ToLower(strings.TrimSpace(argString(args, "repeat")))
	switch repeat {
	case "", "none", "daily", "weekly", "monthly":
	default:
		return ErrorResult(fmt.Sprintf("repeat must be one of none|daily|weekly|monthly (got %q)", repeat))
	}
	settings := ZaloReminderSettings{
		Title:     title,
		StartTime: startMs,
		Repeat:    repeat,
		PinToTop:  argBool(args, "pin_to_top"),
		Emoji:     strings.TrimSpace(argString(args, "emoji")),
	}
	reminderID, err := handle.CreateReminder(ctx, threadID, threadType == "group", settings)
	if err != nil {
		return ErrorResult(fmt.Sprintf("create reminder: %v", err))
	}
	return jsonResult(map[string]any{
		"reminder_id":    reminderID,
		"thread_id":      threadID,
		"thread_type":    threadType,
		"start_time_ms":  startMs,
		"status":         "scheduled",
	})
}

// --- remove_reminder ---

type ZaloPersonalRemoveReminderTool struct{ actionFn ZaloPersonalActionFn }

func NewZaloPersonalRemoveReminderTool() *ZaloPersonalRemoveReminderTool {
	return &ZaloPersonalRemoveReminderTool{}
}

func (t *ZaloPersonalRemoveReminderTool) SetZaloPersonalActionFn(fn ZaloPersonalActionFn) {
	t.actionFn = fn
}

func (t *ZaloPersonalRemoveReminderTool) RequiredChannelTypes() []string {
	return []string{channels.TypeZaloPersonal}
}

func (t *ZaloPersonalRemoveReminderTool) Name() string { return "zalo_personal_remove_reminder" }

func (t *ZaloPersonalRemoveReminderTool) Description() string {
	return "Remove a previously-created Zalo Personal reminder by ID."
}

func (t *ZaloPersonalRemoveReminderTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"reminder_id": map[string]any{"type": "string", "description": "Reminder ID returned by create_reminder"},
			"group_id":    map[string]any{"type": "string", "description": "Group ID where the reminder lives (empty string for DM reminders)"},
		},
		"required": []string{"reminder_id"},
	}
}

func (t *ZaloPersonalRemoveReminderTool) Execute(ctx context.Context, args map[string]any) *Result {
	handle, errRes := resolveZaloPersonalAction(ctx, t.actionFn)
	if errRes != nil {
		return errRes
	}
	reminderID := strings.TrimSpace(argString(args, "reminder_id"))
	if reminderID == "" {
		return ErrorResult("reminder_id is required")
	}
	groupID := strings.TrimSpace(argString(args, "group_id"))
	if err := handle.RemoveReminder(ctx, reminderID, groupID); err != nil {
		return ErrorResult(fmt.Sprintf("remove reminder: %v", err))
	}
	return jsonResult(map[string]any{"status": "removed", "reminder_id": reminderID})
}
