package agent

import (
	"strings"
	"testing"
	"time"
)

func TestBuildTimeSection(t *testing.T) {
	cases := []struct {
		name      string
		tz        string
		wantLabel string
		wantLoc   *time.Location
	}{
		{"empty falls back to UTC", "", "(UTC)", time.UTC},
		{"valid IANA renders in zone", "Asia/Ho_Chi_Minh", "(Asia/Ho_Chi_Minh)", mustLoadLocation(t, "Asia/Ho_Chi_Minh")},
		{"invalid IANA falls back to UTC", "Bogus/Invalid", "(UTC)", time.UTC},
		{"NY zone", "America/New_York", "(America/New_York)", mustLoadLocation(t, "America/New_York")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := buildTimeSection(tc.tz)
			if len(lines) != 3 {
				t.Fatalf("expected 3 lines, got %d", len(lines))
			}
			if lines[2] != "" {
				t.Errorf("expected trailing blank line, got %q", lines[2])
			}
			line := lines[0]
			if !strings.HasPrefix(line, "Current date: ") {
				t.Errorf("missing prefix: %q", line)
			}
			if !strings.Contains(line, tc.wantLabel) {
				t.Errorf("expected label %q in line %q", tc.wantLabel, line)
			}
			now := time.Now().In(tc.wantLoc)
			expectedWeekday := now.Format("Monday")
			if !strings.Contains(line, expectedWeekday) {
				t.Errorf("expected weekday %q in line %q", expectedWeekday, line)
			}
			expectedTomorrow := now.AddDate(0, 0, 1).Format("Monday")
			if !strings.Contains(line, "Tomorrow is "+expectedTomorrow) {
				t.Errorf("expected pre-computed tomorrow %q in line %q", expectedTomorrow, line)
			}
			if !strings.Contains(lines[1], "authoritative") {
				t.Errorf("expected authoritative directive, got %q", lines[1])
			}
		})
	}
}

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("LoadLocation(%q): %v", name, err)
	}
	return loc
}
