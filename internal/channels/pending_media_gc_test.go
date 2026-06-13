package channels

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSweepPendingMedia_RemovesAgedKeepsFresh(t *testing.T) {
	dir := t.TempDir()
	maxAge := time.Hour

	old := filepath.Join(dir, "aged.jpg")
	fresh := filepath.Join(dir, "fresh.jpg")
	for _, p := range []string{old, fresh} {
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	// Age the old file past the cutoff.
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	// A subdir must be ignored.
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if got := sweepPendingMedia(dir, maxAge); got != 1 {
		t.Fatalf("removed = %d, want 1", got)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("aged file should have been removed")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("fresh file should remain: %v", err)
	}
}

func TestSweepPendingMedia_MissingDir(t *testing.T) {
	if got := sweepPendingMedia(filepath.Join(t.TempDir(), "nope"), time.Hour); got != 0 {
		t.Fatalf("missing dir removed = %d, want 0", got)
	}
}
