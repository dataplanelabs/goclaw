package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

const habitTimeFmt = time.RFC3339

// SQLiteHabitChecklistStore is the SQLite-backed habit checklist.
type SQLiteHabitChecklistStore struct {
	db *sql.DB
}

func NewSQLiteHabitChecklistStore(db *sql.DB) *SQLiteHabitChecklistStore {
	return &SQLiteHabitChecklistStore{db: db}
}

func (s *SQLiteHabitChecklistStore) Seed(ctx context.Context, e store.HabitEntry) error {
	id := e.ID
	if id == "" {
		id = uuid.Must(uuid.NewV7()).String()
	}
	now := time.Now().UTC().Format(habitTimeFmt)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO habit_checklist_entries
		 (id, tenant_id, agent_id, user_id, plan_date, task_key, title, scheduled_local, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?,''), 'pending', ?, ?)
		 ON CONFLICT (tenant_id, agent_id, user_id, plan_date, task_key) DO NOTHING`,
		id, e.TenantID, e.AgentID, e.UserID, e.PlanDate, e.TaskKey, e.Title, e.ScheduledLocal, now, now)
	if err != nil {
		return fmt.Errorf("habit seed: %w", err)
	}
	return nil
}

func (s *SQLiteHabitChecklistStore) List(ctx context.Context, sc store.HabitScope, planDate string) ([]store.HabitEntry, error) {
	return s.query(ctx,
		`SELECT id, tenant_id, agent_id, user_id, plan_date, task_key, title,
		        COALESCE(scheduled_local,''), status, nudge_count, last_nudged_at, completed_at, COALESCE(completion_note,'')
		 FROM habit_checklist_entries
		 WHERE tenant_id=? AND agent_id=? AND user_id=? AND plan_date=?
		 ORDER BY scheduled_local IS NULL, scheduled_local, task_key`,
		sc.TenantID, sc.AgentID, sc.UserID, planDate)
}

func (s *SQLiteHabitChecklistStore) ListPendingDue(ctx context.Context, sc store.HabitScope, planDate, nowLocalHHMM string) ([]store.HabitEntry, error) {
	return s.query(ctx,
		`SELECT id, tenant_id, agent_id, user_id, plan_date, task_key, title,
		        COALESCE(scheduled_local,''), status, nudge_count, last_nudged_at, completed_at, COALESCE(completion_note,'')
		 FROM habit_checklist_entries
		 WHERE tenant_id=? AND agent_id=? AND user_id=? AND plan_date=?
		   AND status='pending'
		   AND (scheduled_local IS NULL OR scheduled_local='' OR scheduled_local <= ?)
		 ORDER BY scheduled_local IS NULL, scheduled_local, task_key`,
		sc.TenantID, sc.AgentID, sc.UserID, planDate, nowLocalHHMM)
}

func (s *SQLiteHabitChecklistStore) MarkDone(ctx context.Context, sc store.HabitScope, planDate, taskKey, note string) (bool, error) {
	now := time.Now().UTC().Format(habitTimeFmt)
	res, err := s.db.ExecContext(ctx,
		`UPDATE habit_checklist_entries
		 SET status='done', completed_at=?, completion_note=NULLIF(?,''), updated_at=?
		 WHERE tenant_id=? AND agent_id=? AND user_id=? AND plan_date=? AND task_key=?`,
		now, note, now, sc.TenantID, sc.AgentID, sc.UserID, planDate, taskKey)
	if err != nil {
		return false, fmt.Errorf("habit mark_done: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *SQLiteHabitChecklistStore) MarkSkipped(ctx context.Context, sc store.HabitScope, planDate, taskKey string) (bool, error) {
	now := time.Now().UTC().Format(habitTimeFmt)
	res, err := s.db.ExecContext(ctx,
		`UPDATE habit_checklist_entries SET status='skipped', updated_at=?
		 WHERE tenant_id=? AND agent_id=? AND user_id=? AND plan_date=? AND task_key=?`,
		now, sc.TenantID, sc.AgentID, sc.UserID, planDate, taskKey)
	if err != nil {
		return false, fmt.Errorf("habit mark_skipped: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *SQLiteHabitChecklistStore) IncrementNudge(ctx context.Context, sc store.HabitScope, planDate string, taskKeys []string, at time.Time) error {
	if len(taskKeys) == 0 {
		return nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(taskKeys)), ",")
	args := []any{at.UTC().Format(habitTimeFmt), time.Now().UTC().Format(habitTimeFmt), sc.TenantID, sc.AgentID, sc.UserID, planDate}
	for _, k := range taskKeys {
		args = append(args, k)
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE habit_checklist_entries SET nudge_count=nudge_count+1, last_nudged_at=?, updated_at=?
		 WHERE tenant_id=? AND agent_id=? AND user_id=? AND plan_date=? AND task_key IN (`+ph+`)`,
		args...)
	if err != nil {
		return fmt.Errorf("habit increment_nudge: %w", err)
	}
	return nil
}

func (s *SQLiteHabitChecklistStore) query(ctx context.Context, q string, args ...any) ([]store.HabitEntry, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("habit query: %w", err)
	}
	defer rows.Close()
	var out []store.HabitEntry
	for rows.Next() {
		var e store.HabitEntry
		var lastNudged, completed sql.NullString
		if err := rows.Scan(&e.ID, &e.TenantID, &e.AgentID, &e.UserID, &e.PlanDate, &e.TaskKey, &e.Title,
			&e.ScheduledLocal, &e.Status, &e.NudgeCount, &lastNudged, &completed, &e.CompletionNote); err != nil {
			return nil, fmt.Errorf("habit scan: %w", err)
		}
		e.LastNudgedAt = parseHabitTime(lastNudged)
		e.CompletedAt = parseHabitTime(completed)
		out = append(out, e)
	}
	return out, rows.Err()
}

func parseHabitTime(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	if t, err := time.Parse(habitTimeFmt, ns.String); err == nil {
		return &t
	}
	return nil
}
