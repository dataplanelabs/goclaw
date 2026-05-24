package schedule

import (
	"testing"
	"time"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("LoadLocation(%q): %v", name, err)
	}
	return loc
}

func TestResolver_EmptyScheduleDefaultsActive(t *testing.T) {
	r, err := NewResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.ModeAt(time.Now()); got != ModeActive {
		t.Fatalf("nil schedule: want active, got %v", got)
	}

	r2, err := NewResolver(&Schedule{})
	if err != nil {
		t.Fatal(err)
	}
	if got := r2.ModeAt(time.Now()); got != ModeActive {
		t.Fatalf("empty schedule: want active, got %v", got)
	}
}

func TestResolver_DefaultModeApplies(t *testing.T) {
	r, _ := NewResolver(&Schedule{DefaultMode: ModeStandby})
	if got := r.ModeAt(time.Now()); got != ModeStandby {
		t.Fatalf("default standby: got %v", got)
	}
}

func TestResolver_RecurringWindow(t *testing.T) {
	saigon := mustLoc(t, "Asia/Saigon")
	s := &Schedule{
		DefaultMode: ModeActive,
		Windows: []Window{
			{Mode: ModeStandby, TZ: "Asia/Saigon", Weekday: "mon-fri", Start: "09:00", End: "17:00"},
		},
	}
	r, err := NewResolver(s)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		now  time.Time
		want Mode
	}{
		{"weekday inside", time.Date(2026, 5, 25, 10, 0, 0, 0, saigon), ModeStandby}, // Mon 10:00
		{"weekday before", time.Date(2026, 5, 25, 8, 59, 0, 0, saigon), ModeActive},
		{"weekday after", time.Date(2026, 5, 25, 17, 0, 0, 0, saigon), ModeActive},
		{"weekend", time.Date(2026, 5, 23, 10, 0, 0, 0, saigon), ModeActive}, // Sat
		{"utc inside", time.Date(2026, 5, 25, 3, 0, 0, 0, time.UTC), ModeStandby}, // 10 ICT
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := r.ModeAt(tc.now); got != tc.want {
				t.Fatalf("ModeAt(%v): want %v, got %v", tc.now, tc.want, got)
			}
		})
	}
}

func TestResolver_CrossMidnightWindow(t *testing.T) {
	saigon := mustLoc(t, "Asia/Saigon")
	s := &Schedule{
		Windows: []Window{
			{Mode: ModeStandby, TZ: "Asia/Saigon", Weekday: "fri", Start: "22:00", End: "06:00"},
		},
	}
	r, _ := NewResolver(s)
	cases := []struct {
		name string
		now  time.Time
		want Mode
	}{
		{"fri 23:00", time.Date(2026, 5, 29, 23, 0, 0, 0, saigon), ModeStandby},
		{"sat 02:00", time.Date(2026, 5, 30, 2, 0, 0, 0, saigon), ModeStandby},
		{"sat 06:00", time.Date(2026, 5, 30, 6, 0, 0, 0, saigon), ModeActive},
		{"fri 21:59", time.Date(2026, 5, 29, 21, 59, 0, 0, saigon), ModeActive},
		{"thu 23:00", time.Date(2026, 5, 28, 23, 0, 0, 0, saigon), ModeActive},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := r.ModeAt(tc.now); got != tc.want {
				t.Fatalf("ModeAt(%v): want %v, got %v", tc.now, tc.want, got)
			}
		})
	}
}

func TestResolver_OneShotWindow(t *testing.T) {
	from := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)
	r, _ := NewResolver(&Schedule{
		DefaultMode: ModeActive,
		Windows:     []Window{{Mode: ModeStandby, From: &from, Until: &until}},
	})
	cases := []struct {
		name string
		now  time.Time
		want Mode
	}{
		{"before", time.Date(2026, 6, 14, 23, 59, 0, 0, time.UTC), ModeActive},
		{"start", from, ModeStandby},
		{"mid", time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC), ModeStandby},
		{"end exclusive", until, ModeActive},
		{"after", time.Date(2026, 6, 22, 0, 0, 1, 0, time.UTC), ModeActive},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := r.ModeAt(tc.now); got != tc.want {
				t.Fatalf("ModeAt(%v): want %v, got %v", tc.now, tc.want, got)
			}
		})
	}
}

func TestResolver_OneShotBeatsRecurring(t *testing.T) {
	saigon := mustLoc(t, "Asia/Saigon")
	from := time.Date(2026, 5, 25, 0, 0, 0, 0, saigon)
	until := time.Date(2026, 5, 25, 23, 0, 0, 0, saigon)
	s := &Schedule{
		Windows: []Window{
			{Mode: ModeStandby, TZ: "Asia/Saigon", Weekday: "mon-fri", Start: "09:00", End: "17:00"},
			{Mode: ModeActive, From: &from, Until: &until}, // override Monday — back to active
		},
	}
	r, _ := NewResolver(s)
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, saigon)
	if got := r.ModeAt(now); got != ModeActive {
		t.Fatalf("one-shot active should beat recurring standby: got %v", got)
	}
}

func TestResolver_MultipleRecurringNonOverlap(t *testing.T) {
	saigon := mustLoc(t, "Asia/Saigon")
	s := &Schedule{
		Windows: []Window{
			{Mode: ModeStandby, TZ: "Asia/Saigon", Weekday: "mon", Start: "09:00", End: "12:00"},
			{Mode: ModeStandby, TZ: "Asia/Saigon", Weekday: "wed", Start: "14:00", End: "16:00"},
		},
	}
	r, _ := NewResolver(s)
	if got := r.ModeAt(time.Date(2026, 5, 25, 10, 0, 0, 0, saigon)); got != ModeStandby {
		t.Fatalf("mon morning: %v", got)
	}
	if got := r.ModeAt(time.Date(2026, 5, 27, 15, 0, 0, 0, saigon)); got != ModeStandby {
		t.Fatalf("wed afternoon: %v", got)
	}
	if got := r.ModeAt(time.Date(2026, 5, 26, 10, 0, 0, 0, saigon)); got != ModeActive {
		t.Fatalf("tue: %v", got)
	}
}

func TestResolver_InvalidScheduleReturnsError(t *testing.T) {
	_, err := NewResolver(&Schedule{Windows: []Window{{Mode: ModeStandby, Weekday: "funday", Start: "09:00", End: "17:00"}}})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
