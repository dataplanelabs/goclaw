// Package schedule provides a pure-Go time-aware mode resolver for channel
// standby windows. Used by the pipeline StandbyGate to decide whether the
// agent should reply or stay silent at a given instant.
package schedule

import (
	"encoding/json"
	"time"
)

type Mode string

const (
	ModeActive  Mode = "active"
	ModeStandby Mode = "standby"
)

const DefaultTZ = "Asia/Saigon"

// Schedule is the JSONB blob stored on channel_instances.silence_schedule
// or in channel_thread_schedules.schedule.
type Schedule struct {
	DefaultMode Mode     `json:"default_mode,omitempty"`
	Windows     []Window `json:"windows,omitempty"`
}

// Window is either recurring (Weekday + Start + End) or one-shot (From + Until).
// Validator rejects mixed or empty windows.
type Window struct {
	ID      string     `json:"id,omitempty"`
	Mode    Mode       `json:"mode,omitempty"`
	TZ      string     `json:"tz,omitempty"`
	Weekday string     `json:"weekday,omitempty"`
	Start   string     `json:"start,omitempty"`
	End     string     `json:"end,omitempty"`
	From    *time.Time `json:"from,omitempty"`
	Until   *time.Time `json:"until,omitempty"`
}

func (w Window) IsOneShot() bool   { return w.From != nil && w.Until != nil }
func (w Window) IsRecurring() bool { return w.Weekday != "" || w.Start != "" || w.End != "" }

func (s *Schedule) MarshalJSON() ([]byte, error) {
	type alias Schedule
	return json.Marshal((*alias)(s))
}

func (s *Schedule) UnmarshalJSON(b []byte) error {
	type alias Schedule
	return json.Unmarshal(b, (*alias)(s))
}
