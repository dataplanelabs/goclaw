//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// SQLiteReplayPayloadStore implements store.ReplayPayloadStore on SQLite.
type SQLiteReplayPayloadStore struct {
	db *sql.DB
}

func NewSQLiteReplayPayloadStore(db *sql.DB) *SQLiteReplayPayloadStore {
	return &SQLiteReplayPayloadStore{db: db}
}

func (s *SQLiteReplayPayloadStore) Capture(ctx context.Context, traceID uuid.UUID, sessionKey string, envelope []byte) error {
	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		return errors.New("replay_payload: tenant_id missing in ctx")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO trace_replay_payloads (trace_id, tenant_id, session_key, payload, payload_version, oversize, byte_size)
		 VALUES (?, ?, ?, ?, ?, 0, ?)`,
		traceID, tenantID, sessionKey, string(envelope), store.CurrentReplayPayloadVersion, len(envelope))
	return err
}

func (s *SQLiteReplayPayloadStore) CaptureOversize(ctx context.Context, traceID uuid.UUID, sessionKey string, byteSize int) error {
	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		return errors.New("replay_payload: tenant_id missing in ctx")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO trace_replay_payloads (trace_id, tenant_id, session_key, payload, payload_version, oversize, byte_size)
		 VALUES (?, ?, ?, NULL, ?, 1, ?)`,
		traceID, tenantID, sessionKey, store.CurrentReplayPayloadVersion, byteSize)
	return err
}

func (s *SQLiteReplayPayloadStore) Get(ctx context.Context, traceID uuid.UUID) (*store.ReplayPayloadRow, error) {
	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		return nil, sql.ErrNoRows
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT trace_id, tenant_id, session_key, payload, payload_version, oversize, byte_size, created_at
		 FROM trace_replay_payloads WHERE trace_id = ? AND tenant_id = ?`,
		traceID, tenantID)
	var r store.ReplayPayloadRow
	var payload sql.NullString
	var oversize int
	var createdAt sqliteTime
	if err := row.Scan(&r.TraceID, &r.TenantID, &r.SessionKey, &payload, &r.Version, &oversize, &r.ByteSize, &createdAt); err != nil {
		return nil, err
	}
	if payload.Valid {
		r.Payload = []byte(payload.String)
	}
	r.Oversize = oversize != 0
	r.CreatedAt = createdAt.Time
	return &r, nil
}

func (s *SQLiteReplayPayloadStore) DropForSession(ctx context.Context, sessionKey string, before time.Time) (int, error) {
	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		return 0, errors.New("replay_payload: tenant_id missing in ctx")
	}
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM trace_replay_payloads
		 WHERE session_key = ? AND created_at < ? AND tenant_id = ?`,
		sessionKey, before, tenantID)
	if err != nil {
		return 0, fmt.Errorf("drop replay payloads: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
