package schedule

import (
	"sort"
	"time"
)

// Resolver evaluates the effective Mode at a given instant.
// Precedence: one-shot windows beat recurring; within each tier, first match wins.
// Falls back to Schedule.DefaultMode (default Mode = ModeActive).
type Resolver struct {
	defaultMode Mode
	oneShots    []parsedOneShot
	recurrings  []parsedRecurring
}

type parsedOneShot struct {
	mode  Mode
	from  time.Time
	until time.Time
}

type parsedRecurring struct {
	mode    Mode
	loc     *time.Location
	days    map[time.Weekday]bool
	startMM int
	endMM   int
}

func NewResolver(s *Schedule) (*Resolver, error) {
	if err := Validate(s); err != nil {
		return nil, err
	}
	r := &Resolver{defaultMode: ModeActive}
	if s == nil {
		return r, nil
	}
	if s.DefaultMode != "" {
		r.defaultMode = s.DefaultMode
	}
	for _, w := range s.Windows {
		m := w.Mode
		if m == "" {
			m = ModeStandby
		}
		if w.IsOneShot() {
			r.oneShots = append(r.oneShots, parsedOneShot{mode: m, from: *w.From, until: *w.Until})
			continue
		}
		tz := w.TZ
		if tz == "" {
			tz = DefaultTZ
		}
		loc, _ := time.LoadLocation(tz)
		days, _ := parseWeekdays(w.Weekday)
		startT, _ := parseHM(w.Start)
		endT, _ := parseHM(w.End)
		r.recurrings = append(r.recurrings, parsedRecurring{
			mode:    m,
			loc:     loc,
			days:    days,
			startMM: startT.Hour()*60 + startT.Minute(),
			endMM:   endT.Hour()*60 + endT.Minute(),
		})
	}
	sort.SliceStable(r.oneShots, func(i, j int) bool {
		return r.oneShots[i].from.Before(r.oneShots[j].from)
	})
	return r, nil
}

func (r *Resolver) ModeAt(now time.Time) Mode {
	for _, o := range r.oneShots {
		if !now.Before(o.from) && now.Before(o.until) {
			return o.mode
		}
	}
	for _, w := range r.recurrings {
		if w.matches(now) {
			return w.mode
		}
	}
	return r.defaultMode
}

func (w parsedRecurring) matches(now time.Time) bool {
	local := now.In(w.loc)
	mm := local.Hour()*60 + local.Minute()
	if w.startMM == w.endMM {
		return false
	}
	if w.startMM < w.endMM {
		if !w.days[local.Weekday()] {
			return false
		}
		return mm >= w.startMM && mm < w.endMM
	}
	// Cross-midnight: window opens on Weekday X at startMM, closes next day at endMM.
	if w.days[local.Weekday()] && mm >= w.startMM {
		return true
	}
	prevDay := previousWeekday(local.Weekday())
	if w.days[prevDay] && mm < w.endMM {
		return true
	}
	return false
}

func previousWeekday(d time.Weekday) time.Weekday {
	if d == time.Sunday {
		return time.Saturday
	}
	return d - 1
}
