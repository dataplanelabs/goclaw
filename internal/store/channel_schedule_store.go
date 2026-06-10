package store

import (
	"context"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/channels/schedule"
)

// ThreadSchedule is the per-thread schedule override stored in
// channel_thread_schedules. Replaces the instance-level schedule for that
// thread when present (no merge — first match wins).
type ThreadSchedule struct {
	ChannelInstanceID string
	ThreadKey         string
	Schedule          *schedule.Schedule
	ExpiresAt         *time.Time
	Reason            string
	CreatedBy         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ChannelScheduleStore manages standby schedules on channel_instances and per-thread overrides.
type ChannelScheduleStore interface {
	// ResolveInstanceIDByName maps (tenantID, channelName) → channelInstanceID.
	// Returns ("", nil) when no match (no info leak across tenants).
	ResolveInstanceIDByName(ctx context.Context, tenantID, channelName string) (string, error)

	GetInstanceSchedule(ctx context.Context, channelInstanceID string) (*schedule.Schedule, error)
	SetInstanceSchedule(ctx context.Context, channelInstanceID string, s *schedule.Schedule) error
	DeleteInstanceSchedule(ctx context.Context, channelInstanceID string) error

	ListThreadSchedules(ctx context.Context, channelInstanceID string) ([]ThreadSchedule, error)
	GetThreadSchedule(ctx context.Context, channelInstanceID, threadKey string) (*ThreadSchedule, error)
	SetThreadSchedule(ctx context.Context, t ThreadSchedule) error
	DeleteThreadSchedule(ctx context.Context, channelInstanceID, threadKey string) error

	PurgeExpiredThreadSchedules(ctx context.Context, now time.Time) (int64, error)
}
