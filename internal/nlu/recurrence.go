package nlu

import (
	"errors"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

// Anchor/timezone are assigned by Go, never accepted from model output.
type Recurrence struct {
	Frequency string    `json:"frequency"`
	Interval  int       `json:"interval,omitempty"`
	Weekdays  []string  `json:"weekdays,omitempty"`
	Day       int       `json:"day_of_month,omitempty"`
	Month     int       `json:"month,omitempty"`
	Until     *Temporal `json:"until,omitempty"`
	Count     int       `json:"count,omitempty"`
}

var weekdayNames = []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}

// A reply to the series-boundary question may provide only its new bound.
// Never invent a frequency when there is no persisted rule to extend.
func MergeRecurrence(base, patch *Recurrence) (*Recurrence, error) {
	if patch == nil || patch.Frequency != "" {
		return patch, nil
	}
	if base == nil || base.Frequency == "" || !patch.BoundaryOnly() {
		return nil, errors.New("missing recurrence to continue")
	}
	merged := *base
	if patch.Until != nil {
		merged.Until, merged.Count = patch.Until, 0
	}
	if patch.Count > 0 {
		merged.Count, merged.Until = patch.Count, nil
	}
	return &merged, nil
}

func (r Recurrence) BoundaryOnly() bool {
	return r.Frequency == "" && r.Interval == 0 && len(r.Weekdays) == 0 && r.Day == 0 && r.Month == 0 && (r.Until != nil || r.Count > 0) && !(r.Until != nil && r.Count > 0)
}

func (r Recurrence) Bind(now time.Time, date *domain.Date, clock *domain.TimeOfDay) (*domain.Recurrence, error) {
	interval := r.Interval
	if interval == 0 {
		interval = 1
	}
	rule := &domain.Recurrence{Frequency: r.Frequency, Interval: interval, Day: r.Day, Month: time.Month(r.Month), Count: r.Count, Timezone: now.Location().String(), Anchor: now.UTC()}
	for _, name := range r.Weekdays {
		found := false
		for i, s := range weekdayNames {
			if name == s {
				rule.Weekdays = append(rule.Weekdays, time.Weekday(i))
				found = true
			}
		}
		if !found {
			return nil, errors.New("invalid weekday")
		}
	}
	if r.Until != nil {
		if r.Until.Type == "calendar_period" || r.Until.Type == "rolling_period" || r.Until.Type == "date_range" || r.Until.Type == "relative_delay" {
			return nil, errors.New("specific end date required")
		}
		day, _, _, err := r.Until.Resolve(now)
		if err != nil {
			return nil, err
		}
		rule.Until = day
	}
	if !rule.Elapsed() {
		rule.LocalTime = clock
		if date == nil {
			date = domain.Day(now)
		}
		if clock != nil {
			rule.Anchor = domain.LocalInstant(date.Year(), date.Month(), date.Day(), clock.Hour, clock.Minute, 0, now.Location()).UTC()
		}
	}
	if err := rule.Validate(false); err != nil {
		return nil, err
	}
	return rule, nil
}

func DisplayRecurrence(r *domain.Recurrence) *Recurrence {
	if r == nil {
		return nil
	}
	out := &Recurrence{Frequency: r.Frequency, Interval: r.Interval, Day: r.Day, Month: int(r.Month), Count: r.Count}
	for _, w := range r.Weekdays {
		out.Weekdays = append(out.Weekdays, weekdayNames[w])
	}
	if r.Until != nil {
		out.Until = &Temporal{Type: "absolute", Date: r.Until.String()}
	}
	return out
}
