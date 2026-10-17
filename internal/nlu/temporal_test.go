package nlu

import (
	"testing"
	"time"
)

func TestDayOfMonth(t *testing.T) {
	for _, tc := range []struct {
		name, now, want string
		day             int
	}{
		{"current month", "2026-10-17T12:00:00+03:00", "2026-10-20", 20},
		{"same day", "2026-10-20T23:59:00+03:00", "2026-10-20", 20},
		{"next month", "2026-10-25T12:00:00+03:00", "2026-11-20", 20},
		{"December January", "2026-12-25T12:00:00+03:00", "2027-01-20", 20},
		{"leap February", "2028-02-17T12:00:00+03:00", "2028-02-29", 29},
		{"nonleap February", "2027-02-17T12:00:00+03:00", "2027-03-29", 29},
		{"short month", "2026-04-17T12:00:00+03:00", "2026-05-31", 31},
		{"month end", "2026-01-31T12:00:00+03:00", "2026-03-30", 30},
		{"user timezone", "2026-10-21T00:10:00+14:00", "2026-11-20", 20},
		{"negative timezone", "2026-10-20T23:50:00-10:00", "2026-10-20", 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now, _ := time.Parse(time.RFC3339, tc.now)
			start, end, exact, err := (Temporal{Type: "day_of_month", Day: tc.day}).Resolve(now)
			if err != nil || start.String() != tc.want || end.String() != start.AddDays(1).String() || exact != nil {
				t.Fatalf("start=%v end=%v exact=%v err=%v", start, end, exact, err)
			}
		})
	}
}

func TestDayOfMonthInvalid(t *testing.T) {
	for _, meaning := range []Temporal{
		{Type: "day_of_month"}, {Type: "day_of_month", Day: -1}, {Type: "day_of_month", Day: 32},
		{Type: "day_of_month", Day: 20, Date: "2026-10-20"}, {Type: "day_of_month", Day: 20, Offset: 1},
		{Type: "day_of_month", Day: 20, EndDate: "2026-10-21"}, {Type: "relative_day", Day: 20},
	} {
		if _, _, _, err := meaning.Resolve(time.Now()); err == nil {
			t.Fatalf("accepted %+v", meaning)
		}
	}
}
