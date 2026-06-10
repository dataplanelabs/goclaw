package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// RetryLockStore gates concurrent retries on the same failed trace using a
// short-TTL row. Acquire is idempotent across expiry — first caller wins,
// double-clickers within ttl get acquired=false.
type RetryLockStore interface {
	Acquire(ctx context.Context, traceID, lockedBy uuid.UUID, ttl time.Duration) (bool, error)
	Release(ctx context.Context, traceID uuid.UUID) error
}
