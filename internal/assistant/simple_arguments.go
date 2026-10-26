package assistant

import (
	"errors"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
)

func simpleCreationQuestion(c domain.Command) string {
	if c.Kind == domain.EventMeeting && domain.GenericMeetingTitle(c.Title) {
		return "С кем встреча или по какому поводу?"
	}
	if strings.TrimSpace(c.Title) == "" {
		return "Что нужно сделать или с кем провести встречу?"
	}
	if r := c.Recurrence; r != nil {
		if c.Kind == domain.EventMeeting && !r.Bounded() {
			return "Сколько встреч создать или до какой даты повторять?"
		}
		if (r.Frequency == "month" || r.Frequency == "year") && r.Day == 0 {
			return "Какого числа повторять запись?"
		}
		if r.Frequency == "week" && len(r.Weekdays) == 0 {
			return "В какие дни недели повторять запись?"
		}
		if !r.Elapsed() && c.Time == nil {
			return "Во сколько повторять запись?"
		}
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
	if c.Date != nil && c.Time != nil && c.ExactAt == nil {
		instant := c.Date.At(*c.Time, now.Location())
		if instant.Hour() != c.Time.Hour || instant.Minute() != c.Time.Minute {
			c.Time = nil
			c.Error = "В этот день такого местного времени нет из-за перевода часов. Укажите другое время."
		}
	}
	return nil
}
func simpleChanges(c *domain.Command, ch nlu.Changes, now time.Time) error {
	if ch.Occurrence != nil {
		if ch.Scope != "occurrence" {
			return errors.New("Для даты отдельного повторения укажите scope occurrence")
		}
		if ch.Occurrence.Type == "relative_delay" || ch.Occurrence.Type == "calendar_period" || ch.Occurrence.Type == "rolling_period" || ch.Occurrence.Type == "date_range" {
			return errors.New("Укажите исходную дату одного повторения")
		}
		date, _, at, err := ch.Occurrence.Resolve(now)
		if err != nil {
			return err
		}
		if c.Changes == nil {
			c.Changes = &domain.ExecutionChanges{}
		}
		c.Changes.OccurrenceDate, c.Changes.OccurrenceAt = date, at
	}
	if ch.Scope != "" {
		if ch.Scope != "series" && ch.Scope != "occurrence" || c.Action != domain.ActionUpdate {
			return errors.New("Укажите изменение всей серии или ближайшего повторения")
		}
		if ch.Scope == "occurrence" && (ch.Recurrence != nil || ch.ClearRecurrence) {
			return errors.New("Для одного повторения нельзя менять правило серии")
		}
		if c.Changes == nil {
			c.Changes = &domain.ExecutionChanges{}
		}
		c.Changes.Scope = ch.Scope
	}
	if ch.ClearRecurrence && ch.Recurrence != nil {
		return errors.New("Укажите повторение или его отмену")
	}
	if ch.DurationMinutes != nil && (*ch.DurationMinutes < 5 || *ch.DurationMinutes > 1440) {
		return errors.New("Длительность должна быть от 5 минут до суток.")
	}
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
	if ch.Recurrence != nil {
		r, err := ch.Recurrence.Bind(now, c.Date, c.Time)
		if err != nil {
			return err
		}
		if c.Action == domain.ActionCreate {
			c.Recurrence = r
		} else {
			if c.Changes == nil {
				c.Changes = &domain.ExecutionChanges{}
			}
			c.Changes.Recurrence = r
		}
	}
	if ch.ClearRecurrence {
		if c.Action == domain.ActionCreate {
			c.Recurrence = nil
		} else {
			if c.Changes == nil {
				c.Changes = &domain.ExecutionChanges{}
			}
			c.Changes.ClearRecurrence = true
		}
	}
	if c.Action == domain.ActionCreate && c.Recurrence != nil {
		if err := resolveSimpleRecurrence(c, now); err != nil {
			return err
		}
	}
	if ch.ReminderMinutes != nil {
		c.ReminderMinutes = ch.ReminderMinutes
	}
	if ch.ReminderEnabled != nil {
		c.ReminderEnabled = ch.ReminderEnabled
	}
	if ch.DurationMinutes != nil && c.Action == domain.ActionCreate {
		c.Duration = time.Duration(*ch.DurationMinutes) * time.Minute
	}
	if ch.ShiftMinutes != nil || ch.Status != nil || ch.DurationMinutes != nil && c.Action != domain.ActionCreate {
		changes := domain.ExecutionChanges{}
		if c.Changes != nil {
			changes = *c.Changes
		}
		c.Changes = &changes
		if ch.ShiftMinutes != nil {
			c.Changes.TimeShiftMinutes = ch.ShiftMinutes
		}
		if ch.DurationMinutes != nil {
			c.Changes.DurationMinutes = ch.DurationMinutes
		}
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

func resolveSimpleRecurrence(c *domain.Command, now time.Time) error {
	if c.Recurrence == nil {
		return nil
	}
	r := *c.Recurrence
	if !r.Elapsed() {
		r.LocalTime = c.Time
		if r.LocalTime == nil || r.Frequency == "week" && len(r.Weekdays) == 0 || (r.Frequency == "month" || r.Frequency == "year") && r.Day == 0 {
			c.Recurrence = &r
			return nil
		}
		if r.Anchor.IsZero() {
			r.Anchor = domain.Day(now).At(*c.Time, now.Location()).UTC()
		}
	}
	if err := r.Validate(true); err != nil {
		return err
	}
	next, err := r.Next(now)
	if err != nil {
		return err
	}
	next = next.In(now.Location())
	c.Recurrence = &r
	c.Date = domain.Day(next)
	c.Time = &domain.TimeOfDay{Hour: next.Hour(), Minute: next.Minute()}
	c.StartSecond = next.Second()
	return nil
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
		v.Scope = c.Changes.Scope
		if c.Changes.OccurrenceAt != nil {
			v.Occurrence = &nlu.Temporal{Type: "absolute_time", Date: c.Changes.OccurrenceAt.Format(time.RFC3339)}
		} else if c.Changes.OccurrenceDate != nil {
			v.Occurrence = &nlu.Temporal{Type: "absolute", Date: c.Changes.OccurrenceDate.String()}
		}
		v.DurationMinutes = c.Changes.DurationMinutes
		v.ShiftMinutes = c.Changes.TimeShiftMinutes
		if c.Changes.Status != nil {
			status := c.Changes.Status.String()
			v.Status = &status
		}
	}
	return v
}
