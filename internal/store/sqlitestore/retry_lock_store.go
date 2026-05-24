//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type SQLiteRetryLockStore struct {
	db *sql.DB
}

func NewSQLiteRetryLockStore(db *sql.DB) *SQLiteRetryLockStore {
	return &SQLiteRetryLockStore{db: db}
}

// Acquire is implemented as txn: SELECT existing → expired-or-missing → INSERT/UPDATE.
// SQLite lacks `INTERVAL`; we compare ISO-8601 text timestamps directly.
func (s *SQLiteRetryLockStore) Acquire(ctx context.Context, traceID, lockedBy uuid.UUID, ttl time.Duration) (bool, error) {
	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		return false, errors.New("retry_lock: tenant_id missing in ctx")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	cutoff := time.Now().UTC().Add(-ttl).Format("2006-01-02T15:04:05.000Z")

	var existingTrace string
	var existingLockedAt string
	err = tx.QueryRowContext(ctx,
		`SELECT trace_id, locked_at FROM trace_retry_locks
		 WHERE trace_id = ? AND tenant_id = ?`,
		traceID, tenantID).Scan(&existingTrace, &existingLockedAt)
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO trace_retry_locks (trace_id, tenant_id, locked_by, locked_at)
			 VALUES (?, ?, ?, ?)`,
			traceID, tenantID, lockedBy, now); err != nil {
			return false, err
		}
	case err != nil:
		return false, err
	default:
		if existingLockedAt >= cutoff {
			return false, tx.Commit()
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE trace_retry_locks SET locked_at = ?, locked_by = ?
			 WHERE trace_id = ? AND tenant_id = ?`,
			now, lockedBy, traceID, tenantID); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *SQLiteRetryLockStore) Release(ctx context.Context, traceID uuid.UUID) error {
	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		return errors.New("retry_lock: tenant_id missing in ctx")
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM trace_retry_locks WHERE trace_id = ? AND tenant_id = ?`,
		traceID, tenantID)
	return err
}
