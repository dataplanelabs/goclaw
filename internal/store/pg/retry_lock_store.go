package pg

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type PGRetryLockStore struct {
	db *sql.DB
}

func NewPGRetryLockStore(db *sql.DB) *PGRetryLockStore {
	return &PGRetryLockStore{db: db}
}

func (s *PGRetryLockStore) Acquire(ctx context.Context, traceID, lockedBy uuid.UUID, ttl time.Duration) (bool, error) {
	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		return false, errors.New("retry_lock: tenant_id missing in ctx")
	}
	var acquired bool
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO trace_retry_locks (trace_id, tenant_id, locked_by, locked_at)
		 VALUES ($1, $2, $3, NOW())
		 ON CONFLICT (trace_id) DO UPDATE
		     SET locked_at = NOW(), locked_by = EXCLUDED.locked_by
		     WHERE trace_retry_locks.locked_at < NOW() - make_interval(secs => $4::int)
		 RETURNING locked_by = $3`,
		traceID, tenantID, lockedBy, int(ttl.Seconds())).Scan(&acquired)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return acquired, nil
}

func (s *PGRetryLockStore) Release(ctx context.Context, traceID uuid.UUID) error {
	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		return errors.New("retry_lock: tenant_id missing in ctx")
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM trace_retry_locks WHERE trace_id = $1 AND tenant_id = $2`,
		traceID, tenantID)
	return err
}
