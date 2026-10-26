package domain

import (
	"errors"
	"testing"
	"time"
)

func TestSeriesExpansionBoundsExclusionsAndDST(t *testing.T) {
	zone, _ := time.LoadLocation("Europe/Berlin")
	first := time.Date(2026, 10, 24, 9, 0, 0, 0, zone)
	r := Recurrence{Frequency: "day", Interval: 1, Count: 4, Anchor: first, Timezone: zone.String(), LocalTime: &TimeOfDay{Hour: 9}}
	except := time.Date(2026, 10, 25, 9, 0, 0, 0, zone)
	copy, err := r.Excluding(except)
	if err != nil {
		t.Fatal(err)
	}
	times, err := copy.Occurrences(first, first.AddDate(0, 0, 5), 1000)
	if err != nil || len(times) != 3 {
		t.Fatal(times, err)
	}
	if len(r.ExDates) != 0 || times[1].In(zone).Hour() != 9 || times[1].Sub(times[0]) != 49*time.Hour {
		t.Fatal("DST/exclusion drift", times)
	}
	if _, err = copy.Next(times[2]); !errors.Is(err, ErrRecurrenceExhausted) {
		t.Fatal("count ignored", err)
	}
	if _, err = r.Occurrences(first, first.AddDate(0, 0, 5), 2); !errors.Is(err, ErrSeriesLimit) {
		t.Fatal("silently truncated", err)
	}
	all := r
	for i := 0; i < 4; i++ {
		all.ExDates = append(all.ExDates, first.AddDate(0, 0, i))
	}
	if _, err = all.Next(first.Add(-time.Second)); !errors.Is(err, ErrRecurrenceExhausted) {
		t.Fatal(err)
	}
}

func TestMeetingSeriesRequiresCompleteFiniteValidation(t *testing.T) {
	first := time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)
	e := Event{Kind: EventMeeting, Title: "Планирование", Date: Day(first), StartTime: &TimeOfDay{Hour: 9}, Duration: 2 * time.Hour, Status: StatusOpen, Priority: PriorityNormal, EventDetails: EventDetails{Recurrence: &Recurrence{Frequency: "hour", Interval: 1, Timezone: "UTC", Anchor: first.Add(-time.Hour)}}}
	if e.Validate() == nil {
		t.Fatal("unbounded meeting accepted")
	}
	e.Recurrence.Count = 3
	if e.Validate() == nil {
		t.Fatal("self-overlapping series accepted")
	}
	e.Duration = 30 * time.Minute
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestElapsedRangeCountsDSTExclusionsAndFiniteBounds(t *testing.T) {
	zone, _ := time.LoadLocation("Europe/Berlin")
	start := time.Date(2026, 10, 25, 0, 0, 0, 0, zone)
	r := Recurrence{Frequency: "minute", Interval: 1, Timezone: zone.String(), Anchor: start.Add(-time.Minute), ExDates: []time.Time{start.Add(time.Hour)}}
	first, count, err := r.ElapsedRange(start, start.AddDate(0, 0, 1))
	if err != nil || count != 1499 || !first.Equal(start) {
		t.Fatal(first, count, err)
	}
	r.Count = 2
	r.ExDates = []time.Time{start}
	first, count, err = r.ElapsedRange(start, start.AddDate(0, 0, 1))
	if err != nil || count != 1 || !first.Equal(start.Add(time.Minute)) {
		t.Fatal(first, count, err)
	}
}
