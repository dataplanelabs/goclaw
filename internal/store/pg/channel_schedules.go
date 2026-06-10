package pg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/channels/schedule"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type PGChannelScheduleStore struct {
	db *sql.DB
}

func NewPGChannelScheduleStore(db *sql.DB) *PGChannelScheduleStore {
	return &PGChannelScheduleStore{db: db}
}

func (s *PGChannelScheduleStore) ResolveInstanceIDByName(ctx context.Context, tenantID, channelName string) (string, error) {
	if tenantID == "" || channelName == "" {
		return "", nil
	}
	var id string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM channel_instances WHERE tenant_id = $1 AND name = $2`,
		tenantID, channelName).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve instance id: %w", err)
	}
	return id, nil
}

func (s *PGChannelScheduleStore) GetInstanceSchedule(ctx context.Context, channelInstanceID string) (*schedule.Schedule, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT silence_schedule FROM channel_instances WHERE id = $1`, channelInstanceID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get instance schedule: %w", err)
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var sc schedule.Schedule
	if err := json.Unmarshal(raw, &sc); err != nil {
		return nil, fmt.Errorf("unmarshal schedule: %w", err)
	}
	return &sc, nil
}

func (s *PGChannelScheduleStore) SetInstanceSchedule(ctx context.Context, channelInstanceID string, sc *schedule.Schedule) error {
	if sc == nil {
		return s.DeleteInstanceSchedule(ctx, channelInstanceID)
	}
	b, err := json.Marshal(sc)
	if err != nil {
		return fmt.Errorf("marshal schedule: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE channel_instances SET silence_schedule = $1::jsonb, updated_at = NOW() WHERE id = $2`,
		string(b), channelInstanceID)
	return err
}

func (s *PGChannelScheduleStore) DeleteInstanceSchedule(ctx context.Context, channelInstanceID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE channel_instances SET silence_schedule = NULL, updated_at = NOW() WHERE id = $1`,
		channelInstanceID)
	return err
}

func (s *PGChannelScheduleStore) ListThreadSchedules(ctx context.Context, channelInstanceID string) ([]store.ThreadSchedule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT channel_instance_id, thread_key, schedule, expires_at, reason, created_by, created_at, updated_at
		   FROM channel_thread_schedules
		  WHERE channel_instance_id = $1
		  ORDER BY thread_key`, channelInstanceID)
	if err != nil {
		return nil, fmt.Errorf("list thread schedules: %w", err)
	}
	defer rows.Close()
	var out []store.ThreadSchedule
	for rows.Next() {
		t, err := scanThreadSchedule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (s *PGChannelScheduleStore) GetThreadSchedule(ctx context.Context, channelInstanceID, threadKey string) (*store.ThreadSchedule, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT channel_instance_id, thread_key, schedule, expires_at, reason, created_by, created_at, updated_at
		   FROM channel_thread_schedules
		  WHERE channel_instance_id = $1 AND thread_key = $2`, channelInstanceID, threadKey)
	t, err := scanThreadSchedule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

func (s *PGChannelScheduleStore) SetThreadSchedule(ctx context.Context, t store.ThreadSchedule) error {
	if t.Schedule == nil {
		return fmt.Errorf("schedule required")
	}
	b, err := json.Marshal(t.Schedule)
	if err != nil {
		return fmt.Errorf("marshal schedule: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO channel_thread_schedules (channel_instance_id, thread_key, schedule, expires_at, reason, created_by, updated_at)
		 VALUES ($1, $2, $3::jsonb, $4, $5, $6, NOW())
		 ON CONFLICT (channel_instance_id, thread_key)
		 DO UPDATE SET schedule = EXCLUDED.schedule,
		               expires_at = EXCLUDED.expires_at,
		               reason = EXCLUDED.reason,
		               updated_at = NOW()`,
		t.ChannelInstanceID, t.ThreadKey, string(b), t.ExpiresAt, t.Reason, t.CreatedBy)
	return err
}

func (s *PGChannelScheduleStore) DeleteThreadSchedule(ctx context.Context, channelInstanceID, threadKey string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM channel_thread_schedules WHERE channel_instance_id = $1 AND thread_key = $2`,
		channelInstanceID, threadKey)
	return err
}

func (s *PGChannelScheduleStore) PurgeExpiredThreadSchedules(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM channel_thread_schedules WHERE expires_at IS NOT NULL AND expires_at < $1`, now)
	if err != nil {
		return 0, fmt.Errorf("purge expired: %w", err)
	}
	return res.RowsAffected()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanThreadSchedule(s scanner) (*store.ThreadSchedule, error) {
	var (
		t         store.ThreadSchedule
		raw       []byte
		expiresAt sql.NullTime
		reason    sql.NullString
		createdBy sql.NullString
	)
	if err := s.Scan(&t.ChannelInstanceID, &t.ThreadKey, &raw, &expiresAt, &reason, &createdBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	var sc schedule.Schedule
	if err := json.Unmarshal(raw, &sc); err != nil {
		return nil, fmt.Errorf("unmarshal schedule: %w", err)
	}
	t.Schedule = &sc
	if expiresAt.Valid {
		t.ExpiresAt = &expiresAt.Time
	}
	if reason.Valid {
		t.Reason = reason.String
	}
	if createdBy.Valid {
		t.CreatedBy = createdBy.String
	}
	return &t, nil
}
