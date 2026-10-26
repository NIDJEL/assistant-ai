package domain

import (
	"errors"
	"time"
	"unicode/utf8"
)

// ExecutionFilter is grounded exclusively by Go and retained for refinements.
type ExecutionFilter struct {
	Conference string            `json:"conference,omitempty"`
	Query      string            `json:"query,omitempty"`
	Kind       EventType         `json:"kind"`
	Start      *Date             `json:"start,omitempty"`
	End        *Date             `json:"end,omitempty"`
	Status     *Status           `json:"status,omitempty"`
	Priority   *Priority         `json:"priority,omitempty"`
	TimeFrom   *TimeOfDay        `json:"time_from,omitempty"`
	TimeBefore *TimeOfDay        `json:"time_before,omitempty"`
	Exclude    []ExecutionFilter `json:"exclude,omitempty"`
}

type ExecutionChanges struct {
	OccurrenceDate   *Date       `json:"occurrence_date,omitempty"`
	OccurrenceAt     *time.Time  `json:"occurrence_at,omitempty"`
	Scope            string      `json:"scope,omitempty"`
	Recurrence       *Recurrence `json:"recurrence,omitempty"`
	ClearRecurrence  bool        `json:"clear_recurrence,omitempty"`
	DurationMinutes  *int        `json:"duration_minutes,omitempty"`
	Priority         *Priority   `json:"priority,omitempty"`
	Status           *Status     `json:"status,omitempty"`
	TimeShiftMinutes *int        `json:"time_shift_minutes,omitempty"`
}

func (f ExecutionFilter) Validate() error { return f.validateDepth(0) }
func (f ExecutionFilter) validateDepth(depth int) error {
	bad := errors.New("invalid grounded filter")
	if f.Conference != "" && f.Conference != "zoom" {
		return bad
	}
	if depth > 4 || len(f.Exclude) > 20 || utf8.RuneCountInString(f.Query) > 2000 || f.Kind > EventReminder {
		return bad
	}
	if f.Status != nil && *f.Status > StatusDone || f.Priority != nil && (*f.Priority < PriorityNormal || *f.Priority > PriorityHigh) {
		return bad
	}
	if f.Start != nil && f.End != nil && !f.End.After(f.Start.Time) {
		return bad
	}
	for _, c := range []*TimeOfDay{f.TimeFrom, f.TimeBefore} {
		if c != nil && (c.Hour < 0 || c.Hour > 23 || c.Minute < 0 || c.Minute > 59) {
			return bad
		}
	}
	if f.TimeFrom != nil && f.TimeBefore != nil && f.TimeFrom.Hour*60+f.TimeFrom.Minute >= f.TimeBefore.Hour*60+f.TimeBefore.Minute {
		return bad
	}
	for _, e := range f.Exclude {
		if err := e.validateDepth(depth + 1); err != nil {
			return err
		}
	}
	return nil
}
