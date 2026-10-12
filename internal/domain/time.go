package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// Date is a civil calendar day, not a midnight appointment.
type Date struct{ time.Time }

func ParseDate(s string) (*Date, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, err
	}
	if t.Year() < 1 || t.Year() > 9999 {
		return nil, fmt.Errorf("invalid calendar year")
	}
	return &Date{t}, nil
}
func Day(t time.Time) *Date                 { d, _ := ParseDate(t.Format("2006-01-02")); return d }
func (d Date) String() string               { return d.Format("2006-01-02") }
func (d Date) AddDays(n int) *Date          { return &Date{d.AddDate(0, 0, n)} }
func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }
func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := ParseDate(s)
	if err == nil {
		*d = *v
	}
	return err
}

type TimeOfDay struct{ Hour, Minute int }

func ParseTime(s string) (*TimeOfDay, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return nil, err
	}
	return &TimeOfDay{t.Hour(), t.Minute()}, nil
}
func (t TimeOfDay) String() string               { return fmt.Sprintf("%02d:%02d", t.Hour, t.Minute) }
func (t TimeOfDay) MarshalJSON() ([]byte, error) { return json.Marshal(t.String()) }
func (t *TimeOfDay) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := ParseTime(s)
	if err == nil {
		*t = *v
	}
	return err
}
func (d Date) At(t TimeOfDay, loc *time.Location) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day(), t.Hour, t.Minute, 0, 0, loc)
}
