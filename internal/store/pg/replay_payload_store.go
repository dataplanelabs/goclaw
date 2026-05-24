package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PGReplayPayloadStore implements store.ReplayPayloadStore on Postgres.
type PGReplayPayloadStore struct {
	db *sql.DB
}

func NewPGReplayPayloadStore(db *sql.DB) *PGReplayPayloadStore {
	return &PGReplayPayloadStore{db: db}
}

func (s *PGReplayPayloadStore) Capture(ctx context.Context, traceID uuid.UUID, sessionKey string, envelope []byte) error {
	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		return errors.New("replay_payload: tenant_id missing in ctx")
	}
	// ON CONFLICT covers the rare double-capture (retry after partial write).
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO trace_replay_payloads (trace_id, tenant_id, session_key, payload, payload_version, oversize, byte_size)
		 VALUES ($1, $2, $3, $4, $5, false, $6)
		 ON CONFLICT (trace_id) DO NOTHING`,
		traceID, tenantID, sessionKey, envelope, store.CurrentReplayPayloadVersion, len(envelope))
	return err
}

func (s *PGReplayPayloadStore) CaptureOversize(ctx context.Context, traceID uuid.UUID, sessionKey string, byteSize int) error {
	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		return errors.New("replay_payload: tenant_id missing in ctx")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO trace_replay_payloads (trace_id, tenant_id, session_key, payload, payload_version, oversize, byte_size)
		 VALUES ($1, $2, $3, NULL, $4, true, $5)
		 ON CONFLICT (trace_id) DO NOTHING`,
		traceID, tenantID, sessionKey, store.CurrentReplayPayloadVersion, byteSize)
	return err
}

func (s *PGReplayPayloadStore) Get(ctx context.Context, traceID uuid.UUID) (*store.ReplayPayloadRow, error) {
	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		return nil, sql.ErrNoRows
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT trace_id, tenant_id, session_key, payload, payload_version, oversize, byte_size, created_at
		 FROM trace_replay_payloads WHERE trace_id = $1 AND tenant_id = $2`,
		traceID, tenantID)
	var r store.ReplayPayloadRow
	var payload sql.NullString
	if err := row.Scan(&r.TraceID, &r.TenantID, &r.SessionKey, &payload, &r.Version, &r.Oversize, &r.ByteSize, &r.CreatedAt); err != nil {
		return nil, err
	}
	if payload.Valid {
		r.Payload = []byte(payload.String)
	}
	return &r, nil
}

func (s *PGReplayPayloadStore) DropForSession(ctx context.Context, sessionKey string, before time.Time) (int, error) {
	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		return 0, errors.New("replay_payload: tenant_id missing in ctx")
	}
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM trace_replay_payloads
		 WHERE session_key = $1 AND created_at < $2 AND tenant_id = $3`,
		sessionKey, before, tenantID)
	if err != nil {
		return 0, fmt.Errorf("drop replay payloads: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
