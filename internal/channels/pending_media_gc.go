package channels

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

const (
	// pendingMediaMaxAge bounds how long a durable .pending-media file lives
	// without being consumed. Files survive a restart (unlike the old /tmp
	// behavior), so a quiet/never-mentioned group or an orphan from an
	// ungraceful shutdown would otherwise grow the PVC unbounded. 72h covers
	// the realistic "send media now, @mention days later" window.
	pendingMediaMaxAge     = 72 * time.Hour
	pendingMediaGCInterval = 1 * time.Hour
)

// StartPendingMediaGC runs a background sweep that removes aged files from the
// shared durable media dir until ctx is done. The dir is shared across all
// channels, so it is started once (from the gateway) rather than per-channel.
func StartPendingMediaGC(ctx context.Context, dir string) {
	if dir == "" {
		return
	}
	go func() {
		t := time.NewTicker(pendingMediaGCInterval)
		defer t.Stop()
		sweepPendingMedia(dir, pendingMediaMaxAge)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				sweepPendingMedia(dir, pendingMediaMaxAge)
			}
		}
	}()
}

// sweepPendingMedia removes regular files in dir whose modtime is older than
// maxAge. Best-effort; returns the count removed.
func sweepPendingMedia(dir string, maxAge time.Duration) int {
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
		info, err := e.Info()
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if rmErr := os.Remove(filepath.Join(dir, e.Name())); rmErr != nil && !os.IsNotExist(rmErr) {
				slog.Warn("pending_media_gc: remove failed", "file", e.Name(), "error", rmErr)
				continue
			}
			removed++
		}
	}
	if removed > 0 {
		slog.Info("pending_media_gc: swept aged files", "dir", dir, "removed", removed, "max_age", maxAge.String())
	}
	return removed
}
