package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/channels/schedule"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// StandbyScheduleStore is the subset of store.ChannelScheduleStore the tool needs.
// Defined here so the tools pkg avoids a hard dep on the entire store interface
// for this single feature; satisfied by *pg.PGChannelScheduleStore and
// *sqlitestore.SQLiteChannelScheduleStore.
type StandbyScheduleStore interface {
	ResolveInstanceIDByName(ctx context.Context, tenantID, channelName string) (string, error)
	SetThreadSchedule(ctx context.Context, t store.ThreadSchedule) error
}

// EnterStandbyTool lets an agent self-pause replies in the current thread.
// One-shot window written to channel_thread_schedules with expires_at = now+duration.
type EnterStandbyTool struct {
	store  StandbyScheduleStore
	reload func(channelInstanceID string)
}

func NewEnterStandbyTool(s StandbyScheduleStore, reload func(string)) *EnterStandbyTool {
	return &EnterStandbyTool{store: s, reload: reload}
}

// SetReload wires a post-write callback (registry.Reload) so the editing pod
// sees the new schedule within ~ms instead of waiting for the 60s TTL refresh.
// Safe to call after registration; nil-clear is allowed for tests.
func (t *EnterStandbyTool) SetReload(reload func(channelInstanceID string)) {
	t.reload = reload
}

func (t *EnterStandbyTool) Name() string { return "enter_standby" }

func (t *EnterStandbyTool) Description() string {
	return i18n.T("en", i18n.StandbyToolDescription)
}

func (t *EnterStandbyTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"duration_seconds": map[string]any{
				"type":        "integer",
				"description": i18n.T("en", i18n.StandbyToolParamDuration),
			},
			"reason": map[string]any{
				"type":        "string",
				"description": i18n.T("en", i18n.StandbyToolParamReason),
			},
		},
		"required": []string{"duration_seconds"},
	}
}

func (t *EnterStandbyTool) Execute(ctx context.Context, args map[string]any) *Result {
	locale := store.LocaleFromContext(ctx)
	channelName := ToolChannelFromCtx(ctx)
	chatID := ToolChatIDFromCtx(ctx)
	peerKind := ToolPeerKindFromCtx(ctx)
	if channelName == "" || chatID == "" {
		return ErrorResult(i18n.T(locale, i18n.StandbyErrorNoChannelCtx))
	}
	duration, _ := numAsInt(args["duration_seconds"])
	if duration < 60 || duration > 86400 {
		return ErrorResult(i18n.T(locale, i18n.StandbyErrorInvalidDuration))
	}
	reason, _ := args["reason"].(string)

	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		return ErrorResult(i18n.T(locale, i18n.StandbyErrorNoChannelCtx))
	}
	if t.store == nil {
		return ErrorResult("enter_standby: store not wired")
	}

	instanceID, err := t.store.ResolveInstanceIDByName(ctx, tenantID.String(), channelName)
	if err != nil {
		return ErrorResult(fmt.Sprintf("resolve channel instance: %v", err))
	}
	if instanceID == "" {
		return ErrorResult(i18n.T(locale, i18n.StandbyErrorNoChannelCtx))
	}

	from := time.Now()
	until := from.Add(time.Duration(duration) * time.Second)
	sc := &schedule.Schedule{
		DefaultMode: schedule.ModeStandby,
		Windows: []schedule.Window{{
			ID:    "self-pause-" + uuid.NewString()[:8],
			Mode:  schedule.ModeStandby,
			From:  &from,
			Until: &until,
		}},
	}
	threadKey := buildThreadKey(peerKind, chatID)
	row := store.ThreadSchedule{
		ChannelInstanceID: instanceID,
		ThreadKey:         threadKey,
		Schedule:          sc,
		ExpiresAt:         &until,
		Reason:            reason,
		CreatedBy:         "agent",
	}
	if err := t.store.SetThreadSchedule(ctx, row); err != nil {
		return ErrorResult(fmt.Sprintf("set thread schedule: %v", err))
	}
	if t.reload != nil {
		t.reload(instanceID)
	}
	return NewResult(i18n.T(locale, i18n.StandbyEntered, humanDuration(duration), reasonOrNone(reason)))
}

// buildThreadKey mirrors pipeline.BuildStandbyThreadKey — kept duplicate here to
// avoid a tools→pipeline import cycle. Format MUST stay in sync.
func buildThreadKey(kind, chatID string) string {
	if kind == "" {
		kind = "direct"
	}
	return fmt.Sprintf("%s:%s", kind, chatID)
}

func numAsInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}

func humanDuration(seconds int) string {
	d := time.Duration(seconds) * time.Second
	switch {
	case d >= time.Hour:
		return fmt.Sprintf("%.1fh", d.Hours())
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%ds", seconds)
	}
}

func reasonOrNone(r string) string {
	if r == "" {
		return "—"
	}
	return r
}
