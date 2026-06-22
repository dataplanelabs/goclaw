package store

import (
	"context"
	"time"
)

// Habit task statuses.
const (
	HabitStatusPending = "pending"
	HabitStatusDone    = "done"
	HabitStatusSkipped = "skipped"
)

// HabitScope is the (tenant, agent, user) key every habit-checklist query is
// isolated by — the same keys the dispatcher cron and completion tool resolve.
type HabitScope struct {
	TenantID string
	AgentID  string
	UserID   string
}

// HabitEntry is one habit task instance for one user on one local calendar day.
type HabitEntry struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenantId"`
	AgentID        string     `json:"agentId"`
	UserID         string     `json:"userId"`
	PlanDate       string     `json:"planDate"`       // "YYYY-MM-DD" in user TZ
	TaskKey        string     `json:"taskKey"`        // stable slug: guzheng|piano|run|...
	Title          string     `json:"title"`          // display label for the nudge
	ScheduledLocal string     `json:"scheduledLocal"` // "HH:MM" local; "" = anytime-today
	Status         string     `json:"status"`         // pending|done|skipped
	NudgeCount     int        `json:"nudgeCount"`
	LastNudgedAt   *time.Time `json:"lastNudgedAt,omitempty"`
	CompletedAt    *time.Time `json:"completedAt,omitempty"`
	CompletionNote string     `json:"completionNote,omitempty"`
}

// HabitChecklistStore is the deterministic per-user/day habit state the single
// coach-dispatcher cron gates on (so the LLM never decides "is X done today?").
type HabitChecklistStore interface {
	// Seed idempotently inserts a task row for the day. An existing row (same
	// unique key) is left untouched — completion/nudge progress is preserved.
	Seed(ctx context.Context, e HabitEntry) error

	// List returns all rows for the user/day, ordered by scheduled_local.
	List(ctx context.Context, scope HabitScope, planDate string) ([]HabitEntry, error)

	// ListPendingDue returns pending tasks that are due now: scheduled_local is
	// empty (anytime) OR <= nowLocalHHMM ("HH:MM"). This is the dispatcher gate.
	ListPendingDue(ctx context.Context, scope HabitScope, planDate, nowLocalHHMM string) ([]HabitEntry, error)

	// MarkDone flips a pending/skipped task to done (idempotent). Reports whether
	// a matching row existed.
	MarkDone(ctx context.Context, scope HabitScope, planDate, taskKey, note string) (bool, error)

	// MarkSkipped flips a task to skipped.
	MarkSkipped(ctx context.Context, scope HabitScope, planDate, taskKey string) (bool, error)

	// IncrementNudge bumps nudge_count and last_nudged_at for the given tasks,
	// called after a nudge is actually delivered (drives N-ticks escalation).
	IncrementNudge(ctx context.Context, scope HabitScope, planDate string, taskKeys []string, at time.Time) error
}
