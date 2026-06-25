package channels

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// 08:59 UTC is 15:59 in Asia/Ho_Chi_Minh (+07).
var fixedUTC = time.Date(2026, 1, 2, 8, 59, 0, 0, time.UTC)

func TestBuildContextLocalizesBufferTimestamp(t *testing.T) {
	ph := NewPendingHistory()
	ph.SetTimezone("Asia/Ho_Chi_Minh")
	key := "group:testchan:900000000000000001"
	ph.Record(key, HistoryEntry{Sender: "Writer One", Body: "hello", Timestamp: fixedUTC}, 50)

	out := ph.BuildContext(key, "current", 50)
	if !strings.Contains(out, "[2026-01-02 15:59]") {
		t.Fatalf("expected localized 2026-01-02 15:59 in buffer, got:\n%s", out)
	}
	if strings.Contains(out, "[2026-01-02 08:59]") {
		t.Fatalf("buffer must not show raw UTC 08:59, got:\n%s", out)
	}
	if !strings.Contains(out, CurrentMessageMarker) {
		t.Fatalf("expected current-message marker, got:\n%s", out)
	}
}

// Regression: a group photo posted by sharer A must be attributed to A, not to
// the later sender B who mentions the bot. The historical image is re-attached to
// B's turn, so the context must carry an explicit sharer note.
func TestBuildContextAttributesHistoricalMediaToSharer(t *testing.T) {
	ph := NewPendingHistory()
	ph.SetTimezone("Asia/Ho_Chi_Minh")
	key := "group:testchan:900000000000000010"
	ph.Record(key, HistoryEntry{
		Sender:    "Photo Sharer",
		SenderID:  "100000000000000001",
		Body:      `<media:image path="/tmp/cafe.jpg">`,
		Media:     []string{"/tmp/cafe.jpg"},
		Timestamp: fixedUTC,
	}, 50)

	current := "[From: Mention Sender (uid:100000000000000002)]\nhey bot"
	out, paths := ph.BuildContextAndCollectMedia(key, current, 50)

	if !strings.Contains(out, "Photo Sharer shared an image/file at 15:59") {
		t.Fatalf("expected sharer attribution for Photo Sharer, got:\n%s", out)
	}
	if !strings.Contains(out, "not to the current message's sender") {
		t.Fatalf("expected explicit anti-misattribution caveat, got:\n%s", out)
	}
	if strings.Contains(out, "Mention Sender shared") {
		t.Fatalf("current sender must not be credited with the image, got:\n%s", out)
	}
	if len(paths) != 1 || paths[0] != "/tmp/cafe.jpg" {
		t.Fatalf("expected the historical media path collected, got: %v", paths)
	}
}

// The note must survive the Slack-style ordering where CollectMedia nils entry
// Media before BuildContext formats — detection falls back to the Body media tag.
func TestMediaNoteSurvivesCollectMediaNiling(t *testing.T) {
	ph := NewPendingHistory()
	key := "group:testchan:900000000000000011"
	ph.Record(key, HistoryEntry{
		Sender:    "Photo Sharer",
		Body:      `caption here<media:image path="/tmp/x.jpg">`,
		Media:     []string{"/tmp/x.jpg"},
		Timestamp: fixedUTC,
	}, 50)

	if got := ph.CollectMedia(key); len(got) != 1 {
		t.Fatalf("expected 1 collected path, got: %v", got)
	}
	out := ph.BuildContext(key, "current", 50)
	if !strings.Contains(out, "Photo Sharer shared an image/file") {
		t.Fatalf("expected sharer note after CollectMedia niled Media, got:\n%s", out)
	}
}

func TestBuildContextNoMediaNoteWhenTextOnly(t *testing.T) {
	ph := NewPendingHistory()
	key := "group:testchan:900000000000000012"
	ph.Record(key, HistoryEntry{Sender: "Writer One", Body: "just text", Timestamp: fixedUTC}, 50)

	out := ph.BuildContext(key, "current", 50)
	if strings.Contains(out, "Media note") {
		t.Fatalf("text-only history must not emit a media note, got:\n%s", out)
	}
}

func TestBuildContextDefaultsToUTC(t *testing.T) {
	ph := NewPendingHistory()
	key := "group:testchan:900000000000000002"
	ph.Record(key, HistoryEntry{Sender: "Writer One", Body: "hi", Timestamp: fixedUTC}, 50)

	out := ph.BuildContext(key, "current", 50)
	if !strings.Contains(out, "[2026-01-02 08:59]") {
		t.Fatalf("expected UTC 08:59 fallback, got:\n%s", out)
	}
}

func TestSetTimezoneInvalidKeepsUTC(t *testing.T) {
	ph := NewPendingHistory()
	ph.SetTimezone("Not/AZone")
	key := "group:testchan:900000000000000003"
	ph.Record(key, HistoryEntry{Sender: "Writer One", Body: "hi", Timestamp: fixedUTC}, 50)

	out := ph.BuildContext(key, "current", 50)
	if !strings.Contains(out, "[2026-01-02 08:59]") {
		t.Fatalf("invalid tz should fall back to UTC, got:\n%s", out)
	}
}

