package nlp

import (
	"errors"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

// TemporalMeaning contains language-independent calendar semantics, never NLP.
// Null fields are intentional: the same strict shape is used by every provider.
type TemporalMeaning struct {
	Type      string  `json:"type"`
	Unit      *string `json:"unit"`
	Offset    *int    `json:"offset"`
	Value     *int    `json:"value"`
	Weekday   *string `json:"weekday"`
	Direction *string `json:"direction"`
	Year      *int    `json:"year"`
	Month     *int    `json:"month"`
	Day       *int    `json:"day"`
}

type ClockMeaning struct {
	Hour   int `json:"hour"`
	Minute int `json:"minute"`
}

func (c ClockMeaning) Validate() error {
	if c.Hour < 0 || c.Hour > 23 || c.Minute < 0 || c.Minute > 59 {
		return errors.New("invalid semantic clock")
	}
	return nil
}

func oneOf(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}

func (t TemporalMeaning) Validate() error {
	bad := errors.New("invalid normalized temporal meaning")
	unit := semanticString(t.Unit)
	if t.Offset != nil && (*t.Offset < -1200 || *t.Offset > 1200) || t.Value != nil && (*t.Value < 1 || *t.Value > 31536000) {
		return bad
	}
	allowed := map[string]bool{}
	switch t.Type {
	case "calendar_period":
		if !oneOf(unit, "day", "week", "month", "quarter", "year") || t.Offset == nil {
			return bad
		}
		allowed["unit"], allowed["offset"] = true, true
	case "relative_day":
		if t.Offset == nil {
			return bad
		}
		allowed["offset"] = true
	case "relative_delay", "rolling_period":
		if t.Value == nil || !oneOf(unit, "second", "minute", "hour", "day", "week", "month", "year") || t.Type == "rolling_period" && !oneOf(unit, "day", "week", "month", "year") {
			return bad
		}
		allowed["unit"], allowed["value"] = true, true
		limits := map[string]int{"second": 31536000, "minute": 525600, "hour": 8760, "day": 3660, "week": 520, "month": 120, "year": 10}
		if *t.Value > limits[unit] {
			return bad
		}
	case "weekday":
		// An unqualified weekday has canonical next_or_current semantics.
		if !oneOf(semanticString(t.Weekday), "monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday") || t.Direction != nil && !oneOf(*t.Direction, "next_or_current", "next_week", "previous_week") {
			return bad
		}
		allowed["weekday"], allowed["direction"] = true, true
	case "calendar_date":
		if t.Year == nil || t.Month == nil || t.Day == nil || *t.Year < 1 || *t.Year > 9998 {
			return bad
		}
		d := time.Date(*t.Year, time.Month(*t.Month), *t.Day, 0, 0, 0, 0, time.UTC)
		if int(d.Month()) != *t.Month || d.Day() != *t.Day {
			return bad
		}
		allowed["year"], allowed["month"], allowed["day"] = true, true, true
	default:
		return bad
	}
	for k, present := range map[string]bool{"unit": t.Unit != nil, "offset": t.Offset != nil, "value": t.Value != nil, "weekday": t.Weekday != nil, "direction": t.Direction != nil, "year": t.Year != nil, "month": t.Month != nil, "day": t.Day != nil} {
		if present && !allowed[k] {
			return bad
		}
	}
	return nil
}

type ResolvedTemporal struct {
	Start, End *domain.Date
	At         *time.Time
}

// ResolveTemporal never reads the user's message or invokes a Russian parser.
// Periods are half-open, weeks start on Monday, civil arithmetic uses now.Location.
func ResolveTemporal(t TemporalMeaning, now time.Time) (ResolvedTemporal, error) {
	var r ResolvedTemporal
	if now.Year() < 1 || now.Year() > 9998 {
		return r, errors.New("temporal anchor out of range")
	}
	if err := t.Validate(); err != nil {
		return r, err
	}
	d := domain.Day(now)
	unit := semanticString(t.Unit)
	switch t.Type {
	case "relative_day":
		d = d.AddDays(*t.Offset)
	case "calendar_date":
		d = domain.Day(time.Date(*t.Year, time.Month(*t.Month), *t.Day, 0, 0, 0, 0, now.Location()))
	case "weekday":
		weekdays := map[string]int{"monday": 0, "tuesday": 1, "wednesday": 2, "thursday": 3, "friday": 4, "saturday": 5, "sunday": 6}
		current := (int(now.Weekday()) + 6) % 7
		delta := weekdays[*t.Weekday] - current
		direction := "next_or_current"
		if t.Direction != nil {
			direction = *t.Direction
		}
		switch direction {
		case "next_or_current":
			if delta < 0 {
				delta += 7
			}
		case "next_week":
			delta += 7
		case "previous_week":
			delta -= 7
		}
		d = d.AddDays(delta)
	case "relative_delay":
		n := *t.Value
		var at time.Time
		switch unit {
		case "second":
			at = now.Add(time.Duration(n) * time.Second)
		case "minute":
			at = now.Add(time.Duration(n) * time.Minute)
		case "hour":
			at = now.Add(time.Duration(n) * time.Hour)
		case "day":
			at = domain.ShiftCalendar(now, 0, n)
		case "week":
			at = domain.ShiftCalendar(now, 0, n*7)
		case "month":
			at = domain.ShiftCalendar(now, n, 0)
		case "year":
			at = domain.ShiftCalendar(now, n*12, 0)
		}
		if at.Year() < 1 || at.Year() > 9998 || !at.After(now) {
			return r, errors.New("temporal result out of range")
		}
		r.At = &at
		r.Start = domain.Day(at)
		return r, nil
	case "rolling_period":
		n := *t.Value
		end := domain.Day(now).AddDays(1)
		switch unit {
		case "day":
			d = end.AddDays(-n)
		case "week":
			d = end.AddDays(-7 * n)
		case "month":
			d = domain.Day(domain.ShiftCalendar(end.Time, -n, 0))
		case "year":
			d = domain.Day(domain.ShiftCalendar(end.Time, -12*n, 0))
		}
		r.Start, r.End = d, end
		return checkedTemporal(r)
	case "calendar_period":
		o := *t.Offset
		switch unit {
		case "day":
			d = d.AddDays(o)
			r.End = d.AddDays(1)
		case "week":
			d = d.AddDays(-(int(now.Weekday())+6)%7 + 7*o)
			r.End = d.AddDays(7)
		case "month", "quarter", "year":
			month, step := now.Month(), 1
			if unit == "quarter" {
				month = time.Month((int(month)-1)/3*3 + 1)
				step = 3
			}
			if unit == "year" {
				month = time.January
				step = 12
			}
			base := time.Date(now.Year(), month+time.Month(step*o), 1, 0, 0, 0, 0, now.Location())
			if base.Year() < 1 || base.Year() > 9998 {
				return r, errors.New("temporal result out of range")
			}
			d = domain.Day(base)
			r.End = domain.Day(d.AddDate(0, step, 0))
		}
	}
	r.Start = d
	if r.End == nil {
		r.End = d.AddDays(1)
	}
	return checkedTemporal(r)
}

func checkedTemporal(r ResolvedTemporal) (ResolvedTemporal, error) {
	if r.Start == nil || r.Start.Year() < 1 || r.Start.Year() > 9998 || r.End != nil && (r.End.Year() > 9999 || !r.End.After(r.Start.Time)) {
		return ResolvedTemporal{}, errors.New("temporal result out of range")
	}
	return r, nil
}
