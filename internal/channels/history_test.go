package channels

import (
	"strings"
	"testing"
	"time"
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
