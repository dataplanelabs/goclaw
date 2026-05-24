package schedule

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidMode    = errors.New("invalid mode (must be 'active' or 'standby')")
	ErrInvalidTZ      = errors.New("invalid timezone")
	ErrInvalidTime    = errors.New("invalid time format (expected 15:04)")
	ErrInvalidWeekday = errors.New("invalid weekday (expected mon/tue/.../sun, comma list, or range like mon-fri)")
	ErrMixedWindow    = errors.New("window must be recurring (weekday+start+end) OR one-shot (from+until), not both")
	ErrEmptyWindow    = errors.New("window must have either recurring or one-shot fields")
	ErrInvertedRange  = errors.New("one-shot window: from must be before until")
)

func Validate(s *Schedule) error {
	if s == nil {
		return nil
	}
	if s.DefaultMode != "" && !validMode(s.DefaultMode) {
		return fmt.Errorf("default_mode: %w", ErrInvalidMode)
	}
	for i, w := range s.Windows {
		if err := validateWindow(w); err != nil {
			return fmt.Errorf("windows[%d]: %w", i, err)
		}
	}
	return nil
}

func validateWindow(w Window) error {
	if w.Mode != "" && !validMode(w.Mode) {
		return ErrInvalidMode
	}
	if w.IsOneShot() && w.IsRecurring() {
		return ErrMixedWindow
	}
	if !w.IsOneShot() && !w.IsRecurring() {
		return ErrEmptyWindow
	}
	tz := w.TZ
	if tz == "" {
		tz = DefaultTZ
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidTZ, tz)
	}
	if w.IsOneShot() {
		if !w.From.Before(*w.Until) {
			return ErrInvertedRange
		}
		return nil
	}
	if _, err := parseHM(w.Start); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	if _, err := parseHM(w.End); err != nil {
		return fmt.Errorf("end: %w", err)
	}
	if _, err := parseWeekdays(w.Weekday); err != nil {
		return fmt.Errorf("weekday: %w", err)
	}
	return nil
}

func validMode(m Mode) bool { return m == ModeActive || m == ModeStandby }

func parseHM(s string) (time.Time, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q", ErrInvalidTime, s)
	}
	return t, nil
}

var weekdayMap = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday,
	"wed": time.Wednesday, "thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

var weekdayOrder = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

// parseWeekdays accepts: "mon" | "mon,wed,fri" | "mon-fri" (no mixing of list and range).
func parseWeekdays(s string) (map[time.Weekday]bool, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return nil, ErrInvalidWeekday
	}
	out := make(map[time.Weekday]bool, 7)
	if strings.Contains(s, "-") {
		if strings.Contains(s, ",") {
			return nil, ErrInvalidWeekday
		}
		parts := strings.Split(s, "-")
		if len(parts) != 2 {
			return nil, ErrInvalidWeekday
		}
		start, ok1 := weekdayIndex(parts[0])
		end, ok2 := weekdayIndex(parts[1])
		if !ok1 || !ok2 || start > end {
			return nil, ErrInvalidWeekday
		}
		for i := start; i <= end; i++ {
			out[weekdayMap[weekdayOrder[i]]] = true
		}
		return out, nil
	}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		wd, ok := weekdayMap[p]
		if !ok {
			return nil, ErrInvalidWeekday
		}
		out[wd] = true
	}
	return out, nil
}

func weekdayIndex(name string) (int, bool) {
	for i, n := range weekdayOrder {
		if n == name {
			return i, true
		}
	}
	return 0, false
}
