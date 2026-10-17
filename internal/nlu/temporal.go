package nlu

import (
	"errors"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

type Temporal struct {
	Type      string `json:"type"`
	Day       int    `json:"day,omitempty"`
	Offset    int    `json:"offset,omitempty"`
	Value     int    `json:"value,omitempty"`
	Unit      string `json:"unit,omitempty"`
	Date      string `json:"date,omitempty"`
	EndDate   string `json:"end_date,omitempty"`
	Weekday   string `json:"weekday,omitempty"`
	Direction string `json:"direction,omitempty"`
}

var errTemporal = errors.New("invalid temporal meaning")

// Resolve performs calendar arithmetic only, in the supplied user's timezone.
// End is exclusive. Exact is set only for delays.
func (t Temporal) Resolve(now time.Time) (start, end *domain.Date, exact *time.Time, err error) {
	if t.Day != 0 && t.Type != "day_of_month" {
		return nil, nil, nil, errTemporal
	}
	if t.EndDate != "" && t.Type != "date_range" {
		return nil, nil, nil, errTemporal
	}
	if t.Offset < -3660 || t.Offset > 3660 || t.Value < 0 || t.Value > 525600 {
		return nil, nil, nil, errTemporal
	}
	day := domain.Day(now)
	switch t.Type {
	case "day_of_month":
		if t.Day < 1 || t.Day > 31 || t.Offset != 0 || t.Value != 0 || t.Unit != "" || t.Date != "" || t.Weekday != "" || t.Direction != "" {
			return nil, nil, nil, errTemporal
		}
		// Start from the first to avoid AddDate normalizing Jan 31 into March.
		// Skip months which do not contain this day; never silently clamp it.
		month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		for i := 0; i < 14; i++ {
			candidate := time.Date(month.Year(), month.Month(), t.Day, 0, 0, 0, 0, now.Location())
			if candidate.Month() == month.Month() && !domain.Day(candidate).Before(day.Time) {
				start = domain.Day(candidate)
				break
			}
			month = month.AddDate(0, 1, 0)
		}
	case "relative_day":
		if t.Value != 0 || t.Unit != "" || t.Date != "" || t.Weekday != "" || t.Direction != "" {
			err = errTemporal
			break
		}
		start = day.AddDays(t.Offset)
	case "absolute":
		if t.Offset != 0 || t.Value != 0 || t.Unit != "" || t.Weekday != "" || t.Direction != "" {
			err = errTemporal
			break
		}
		start, err = domain.ParseDate(t.Date)
	case "date_range":
		if t.Offset != 0 || t.Value != 0 || t.Unit != "" || t.Weekday != "" || t.Direction != "" {
			err = errTemporal
			break
		}
		start, err = domain.ParseDate(t.Date)
		if err == nil {
			end, err = domain.ParseDate(t.EndDate)
		}
	case "weekday":
		if t.Offset != 0 || t.Value != 0 || t.Unit != "" || t.Date != "" {
			err = errTemporal
			break
		}
		weekdays := map[string]int{"monday": 1, "tuesday": 2, "wednesday": 3, "thursday": 4, "friday": 5, "saturday": 6, "sunday": 7}
		wd, ok := weekdays[t.Weekday]
		if !ok {
			err = errTemporal
			break
		}
		current := (int(now.Weekday())+6)%7 + 1
		delta := (wd - current + 7) % 7
		switch t.Direction {
		case "next", "next_or_current":
			if t.Direction == "next" && delta == 0 {
				delta = 7
			}
		case "next_week":
			delta = 7 - current + wd
		case "previous":
			delta = -((current - wd + 7) % 7)
			if delta == 0 {
				delta = -7
			}
		default:
			err = errTemporal
		}
		start = day.AddDays(delta)
	case "relative_delay":
		if t.Offset != 0 || t.Value <= 0 || t.Date != "" || t.Weekday != "" || t.Direction != "" {
			err = errTemporal
			break
		}
		units := map[string]time.Duration{"second": time.Second, "minute": time.Minute, "hour": time.Hour}
		at := now
		if unit, ok := units[t.Unit]; ok {
			at = now.Add(time.Duration(t.Value) * unit)
		} else if t.Unit == "day" && t.Value <= 3660 {
			at = now.AddDate(0, 0, t.Value)
		} else {
			err = errTemporal
			break
		}
		start, exact = domain.Day(at), &at
	case "calendar_period", "rolling_period":
		if t.Date != "" || t.Weekday != "" || t.Direction != "" {
			err = errTemporal
			break
		}
		if t.Type == "rolling_period" {
			if t.Offset != 0 || t.Value < 1 || t.Value > 3660 || t.Unit != "day" {
				err = errTemporal
				break
			}
			start, end = day.AddDays(1-t.Value), day.AddDays(1)
			break
		}
		if t.Value != 0 {
			err = errTemporal
			break
		}
		switch t.Unit {
		case "day":
			start = day.AddDays(t.Offset)
			end = start.AddDays(1)
		case "week":
			start = day.AddDays(-((int(now.Weekday()) + 6) % 7) + 7*t.Offset)
			end = start.AddDays(7)
		case "month":
			start = domain.Day(time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, t.Offset, 0))
			end = domain.Day(start.AddDate(0, 1, 0))
		case "year":
			start = domain.Day(time.Date(now.Year()+t.Offset, 1, 1, 0, 0, 0, 0, now.Location()))
			end = domain.Day(start.AddDate(1, 0, 0))
		default:
			err = errTemporal
		}
	default:
		err = errTemporal
	}
	if err != nil || start == nil || start.Year() < 1 || start.Year() > 9999 || end != nil && (end.Year() > 9999 || !end.After(start.Time)) {
		return nil, nil, nil, errTemporal
	}
	if end == nil {
		end = start.AddDays(1)
	}
	return start, end, exact, nil
}
