package domain

import (
	"errors"
	"sort"
	"time"
)

const MaxSeriesOccurrences = 1000

var ErrSeriesLimit = errors.New("Серия слишком длинная для полной проверки: сократите её до 1000 повторений")

type EventSlot struct{ Start, End time.Time }

func (r Recurrence) Bounded() bool { return r.Count > 0 || r.Until != nil }
func (r Recurrence) Excluded(at time.Time) bool {
	for _, excluded := range r.ExDates {
		if excluded.Equal(at) {
			return true
		}
	}
	return false
}
func (r Recurrence) Contains(at time.Time) bool {
	r.ExDates = nil
	next, err := r.Next(at.Add(-time.Nanosecond))
	return err == nil && next.Equal(at)
}
func (r Recurrence) Excluding(at time.Time) (*Recurrence, error) {
	if !r.Contains(at) {
		return nil, errors.New("Указанная дата не является повторением этой серии")
	}
	copy := r
	copy.ExDates = append([]time.Time{}, r.ExDates...)
	if !copy.Excluded(at) {
		copy.ExDates = append(copy.ExDates, at.UTC())
	}
	sort.Slice(copy.ExDates, func(i, j int) bool { return copy.ExDates[i].Before(copy.ExDates[j]) })
	return &copy, copy.Validate(true)
}

// Occurrences expands the complete requested interval or returns an explicit
// limit error. Count counts scheduled slots, including excluded occurrences.
func (r Recurrence) Occurrences(from, to time.Time, limit int) ([]time.Time, error) {
	if err := r.Validate(true); err != nil {
		return nil, err
	}
	if limit < 1 || limit > MaxSeriesOccurrences || !to.After(from) {
		return nil, errors.New("Некорректный период серии")
	}
	zone, _ := time.LoadLocation(r.Timezone)
	if r.Until != nil {
		end := r.Until.AddDays(1).At(TimeOfDay{}, zone)
		if end.Before(to) {
			to = end
		}
	}
	base := r
	base.Count = 0
	base.Until = nil
	base.ExDates = nil
	cursor := from.Add(-time.Nanosecond)
	if r.Count > 0 {
		cursor = r.Anchor.Add(-time.Nanosecond)
	}
	result := []time.Time{}
	for scanned := 0; scanned < MaxSeriesOccurrences*2+2; scanned++ {
		if r.Count > 0 && scanned >= r.Count {
			return result, nil
		}
		next, err := base.nextUnbounded(cursor)
		if err != nil {
			return nil, err
		}
		if !next.Before(to) {
			return result, nil
		}
		if !next.Before(from) && !r.Excluded(next) {
			if len(result) == limit {
				return nil, ErrSeriesLimit
			}
			result = append(result, next)
		}
		cursor = next
	}
	return nil, ErrSeriesLimit
}
func (e Event) Slots(from, to time.Time, loc *time.Location) ([]EventSlot, error) {
	start, end, ok := e.Interval(loc)
	if !ok {
		return nil, nil
	}
	if e.Recurrence == nil {
		if start.Before(to) && from.Before(end) {
			return []EventSlot{{start, end}}, nil
		}
		return nil, nil
	}
	times, err := e.Recurrence.Occurrences(from.Add(-e.Duration), to, MaxSeriesOccurrences)
	if err != nil {
		return nil, err
	}
	result := make([]EventSlot, 0, len(times))
	for _, at := range times {
		if at.Before(start) {
			continue
		}
		if at.Add(e.Duration).After(from) {
			result = append(result, EventSlot{at, at.Add(e.Duration)})
		}
	}
	return result, nil
}
func (e Event) MeetingSlots(loc *time.Location) ([]EventSlot, error) {
	start, _, ok := e.Interval(loc)
	if !ok {
		return nil, nil
	}
	if e.Recurrence != nil && !e.Recurrence.Bounded() {
		return nil, errors.New("Для серии встреч укажите число повторений или дату окончания")
	}
	return e.Slots(start, time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC), loc)
}
func SlotsOverlap(a, b []EventSlot) bool {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i].Start.Before(b[j].End) && b[j].Start.Before(a[i].End) {
			return true
		}
		if a[i].End.Before(b[j].End) {
			i++
		} else {
			j++
		}
	}
	return false
}
