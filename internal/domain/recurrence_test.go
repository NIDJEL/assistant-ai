package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func stamp(t *testing.T, value string) time.Time {
	t.Helper()
	v, e := time.Parse(time.RFC3339, value)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func TestCalendarShiftClampsAndPreservesTimezone(t *testing.T) {
	for _, tc := range []struct {
		zone, from, want string
		months, days     int
	}{
		{"Europe/Moscow", "2026-01-31T09:00:00+03:00", "2026-02-28T09:00:00+03:00", 1, 0},
		{"Europe/Moscow", "2024-02-29T09:00:00+03:00", "2025-02-28T09:00:00+03:00", 12, 0},
		{"Europe/Moscow", "2026-12-31T09:00:00+03:00", "2027-03-02T09:00:00+03:00", 2, 2},
		{"Europe/Berlin", "2026-03-28T09:00:00+01:00", "2026-03-29T09:00:00+02:00", 0, 1},
		{"Europe/Berlin", "2026-10-24T09:00:00+02:00", "2026-10-25T09:00:00+01:00", 0, 1},
		{"Europe/Berlin", "2026-03-28T02:30:00+01:00", "2026-03-29T03:30:00+02:00", 0, 1},
		{"America/New_York", "2026-03-07T09:00:00-05:00", "2026-03-08T09:00:00-04:00", 0, 1},
	} {
		t.Run(tc.zone+tc.from, func(t *testing.T) {
			loc, _ := time.LoadLocation(tc.zone)
			got := ShiftCalendar(stamp(t, tc.from).In(loc), tc.months, tc.days)
			if got.Format(time.RFC3339) != tc.want {
				t.Fatal(got, tc.want)
			}
		})
	}
}

func TestNextOccurrenceDSTAndCalendarBoundaries(t *testing.T) {
	for _, tc := range []struct {
		zone, frequency, anchor, after, want, clock string
		interval, day, month                        int
		weekdays                                    []time.Weekday
	}{
		{"Europe/Berlin", "day", "2026-03-28T09:00:00+01:00", "2026-03-28T09:00:00+01:00", "2026-03-29T09:00:00+02:00", "09:00", 1, 0, 0, nil},
		{"Europe/Berlin", "day", "2026-10-24T09:00:00+02:00", "2026-10-24T09:00:00+02:00", "2026-10-25T09:00:00+01:00", "09:00", 1, 0, 0, nil},
		{"Europe/Berlin", "day", "2026-03-28T02:30:00+01:00", "2026-03-28T02:30:00+01:00", "2026-03-29T03:30:00+02:00", "02:30", 1, 0, 0, nil},
		{"Europe/Berlin", "day", "2026-03-28T02:30:00+01:00", "2026-03-29T03:30:00+02:00", "2026-03-30T02:30:00+02:00", "02:30", 1, 0, 0, nil},
		{"Europe/Berlin", "day", "2026-10-24T02:30:00+02:00", "2026-10-24T02:30:00+02:00", "2026-10-25T02:30:00+02:00", "02:30", 1, 0, 0, nil},
		{"Europe/Berlin", "day", "2026-10-24T02:30:00+02:00", "2026-10-25T02:30:00+02:00", "2026-10-26T02:30:00+01:00", "02:30", 1, 0, 0, nil},
		{"America/New_York", "day", "2026-03-07T09:00:00-05:00", "2026-03-07T09:00:00-05:00", "2026-03-08T09:00:00-04:00", "09:00", 1, 0, 0, nil},
		{"Europe/Moscow", "month", "2026-01-31T09:00:00+03:00", "2026-01-31T09:00:00+03:00", "2026-03-31T09:00:00+03:00", "09:00", 1, 31, 0, nil},
		{"Europe/Moscow", "month", "2026-12-15T09:00:00+03:00", "2026-12-15T09:00:00+03:00", "2027-01-15T09:00:00+03:00", "09:00", 1, 15, 0, nil},
		{"Europe/Moscow", "year", "2024-02-29T09:00:00+03:00", "2024-02-29T09:00:00+03:00", "2028-02-29T09:00:00+03:00", "09:00", 1, 29, 2, nil},
		{"Europe/Moscow", "hour", "2026-10-14T12:30:17+03:00", "2026-10-14T21:31:00+03:00", "2026-10-14T22:30:17+03:00", "", 2, 0, 0, nil},
		{"Europe/Moscow", "minute", "2026-10-14T23:50:17+03:00", "2026-10-14T23:50:17+03:00", "2026-10-15T00:20:17+03:00", "", 30, 0, 0, nil},
		{"Europe/Berlin", "week", "2026-03-23T09:00:00+01:00", "2026-03-28T09:00:00+01:00", "2026-03-29T09:00:00+02:00", "09:00", 1, 0, 0, []time.Weekday{time.Sunday}},
		{"Europe/Moscow", "week", "2026-10-12T09:00:00+03:00", "2026-10-16T09:00:00+03:00", "2026-10-26T09:00:00+03:00", "09:00", 2, 0, 0, []time.Weekday{time.Monday, time.Friday}},
	} {
		t.Run(tc.frequency+tc.after+tc.clock, func(t *testing.T) {
			r := Recurrence{Frequency: tc.frequency, Interval: tc.interval, Day: tc.day, Month: time.Month(tc.month), Weekdays: tc.weekdays, Timezone: tc.zone, Anchor: stamp(t, tc.anchor)}
			if tc.clock != "" {
				r.LocalTime, _ = ParseTime(tc.clock)
			}
			got, err := r.Next(stamp(t, tc.after))
			if err != nil || got.Format(time.RFC3339) != tc.want {
				t.Fatal(got, err, tc.want)
			}
			raw, _ := json.Marshal(r)
			var restored Recurrence
			if err = json.Unmarshal(raw, &restored); err != nil {
				t.Fatal(err)
			}
			again, err := restored.Next(stamp(t, tc.after))
			if err != nil || !again.Equal(got) {
				t.Fatal("persistence changed next occurrence", again, err)
			}
		})
	}
}

func TestRejectMalformedRecurrence(t *testing.T) {
	for _, r := range []Recurrence{
		{Frequency: "day", Interval: 0, Timezone: "UTC"},
		{Frequency: "day", Interval: 1, Timezone: "invalid"},
		{Frequency: "hour", Interval: 1, Timezone: "UTC", LocalTime: &TimeOfDay{9, 0}},
		{Frequency: "week", Interval: 1, Timezone: "UTC", Weekdays: []time.Weekday{1, 1}},
		{Frequency: "year", Interval: 1, Timezone: "UTC", Day: 31, Month: time.February},
		{Frequency: "month", Interval: 1, Timezone: "UTC", Day: 32},
		{Frequency: "day", Interval: 1, Timezone: "UTC", Weekdays: []time.Weekday{1}},
	} {
		if err := r.Validate(true); err == nil {
			t.Fatal("malformed rule accepted", r)
		}
	}
}
