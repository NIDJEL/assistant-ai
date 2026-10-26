package domain

import (
	"errors"
	"time"
)

// ElapsedRange counts all elapsed occurrences without expanding a minute rule
// into thousands of calendar cards. The first returned timestamp is editable.
func (r Recurrence) ElapsedRange(from, to time.Time) (time.Time, int, error) {
	if err := r.Validate(true); err != nil {
		return time.Time{}, 0, err
	}
	if !r.Elapsed() || !to.After(from) {
		return time.Time{}, 0, errors.New("Некорректный период часовой или минутной серии")
	}
	if r.Until != nil {
		zone, _ := time.LoadLocation(r.Timezone)
		end := r.Until.AddDays(1).At(TimeOfDay{}, zone)
		if end.Before(to) {
			to = end
		}
	}
	step := time.Duration(r.Interval) * time.Minute
	if r.Frequency == "hour" {
		step *= 60
	}
	n := int64(1)
	if from.After(r.Anchor) {
		delta := from.Sub(r.Anchor)
		n = int64(delta / step)
		if delta%step != 0 {
			n++
		}
		if n < 1 {
			n = 1
		}
	}
	if !to.After(r.Anchor) {
		return time.Time{}, 0, nil
	}
	delta := to.Sub(r.Anchor)
	last := int64(delta / step)
	if delta%step == 0 {
		last--
	}
	if r.Count > 0 && last > int64(r.Count) {
		last = int64(r.Count)
	}
	if last < n {
		return time.Time{}, 0, nil
	}
	count := last - n + 1
	excluded := map[int64]bool{}
	for _, at := range r.ExDates {
		d := at.Sub(r.Anchor)
		index := int64(d / step)
		if d%step == 0 && index >= n && index <= last {
			excluded[index] = true
		}
	}
	count -= int64(len(excluded))
	if count == 0 {
		return time.Time{}, 0, nil
	}
	for excluded[n] {
		n++
	}
	return r.Anchor.Add(time.Duration(n) * step), int(count), nil
}
