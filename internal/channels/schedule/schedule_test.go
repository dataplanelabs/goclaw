package schedule

import (
	"encoding/json"
	"testing"
	"time"
)

func TestValidate_DefaultMode(t *testing.T) {
	cases := []struct {
		name    string
		mode    Mode
		wantErr bool
	}{
		{"empty ok", "", false},
		{"active ok", ModeActive, false},
		{"standby ok", ModeStandby, false},
		{"bogus rejected", "snoozing", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(&Schedule{DefaultMode: tc.mode})
			if tc.wantErr != (err != nil) {
				t.Fatalf("Validate(%q): want err=%v, got %v", tc.mode, tc.wantErr, err)
			}
		})
	}
}

func TestValidate_Window(t *testing.T) {
	from := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	until := from.Add(2 * time.Hour)
	cases := []struct {
		name    string
		w       Window
		wantErr bool
	}{
		{"recurring ok", Window{Mode: ModeStandby, Weekday: "mon-fri", Start: "09:00", End: "17:00"}, false},
		{"recurring list ok", Window{Mode: ModeStandby, Weekday: "mon,wed,fri", Start: "09:00", End: "17:00"}, false},
		{"one-shot ok", Window{Mode: ModeStandby, From: &from, Until: &until}, false},
		{"bad tz", Window{Mode: ModeStandby, TZ: "Mars/Olympus", Weekday: "mon", Start: "09:00", End: "17:00"}, true},
		{"bad time", Window{Mode: ModeStandby, Weekday: "mon", Start: "9am", End: "17:00"}, true},
		{"bad weekday", Window{Mode: ModeStandby, Weekday: "funday", Start: "09:00", End: "17:00"}, true},
		{"empty", Window{Mode: ModeStandby}, true},
		{"mixed", Window{Mode: ModeStandby, Weekday: "mon", Start: "09:00", End: "17:00", From: &from, Until: &until}, true},
		{"inverted oneshot", Window{Mode: ModeStandby, From: &until, Until: &from}, true},
		{"list-range mix bad", Window{Mode: ModeStandby, Weekday: "mon-fri,sat", Start: "09:00", End: "17:00"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(&Schedule{Windows: []Window{tc.w}})
			if tc.wantErr != (err != nil) {
				t.Fatalf("Validate: want err=%v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestSchedule_JSONRoundTrip(t *testing.T) {
	from := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	until := from.Add(7 * 24 * time.Hour)
	original := &Schedule{
		DefaultMode: ModeActive,
		Windows: []Window{
			{ID: "recur", Mode: ModeStandby, TZ: "Asia/Saigon", Weekday: "mon-fri", Start: "09:00", End: "17:00"},
			{ID: "vacation", Mode: ModeStandby, From: &from, Until: &until},
		},
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var got Schedule
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.DefaultMode != original.DefaultMode || len(got.Windows) != 2 {
		t.Fatalf("round trip mismatch: %+v vs %+v", got, original)
	}
	if !got.Windows[1].From.Equal(*original.Windows[1].From) {
		t.Fatalf("from time lost: %v vs %v", got.Windows[1].From, original.Windows[1].From)
	}
}

func TestParseWeekdays(t *testing.T) {
	cases := []struct {
		in      string
		wantLen int
		wantErr bool
	}{
		{"mon", 1, false},
		{"mon,wed,fri", 3, false},
		{"mon-fri", 5, false},
		{"sun-sat", 7, false},
		{"fri-mon", 0, true},
		{"mon-fri,sat", 0, true},
		{"funday", 0, true},
		{"", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			out, err := parseWeekdays(tc.in)
			if tc.wantErr != (err != nil) {
				t.Fatalf("want err=%v, got %v (out=%v)", tc.wantErr, err, out)
			}
			if !tc.wantErr && len(out) != tc.wantLen {
				t.Fatalf("len=%d, want %d", len(out), tc.wantLen)
			}
		})
	}
}
