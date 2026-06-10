package sqlitestore

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

type SQLiteChannelScheduleStore struct {
	db *sql.DB
}

func NewSQLiteChannelScheduleStore(db *sql.DB) *SQLiteChannelScheduleStore {
	return &SQLiteChannelScheduleStore{db: db}
}

func (s *SQLiteChannelScheduleStore) ResolveInstanceIDByName(ctx context.Context, tenantID, channelName string) (string, error) {
	if tenantID == "" || channelName == "" {
		return "", nil
	}
	var id string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM channel_instances WHERE tenant_id = ? AND name = ?`,
		tenantID, channelName).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve instance id: %w", err)
	}
	return id, nil
}

func (s *SQLiteChannelScheduleStore) GetInstanceSchedule(ctx context.Context, channelInstanceID string) (*schedule.Schedule, error) {
	var raw sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT silence_schedule FROM channel_instances WHERE id = ?`, channelInstanceID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get instance schedule: %w", err)
	}
	if !raw.Valid || raw.String == "" {
		return nil, nil
	}
	var sc schedule.Schedule
	if err := json.Unmarshal([]byte(raw.String), &sc); err != nil {
		return nil, fmt.Errorf("unmarshal schedule: %w", err)
	}
	return &sc, nil
}

func (s *SQLiteChannelScheduleStore) SetInstanceSchedule(ctx context.Context, channelInstanceID string, sc *schedule.Schedule) error {
	if sc == nil {
		return s.DeleteInstanceSchedule(ctx, channelInstanceID)
	}
	b, err := json.Marshal(sc)
	if err != nil {
		return fmt.Errorf("marshal schedule: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE channel_instances SET silence_schedule = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
		string(b), channelInstanceID)
	return err
}

func (s *SQLiteChannelScheduleStore) DeleteInstanceSchedule(ctx context.Context, channelInstanceID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE channel_instances SET silence_schedule = NULL, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
		channelInstanceID)
	return err
}

func (s *SQLiteChannelScheduleStore) ListThreadSchedules(ctx context.Context, channelInstanceID string) ([]store.ThreadSchedule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT channel_instance_id, thread_key, schedule, expires_at, reason, created_by, created_at, updated_at
		   FROM channel_thread_schedules
		  WHERE channel_instance_id = ?
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

func (s *SQLiteChannelScheduleStore) GetThreadSchedule(ctx context.Context, channelInstanceID, threadKey string) (*store.ThreadSchedule, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT channel_instance_id, thread_key, schedule, expires_at, reason, created_by, created_at, updated_at
		   FROM channel_thread_schedules
		  WHERE channel_instance_id = ? AND thread_key = ?`, channelInstanceID, threadKey)
	t, err := scanThreadSchedule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

func (s *SQLiteChannelScheduleStore) SetThreadSchedule(ctx context.Context, t store.ThreadSchedule) error {
	if t.Schedule == nil {
		return fmt.Errorf("schedule required")
	}
	b, err := json.Marshal(t.Schedule)
	if err != nil {
		return fmt.Errorf("marshal schedule: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO channel_thread_schedules (channel_instance_id, thread_key, schedule, expires_at, reason, created_by, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(channel_instance_id, thread_key)
		 DO UPDATE SET schedule = excluded.schedule,
		               expires_at = excluded.expires_at,
		               reason = excluded.reason,
		               updated_at = CURRENT_TIMESTAMP`,
		t.ChannelInstanceID, t.ThreadKey, string(b), t.ExpiresAt, t.Reason, t.CreatedBy)
	return err
}

func (s *SQLiteChannelScheduleStore) DeleteThreadSchedule(ctx context.Context, channelInstanceID, threadKey string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM channel_thread_schedules WHERE channel_instance_id = ? AND thread_key = ?`,
		channelInstanceID, threadKey)
	return err
}

func (s *SQLiteChannelScheduleStore) PurgeExpiredThreadSchedules(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM channel_thread_schedules WHERE expires_at IS NOT NULL AND expires_at < ?`, now)
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
		raw       string
		expiresAt sql.NullTime
		reason    sql.NullString
		createdBy sql.NullString
	)
	if err := s.Scan(&t.ChannelInstanceID, &t.ThreadKey, &raw, &expiresAt, &reason, &createdBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	var sc schedule.Schedule
	if err := json.Unmarshal([]byte(raw), &sc); err != nil {
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
