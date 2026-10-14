package domain

import (
	"errors"
	"time"
)

// Recurrence is a rule in an IANA timezone. Anchor fixes interval alignment;
// advancing an occurrence never changes it. Minute/hour rules measure elapsed
// time; day/week/month/year rules preserve the civil clock.
type Recurrence struct {
	Frequency string         `json:"frequency"`
	Interval  int            `json:"interval"`
	Weekdays  []time.Weekday `json:"weekdays,omitempty"`
	Day       int            `json:"day_of_month,omitempty"`
	Month     time.Month     `json:"month,omitempty"`
	LocalTime *TimeOfDay     `json:"local_time,omitempty"`
	Timezone  string         `json:"timezone"`
	Anchor    time.Time      `json:"anchor"`
}

func (r Recurrence) Elapsed() bool { return r.Frequency == "minute" || r.Frequency == "hour" }
func (r Recurrence) Validate(complete bool) error {
	bad := errors.New("Некорректное правило повторения")
	if r.Interval < 1 || r.Interval > 366 {
		return bad
	}
	if r.Timezone == "" || r.Timezone == "Local" {
		return errors.New("Укажите часовой пояс IANA для повторения")
	}
	if _, err := time.LoadLocation(r.Timezone); err != nil {
		return errors.New("Укажите часовой пояс IANA для повторения")
	}
	if !r.Anchor.IsZero() && (r.Anchor.Year() < 1 || r.Anchor.Year() > 9998) {
		return bad
	}
	switch r.Frequency {
	case "minute", "hour", "day", "week", "month", "year":
	default:
		return bad
	}
	if r.Elapsed() && r.LocalTime != nil || r.Frequency != "week" && len(r.Weekdays) != 0 || r.Frequency != "month" && r.Frequency != "year" && r.Day != 0 || r.Frequency != "year" && r.Month != 0 {
		return bad
	}
	seen := map[time.Weekday]bool{}
	for _, w := range r.Weekdays {
		if w < 0 || w > 6 || seen[w] {
			return bad
		}
		seen[w] = true
	}
	if complete && r.Frequency == "week" && len(r.Weekdays) == 0 || (r.Frequency == "month" || r.Frequency == "year") && (r.Day < 1 || r.Day > 31) {
		return bad
	}
	if r.Frequency == "year" && (r.Month < 1 || r.Month > 12 || r.Day > time.Date(2000, r.Month+1, 0, 0, 0, 0, 0, time.UTC).Day()) {
		return bad
	}
	if r.LocalTime != nil && (r.LocalTime.Hour < 0 || r.LocalTime.Hour > 23 || r.LocalTime.Minute < 0 || r.LocalTime.Minute > 59) {
		return bad
	}
	if complete && (r.Anchor.IsZero() || !r.Elapsed() && r.LocalTime == nil) {
		return errors.New("Для повторения нужно время")
	}
	return nil
}

// LocalInstant resolves folds to their first occurrence, and gaps by shifting
// forward by the DST gap. Only one occurrence belongs to a repeated civil hour.
func LocalInstant(year int, month time.Month, day, hour, minute, second int, loc *time.Location) time.Time {
	civil := time.Date(year, month, day, hour, minute, second, 0, time.UTC)
	guess := time.Date(year, month, day, hour, minute, second, 0, loc)
	var exact, forward time.Time
	for _, delta := range []time.Duration{-48 * time.Hour, -24 * time.Hour, 0, 24 * time.Hour, 48 * time.Hour} {
		_, offset := guess.Add(delta).Zone()
		candidate := civil.Add(-time.Duration(offset) * time.Second).In(loc)
		wall := time.Date(candidate.Year(), candidate.Month(), candidate.Day(), candidate.Hour(), candidate.Minute(), candidate.Second(), 0, time.UTC)
		if wall.Equal(civil) && (exact.IsZero() || candidate.Before(exact)) {
			exact = candidate
		}
		if wall.After(civil) && (forward.IsZero() || candidate.Before(forward)) {
			forward = candidate
		}
	}
	if !exact.IsZero() {
		return exact
	}
	if !forward.IsZero() {
		return forward
	}
	return guess
}

