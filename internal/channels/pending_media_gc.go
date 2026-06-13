package channels

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

const (
	pendingMediaDefaultMaxAge     = 72 * time.Hour
	pendingMediaDefaultGCInterval = 1 * time.Hour
)

// PendingMediaDefaultMaxAge returns the fallback max-age when config is nil/zero.
func PendingMediaDefaultMaxAge() time.Duration { return pendingMediaDefaultMaxAge }

// PendingMediaDefaultInterval returns the fallback sweep interval when config is nil/zero.
func PendingMediaDefaultInterval() time.Duration { return pendingMediaDefaultGCInterval }

// PendingMediaGCSettings carries live-readable GC parameters.
type PendingMediaGCSettings struct {
	Enabled  bool
	MaxAge   time.Duration
	Interval time.Duration
}

// StartPendingMediaGC sweeps the shared durable media dir using live settings
// so config.patch applies without restart. referenced (optional) yields the set
// of paths still referenced by a pending message; those are never deleted, only
// aged-out orphans are. If referenced errors, the sweep is skipped (fail-safe).
func StartPendingMediaGC(ctx context.Context, dir string, settings func() PendingMediaGCSettings, referenced func(context.Context) (map[string]struct{}, error)) {
	if dir == "" || settings == nil {
		return
	}
	go func() {
		for {
			s := settings()
			if s.Enabled && s.MaxAge > 0 {
				ref, ok := map[string]struct{}{}, true
				if referenced != nil {
					var err error
					if ref, err = referenced(ctx); err != nil {
						slog.Warn("pending_media_gc: skip sweep — cannot list referenced paths", "error", err)
						ok = false
					}
				}
				if ok {
					sweepPendingMedia(dir, s.MaxAge, ref)
				}
			}
			interval := s.Interval
			if interval <= 0 {
				interval = pendingMediaDefaultGCInterval
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			}
		}
	}()
}

// sweepPendingMedia removes regular files in dir that are older than maxAge AND
// not referenced by any pending message. Best-effort; returns the count removed.
// A nil referenced map means "nothing referenced" (pure age-based).
func sweepPendingMedia(dir string, maxAge time.Duration, referenced map[string]struct{}) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		full := filepath.Join(dir, e.Name())
		if _, ok := referenced[full]; ok {
			continue // still pending consumption — keep for later use
		}
		info, err := e.Info()
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if rmErr := os.Remove(full); rmErr != nil && !os.IsNotExist(rmErr) {
				slog.Warn("pending_media_gc: remove failed", "file", e.Name(), "error", rmErr)
				continue
			}
			removed++
		}
	}
	if removed > 0 {
		slog.Info("pending_media_gc: swept aged unreferenced files", "dir", dir, "removed", removed, "max_age", maxAge.String())
	}
	return removed
}