// fakeStore is an in-memory stub for store.PendingMessageStore used in tests.
type fakeStore struct {
	batches []store.PendingMessage
}

func (f *fakeStore) AppendBatch(_ context.Context, msgs []store.PendingMessage) error {
	f.batches = append(f.batches, msgs...)
	return nil
}
func (f *fakeStore) ListByKey(_ context.Context, _, _ string) ([]store.PendingMessage, error) {
	return f.batches, nil
}
func (f *fakeStore) DeleteByKey(_ context.Context, _, _ string) error          { return nil }
func (f *fakeStore) Compact(_ context.Context, _ []uuid.UUID, _ *store.PendingMessage) error {
	return nil
}
func (f *fakeStore) DeleteStale(_ context.Context, _ time.Duration) (int64, error) { return 0, nil }
func (f *fakeStore) ListGroups(_ context.Context) ([]store.PendingMessageGroup, error) {
	return nil, nil
}
func (f *fakeStore) CountAll(_ context.Context) (int64, error)                   { return 0, nil }
func (f *fakeStore) CountByKey(_ context.Context, _, _ string) (int, error)      { return 0, nil }
func (f *fakeStore) ResolveGroupTitles(_ context.Context, _ []store.PendingMessageGroup) (map[string]string, error) {
	return nil, nil
}
func (f *fakeStore) ListReferencedMediaPaths(_ context.Context) ([]string, error) { return nil, nil }

func TestDurableMediaDirMovesFilesAndPersistsToStore(t *testing.T) {
	durableDir := t.TempDir()
	tmpFile := filepath.Join(t.TempDir(), "inbound.pdf")
	if err := os.WriteFile(tmpFile, []byte("PDF content"), 0644); err != nil {
		t.Fatal(err)
	}

	fs := &fakeStore{}
	ph := NewPersistentHistory("telegram", fs, uuid.New())
	ph.SetDurableMediaDir(durableDir)
	ph.StartFlusher()
	defer ph.StopFlusher()

	key := "group:testchan:900000000000000004"
	ph.Record(key, HistoryEntry{
		Sender: "Writer One",
		Body:   "here is a PDF",
		Media:  []string{tmpFile},
	}, 50)

	// Flush synchronously so the fake store has the batch.
	ph.flushNow()

	// The temp file must have been moved to durableDir.
	entries := ph.GetEntries(key)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if len(entries[0].Media) != 1 {
		t.Fatalf("expected 1 media path, got %d", len(entries[0].Media))
	}
	durablePath := entries[0].Media[0]
	if !strings.HasPrefix(durablePath, durableDir) {
		t.Fatalf("media path %q not in durable dir %q", durablePath, durableDir)
	}
	if _, err := os.Stat(durablePath); err != nil {
		t.Fatalf("durable file missing: %v", err)
	}
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Fatalf("original temp file should have been removed")
	}

	// The DB row must carry the durable path in MediaPaths.
	if len(fs.batches) != 1 {
		t.Fatalf("expected 1 DB row, got %d", len(fs.batches))
	}
	if len(fs.batches[0].MediaPaths) != 1 || fs.batches[0].MediaPaths[0] != durablePath {
		t.Fatalf("DB row MediaPaths mismatch: %v", fs.batches[0].MediaPaths)
	}
}

func TestLoadFromDBReconstructsMediaPaths(t *testing.T) {
	durablePath := "/durable/.pending-media/some-uuid.pdf"

	fs := &fakeStore{
		batches: []store.PendingMessage{
			{
				ChannelName:   "telegram",
				HistoryKey:    "group:testchan:900000000000000005",
				Sender:        "Writer One",
				Body:          "document msg",
				MediaPaths:    []string{durablePath},
				CreatedAt:     time.Now(),
			},
		},
	}
	ph := NewPersistentHistory("telegram", fs, uuid.New())

	entries := ph.loadFromDB("group:testchan:900000000000000005")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if len(entries[0].Media) != 1 || entries[0].Media[0] != durablePath {
		t.Fatalf("expected Media[0]=%q, got %v", durablePath, entries[0].Media)
	}
}

func TestDurableMediaDirIgnoredWhenEmpty(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "inbound.pdf")
	if err := os.WriteFile(tmpFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	ph := NewPendingHistory() // RAM-only, no durableMediaDir
	key := "group:testchan:900000000000000006"
	ph.Record(key, HistoryEntry{
		Sender: "Writer One",
		Body:   "msg",
		Media:  []string{tmpFile},
	}, 50)

	entries := ph.GetEntries(key)
	if len(entries) == 0 || len(entries[0].Media) == 0 {
		t.Fatal("expected entry with media in RAM-only mode")
	}
	// Original path preserved (no move).
	if entries[0].Media[0] != tmpFile {
		t.Fatalf("expected original path %q, got %q", tmpFile, entries[0].Media[0])
	}
}
