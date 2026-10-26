package domain

import (
	"errors"
	"testing"
	"time"
)

func TestRecurrenceCountDoesNotRestartWhenAdvanced(t *testing.T) {
	anchor := stamp(t, "2026-10-26T09:00:00+03:00")
	clock := &TimeOfDay{Hour: 9}
	for _, frequency := range []string{"minute", "hour", "day", "week", "month", "year"} {
		t.Run(frequency, func(t *testing.T) {
			r := Recurrence{Frequency: frequency, Interval: 1, Count: 3, Timezone: "Europe/Moscow", Anchor: anchor, LocalTime: clock}
			switch frequency {
			case "minute", "hour":
				r.LocalTime = nil
			case "week":
				r.Weekdays = []time.Weekday{time.Monday}
			case "month":
				r.Day = 26
			case "year":
				r.Day = 26
				r.Month = time.October
			}
			cursor := anchor.Add(-time.Second)
			for range 3 {
				next, err := r.Next(cursor)
				if err != nil || !next.After(cursor) {
					t.Fatal(next, err)
				}
				cursor = next
			}
			if _, err := r.Next(cursor); !errors.Is(err, ErrRecurrenceExhausted) {
				t.Fatal("count restarted", err)
			}
		})
	}
}

func TestRecurrenceUntilIsInclusiveInItsTimezone(t *testing.T) {
	until, _ := ParseDate("2026-10-27")
	r := Recurrence{Frequency: "day", Interval: 1, Until: until, Timezone: "Europe/Moscow", Anchor: stamp(t, "2026-10-26T09:00:00+03:00"), LocalTime: &TimeOfDay{Hour: 9}}
	next, err := r.Next(stamp(t, "2026-10-26T09:00:00+03:00"))
	if err != nil || next.In(time.FixedZone("Moscow", 10800)).Day() != 27 {
		t.Fatal(next, err)
	}
	if _, err = r.Next(next); !errors.Is(err, ErrRecurrenceExhausted) {
		t.Fatal(err)
	}
}
