package schedule

import (
	"testing"
	"time"
)

// US DST forward shift in 2026: Sun Mar 8 02:00 → 03:00 (America/New_York).
// US DST backward shift in 2026: Sun Nov 1 02:00 → 01:00.
func TestResolver_DST_ForwardShift(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	s := &Schedule{
		Windows: []Window{
			{Mode: ModeStandby, TZ: "America/New_York", Weekday: "mon-fri", Start: "09:00", End: "17:00"},
		},
	}
	r, _ := NewResolver(s)
	// The Monday following forward shift (2026-03-09): 09:00 local should still resolve as standby.
	mon0900 := time.Date(2026, 3, 9, 9, 0, 0, 0, ny)
	if got := r.ModeAt(mon0900); got != ModeStandby {
		t.Fatalf("post-DST-forward Monday 09:00: got %v want standby", got)
	}
	if got := r.ModeAt(time.Date(2026, 3, 9, 8, 59, 0, 0, ny)); got != ModeActive {
		t.Fatalf("08:59 boundary: got %v want active", got)
	}
}

func TestResolver_DST_BackwardShift(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	// Cross-midnight window straddling DST backward shift Sun Nov 1 02:00 → 01:00.
	// Window: Sat 22:00 → Sun 06:00. Verify Sun 01:30 (ambiguous local hour) resolves inside window once.
	s := &Schedule{
		Windows: []Window{
			{Mode: ModeStandby, TZ: "America/New_York", Weekday: "sat", Start: "22:00", End: "06:00"},
		},
	}
	r, _ := NewResolver(s)
	// 2026-11-01 01:30 NY — single-instant lookup, no double-fire by virtue of single time.Time.
	t1 := time.Date(2026, 11, 1, 1, 30, 0, 0, ny)
	if got := r.ModeAt(t1); got != ModeStandby {
		t.Fatalf("DST fall-back 01:30: got %v want standby", got)
	}
	if got := r.ModeAt(time.Date(2026, 11, 1, 6, 0, 0, 0, ny)); got != ModeActive {
		t.Fatalf("06:00 boundary: got %v want active", got)
	}
}

func TestResolver_TZPropagation_UTCInput(t *testing.T) {
	s := &Schedule{
		Windows: []Window{
			{Mode: ModeStandby, TZ: "Asia/Saigon", Weekday: "mon", Start: "09:00", End: "17:00"},
		},
	}
	r, _ := NewResolver(s)
	// 2026-05-25 02:00 UTC == 2026-05-25 09:00 Asia/Saigon (UTC+7)
	if got := r.ModeAt(time.Date(2026, 5, 25, 2, 0, 0, 0, time.UTC)); got != ModeStandby {
		t.Fatalf("utc 02:00 (Saigon 09:00 Mon): got %v", got)
	}
}
