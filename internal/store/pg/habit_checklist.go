package pg

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PGHabitChecklistStore is the Postgres-backed habit checklist (habit_checklist_entries).
type PGHabitChecklistStore struct {
	db *sql.DB
}

func NewPGHabitChecklistStore(db *sql.DB) *PGHabitChecklistStore {
	return &PGHabitChecklistStore{db: db}
}

func (s *PGHabitChecklistStore) Seed(ctx context.Context, e store.HabitEntry) error {
	id := e.ID
	if id == "" {
		id = uuid.Must(uuid.NewV7()).String()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO habit_checklist_entries
		 (id, tenant_id, agent_id, user_id, plan_date, task_key, title, scheduled_local, status)
		 VALUES ($1, $2::uuid, $3::uuid, $4, $5::date, $6, $7, NULLIF($8,''), 'pending')
		 ON CONFLICT (tenant_id, agent_id, user_id, plan_date, task_key) DO NOTHING`,
		id, e.TenantID, e.AgentID, e.UserID, e.PlanDate, e.TaskKey, e.Title, e.ScheduledLocal)
	if err != nil {
		return fmt.Errorf("habit seed: %w", err)
	}
	return nil
}

func (s *PGHabitChecklistStore) List(ctx context.Context, sc store.HabitScope, planDate string) ([]store.HabitEntry, error) {
	return s.query(ctx,
		`SELECT id, tenant_id, agent_id, user_id, plan_date::text, task_key, title,
		        COALESCE(scheduled_local,''), status, nudge_count, last_nudged_at, completed_at, COALESCE(completion_note,'')
		 FROM habit_checklist_entries
		 WHERE tenant_id=$1::uuid AND agent_id=$2::uuid AND user_id=$3 AND plan_date=$4::date
		 ORDER BY scheduled_local NULLS LAST, task_key`,
		sc.TenantID, sc.AgentID, sc.UserID, planDate)
}

func (s *PGHabitChecklistStore) ListPendingDue(ctx context.Context, sc store.HabitScope, planDate, nowLocalHHMM string) ([]store.HabitEntry, error) {
	return s.query(ctx,
		`SELECT id, tenant_id, agent_id, user_id, plan_date::text, task_key, title,
		        COALESCE(scheduled_local,''), status, nudge_count, last_nudged_at, completed_at, COALESCE(completion_note,'')
		 FROM habit_checklist_entries
		 WHERE tenant_id=$1::uuid AND agent_id=$2::uuid AND user_id=$3 AND plan_date=$4::date
		   AND status='pending'
		   AND (scheduled_local IS NULL OR scheduled_local='' OR scheduled_local <= $5)
		 ORDER BY scheduled_local NULLS LAST, task_key`,
		sc.TenantID, sc.AgentID, sc.UserID, planDate, nowLocalHHMM)
}

func (s *PGHabitChecklistStore) MarkDone(ctx context.Context, sc store.HabitScope, planDate, taskKey, note string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE habit_checklist_entries
		 SET status='done', completed_at=COALESCE(completed_at, NOW()),
		     completion_note=COALESCE(NULLIF($5,''), completion_note), updated_at=NOW()
		 WHERE tenant_id=$1::uuid AND agent_id=$2::uuid AND user_id=$3 AND plan_date=$4::date AND task_key=$6`,
		sc.TenantID, sc.AgentID, sc.UserID, planDate, note, taskKey)
	if err != nil {
		return false, fmt.Errorf("habit mark_done: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *PGHabitChecklistStore) MarkSkipped(ctx context.Context, sc store.HabitScope, planDate, taskKey string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE habit_checklist_entries
		 SET status='skipped', updated_at=NOW()
		 WHERE tenant_id=$1::uuid AND agent_id=$2::uuid AND user_id=$3 AND plan_date=$4::date AND task_key=$5
		   AND status <> 'done'`,
		sc.TenantID, sc.AgentID, sc.UserID, planDate, taskKey)
	if err != nil {
		return false, fmt.Errorf("habit mark_skipped: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *PGHabitChecklistStore) IncrementNudge(ctx context.Context, sc store.HabitScope, planDate string, taskKeys []string, at time.Time) error {
	if len(taskKeys) == 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE habit_checklist_entries
		 SET nudge_count=nudge_count+1, last_nudged_at=$5, updated_at=NOW()
		 WHERE tenant_id=$1::uuid AND agent_id=$2::uuid AND user_id=$3 AND plan_date=$4::date
		   AND status='pending' AND task_key = ANY($6)`,
		sc.TenantID, sc.AgentID, sc.UserID, planDate, at.UTC(), pqStringArray(taskKeys))
	if err != nil {
		return fmt.Errorf("habit increment_nudge: %w", err)
	}
	return nil
}

func (s *PGHabitChecklistStore) query(ctx context.Context, q string, args ...any) ([]store.HabitEntry, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("habit query: %w", err)
	}
	defer rows.Close()
	var out []store.HabitEntry
	for rows.Next() {
		var e store.HabitEntry
		var lastNudged, completed sql.NullTime
		if err := rows.Scan(&e.ID, &e.TenantID, &e.AgentID, &e.UserID, &e.PlanDate, &e.TaskKey, &e.Title,
			&e.ScheduledLocal, &e.Status, &e.NudgeCount, &lastNudged, &completed, &e.CompletionNote); err != nil {
			return nil, fmt.Errorf("habit scan: %w", err)
		}
		if lastNudged.Valid {
			e.LastNudgedAt = &lastNudged.Time
		}
		if completed.Valid {
			e.CompletedAt = &completed.Time
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
