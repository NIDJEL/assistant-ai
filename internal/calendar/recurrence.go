package calendar

import (
	"errors"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

// Date moves apply to the whole record, including its calendar selectors.
func MoveSeriesDate(e *domain.Event) error {
	if e.Recurrence == nil || e.Date == nil {
		return nil
	}
	r := *e.Recurrence
	switch r.Frequency {
	case "month":
		r.Day = e.Date.Day()
	case "year":
		r.Day = e.Date.Day()
		r.Month = e.Date.Month()
	case "week":
		if len(r.Weekdays) == 1 {
			r.Weekdays = []time.Weekday{e.Date.Weekday()}
		} else {
			found := false
			for _, w := range r.Weekdays {
				if w == e.Date.Weekday() {
					found = true
				}
			}
			if !found {
				return errors.New("Дата переноса должна совпадать с одним из дней повторения. Измените правило повторения отдельно.")
			}
		}
	}
	e.Recurrence = &r
	return nil
}

// Rebase changes the whole series, matching the existing record-level UI.
// A time/date edit becomes its new anchor; weekday/month selectors remain.
func Rebase(e *domain.Event, now time.Time, loc *time.Location) error {
	if e.Recurrence == nil {
		e.OccurrenceAt = nil
		e.LastNotificationAt = nil
		return nil
	}
	r := *e.Recurrence
	if len(r.ExDates) > 0 {
		return errors.New("У серии есть отдельные изменения. Меняйте нужные повторения по одному; для нового общего расписания создайте новую серию.")
	}
	if !r.Elapsed() {
		r.LocalTime = e.StartTime
	}
	if r.Timezone == "" {
		r.Timezone = loc.String()
	}
	zone, err := time.LoadLocation(r.Timezone)
	if err != nil {
		return err
	}
	r.Anchor = now.UTC()
	if e.Date != nil && e.StartTime != nil {
		at := domain.LocalInstant(e.Date.Year(), e.Date.Month(), e.Date.Day(), e.StartTime.Hour, e.StartTime.Minute, e.StartSecond, zone)
		if r.Elapsed() {
			step := time.Duration(r.Interval) * time.Minute
			if r.Frequency == "hour" {
				step *= 60
			}
			r.Anchor = at.Add(-step).UTC()
		} else {
			r.Anchor = at.UTC()
		}
	}
	if err := r.Validate(true); err != nil {
		return err
	}
	e.Recurrence = &r
	next, err := r.Next(now)
	if err != nil {
		return err
	}
	e.SetOccurrence(next)
	e.LastNotificationAt = nil
	return nil
}

func Resume(e *domain.Event, now time.Time) error {
	if e.Recurrence == nil {
		return nil
	}
	next, err := e.Recurrence.Next(now)
	if err != nil {
		return err
	}
	e.SetOccurrence(next)
	e.LastNotificationAt = nil
	return nil
}
