package assistant

import (
	"errors"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
)

func simpleCreationQuestion(c domain.Command) string {
	if strings.TrimSpace(c.Title) == "" {
		return "Что нужно сделать или с кем провести встречу?"
	}
	if c.Date == nil && (c.Kind != domain.EventTask || c.Time != nil) {
		return "На какую дату поставить запись?"
	}
	if c.Time == nil && c.Kind != domain.EventTask {
		return "Во сколько?"
	}
	return ""
}
func simpleDate(c *domain.Command, t *nlu.Temporal, clock *string, now time.Time) error {
	if t != nil {
		if t.Type == "calendar_period" || t.Type == "rolling_period" || t.Type == "date_range" {
			return errors.New("Для записи нужен конкретный день.")
		}
		start, _, exact, err := t.Resolve(now)
		if err != nil {
			return err
		}
		c.Date, c.ExactAt = start, exact
		if exact != nil {
			if clock != nil {
				return errors.New("Укажите время или задержку, а не оба сразу.")
			}
			c.Time = &domain.TimeOfDay{Hour: exact.Hour(), Minute: exact.Minute()}
			c.StartSecond = exact.Second()
		}
	}
	if clock != nil {
		v, err := domain.ParseTime(*clock)
		if err != nil {
			return err
		}
		c.Time, c.ExactAt, c.StartSecond = v, nil, 0
	}
	return nil
}
func simpleChanges(c *domain.Command, ch nlu.Changes, now time.Time) error {
	if ch.Title != nil {
		if strings.TrimSpace(*ch.Title) == "" {
			return errors.New("Название не должно быть пустым.")
		}
		c.Title = *ch.Title
	}
	if ch.ShiftMinutes != nil && (ch.Date != nil || ch.Time != nil) {
		return errors.New("Укажите сдвиг или новое время.")
	}
	if err := simpleDate(c, ch.Date, ch.Time, now); err != nil {
		return err
	}
	if ch.ReminderMinutes != nil {
		c.ReminderMinutes = ch.ReminderMinutes
	}
	if ch.ReminderEnabled != nil {
		c.ReminderEnabled = ch.ReminderEnabled
	}
	if ch.ShiftMinutes != nil || ch.Status != nil {
		c.Changes = &domain.ExecutionChanges{TimeShiftMinutes: ch.ShiftMinutes}
		if ch.Status != nil {
			status, err := domain.ParseStatus(*ch.Status)
			if err != nil || status == domain.StatusUnknown {
				return errors.New("Некорректный статус.")
			}
			c.Changes.Status = &status
		}
	}
	return c.Validate()
}
func simpleFilter(f nlu.Filter, now time.Time) (*domain.ExecutionFilter, error) {
	v := &domain.ExecutionFilter{Query: f.Query}
	if f.Conference != "" && f.Conference != "any" {
		v.Conference = f.Conference
	}
	if f.Kind != "" && f.Kind != "any" {
		kind, err := domain.ParseEventType(f.Kind)
		if err != nil {
			return nil, err
		}
		v.Kind = kind
	}
	if f.Status != nil {
		status := domain.StatusUnknown
		if *f.Status != "any" {
			var err error
			status, err = domain.ParseStatus(*f.Status)
			if err != nil || status == domain.StatusUnknown {
				return nil, errors.New("invalid filter status")
			}
		}
		v.Status = &status
	}
	if f.Date != nil && f.Period != nil {
		return nil, errors.New("Укажите один период.")
	}
	t := f.Date
	if t == nil {
		t = f.Period
	}
	if t != nil {
		if t.Type == "relative_delay" {
			return nil, errors.New("Для поиска нужен день или календарный период.")
		}
		var err error
		v.Start, v.End, _, err = t.Resolve(now)
		if err != nil {
			return nil, err
		}
	}
	return v, v.Validate()
}
func setSimpleFilter(c *domain.Command, f *domain.ExecutionFilter) {
	c.Filter, c.Query, c.Kind, c.PeriodStart, c.PeriodEnd = f, f.Query, f.Kind, f.Start, f.End
}
func displayFilter(f *domain.ExecutionFilter) *nlu.Filter {
	if f == nil {
		return nil
	}
	v := &nlu.Filter{Kind: f.Kind.String(), Query: f.Query, Conference: f.Conference}
	if f.Status != nil {
		status := f.Status.String()
		if status == "" {
			status = "any"
		}
		v.Status = &status
	}
	if f.Start != nil && f.End != nil {
		v.Period = &nlu.Temporal{Type: "date_range", Date: f.Start.String(), EndDate: f.End.String()}
	}
	return v
}
func displayChanges(c domain.Command) *nlu.Changes {
	v := &nlu.Changes{ReminderMinutes: c.ReminderMinutes, ReminderEnabled: c.ReminderEnabled}
	if c.Title != "" {
		title := c.Title
		v.Title = &title
	}
	if c.Date != nil {
		v.Date = &nlu.Temporal{Type: "absolute", Date: c.Date.String()}
	}
	if c.Time != nil {
		clock := c.Time.String()
		v.Time = &clock
	}
	if c.Changes != nil {
		v.ShiftMinutes = c.Changes.TimeShiftMinutes
		if c.Changes.Status != nil {
			status := c.Changes.Status.String()
			v.Status = &status
		}
	}
	return v
}