// ShiftCalendar clamps the source day to the target month's last day before
// adding calendar days. It does not interpret a month as a duration.
func ShiftCalendar(t time.Time, months, days int) time.Time {
	base := time.Date(t.Year(), t.Month()+time.Month(months), 1, 12, 0, 0, 0, t.Location())
	last := time.Date(base.Year(), base.Month()+1, 0, 12, 0, 0, 0, t.Location()).Day()
	d := t.Day()
	if d > last {
		d = last
	}
	base = time.Date(base.Year(), base.Month(), d+days, 12, 0, 0, 0, t.Location())
	return LocalInstant(base.Year(), base.Month(), base.Day(), t.Hour(), t.Minute(), t.Second(), t.Location())
}

// Next returns the first occurrence strictly after after. Missing month dates
// (31st, February 29th) are skipped, retaining the original rule without drift.
func (r Recurrence) Next(after time.Time) (time.Time, error) {
	if err := r.Validate(true); err != nil {
		return time.Time{}, err
	}
	loc, _ := time.LoadLocation(r.Timezone)
	a, b := r.Anchor.In(loc), after.In(loc)
	if r.Elapsed() {
		step := time.Duration(r.Interval) * time.Minute
		if r.Frequency == "hour" {
			step *= 60
		}
		n := int64(1)
		if !after.Before(a) {
			n = int64(after.Sub(a)/step) + 1
		}
		return a.Add(time.Duration(n) * step), nil
	}
	clock := *r.LocalTime
	candidate := func(y int, m time.Month, d int) (time.Time, bool) {
		if y < 1 || y > 9999 {
			return time.Time{}, false
		}
		if d > time.Date(y, m+1, 0, 12, 0, 0, 0, loc).Day() {
			return time.Time{}, false
		}
		t := LocalInstant(y, m, d, clock.Hour, clock.Minute, 0, loc)
		return t, t.After(after) && !t.Before(a)
	}
	if r.Frequency == "month" || r.Frequency == "year" {
		step := r.Interval
		if r.Frequency == "year" {
			step *= 12
		}
		start := a.Year()*12 + int(a.Month()) - 1
		n := (b.Year()*12 + int(b.Month()) - 1 - start) / step
		if n < 0 {
			n = 0
		}
		for i := 0; i < 4800; i++ {
			index := start + (n+i)*step
			y, m := index/12, time.Month(index%12+1)
			if r.Frequency == "year" {
				m = r.Month
			}
			if y > 9999 {
				break
			}
			if t, ok := candidate(y, m, r.Day); ok {
				return t, nil
			}
		}
	} else {
		start := Day(a)
		if r.Frequency == "week" {
			start = start.AddDays(-(int(a.Weekday()) + 6) % 7)
		}
		day := Day(b)
		if day.Before(start.Time) {
			day = start
		}
		for i := 0; i <= r.Interval*7+8; i++ {
			d := day.AddDays(i)
			gap := int(d.Sub(start.Time) / (24 * time.Hour))
			match := gap%r.Interval == 0
			if r.Frequency == "week" {
				match = (gap/7)%r.Interval == 0
				weekday := d.Weekday()
				found := false
				for _, w := range r.Weekdays {
					if w == weekday {
						found = true
					}
				}
				match = match && found
			}
			if match {
				if t, ok := candidate(d.Year(), d.Month(), d.Day()); ok {
					return t, nil
				}
			}
		}
	}
	return time.Time{}, errors.New("Не удалось вычислить следующее повторение")
}

func (e *Event) SetOccurrence(at time.Time) {
	loc, _ := time.LoadLocation(e.Recurrence.Timezone)
	local := at.In(loc)
	e.Date, e.StartTime, e.StartSecond = Day(local), &TimeOfDay{local.Hour(), local.Minute()}, local.Second()
	utc := at.UTC()
	e.OccurrenceAt = &utc
}
