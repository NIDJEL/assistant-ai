package nlp

import (
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

func MaterializePlan(p SemanticPlan, now time.Time) (domain.Command, error) {
	p = CanonicalizePlan(p)
	c := domain.Command{Action: domain.ActionHelp, Duration: time.Hour, Provider: "semantic_llm"}
	if err := p.Validate(); err != nil {
		return c, err
	}
	if p.Status != "ok" {
		return c, nil
	}
	op := semanticString(p.Operation)
	c.Action = map[string]domain.Action{"create": domain.ActionCreate, "list": domain.ActionList, "search": domain.ActionSearch, "update": domain.ActionUpdate, "delete": domain.ActionDelete, "complete": domain.ActionComplete, "batch_update": domain.ActionUpdate, "batch_delete": domain.ActionDelete, "batch_complete": domain.ActionComplete, "chat": domain.ActionHelp}[op]
	c.Kind, _ = domain.ParseEventType(semanticString(p.Kind))
	c.Title = semanticString(p.Title)
	c.Query = semanticString(p.Query)
	c.Batch = strings.HasPrefix(op, "batch_")
	if p.Target != nil && p.Target.Query != nil {
		c.Query = *p.Target.Query
	}
	if p.Filters != nil {
		f, err := MaterializeFilters(*p.Filters, now)
		if err != nil {
			return c, err
		}
		c.Filter = &f
	}
	if p.Schedule != nil {
		s := p.Schedule
		if err := applyNormalizedSchedule(&c, s.Date, s.Time, s.Delay, now); err != nil {
			return c, err
		}
		c.ReminderMinutes, c.ReminderEnabled = s.ReminderMinutes, s.ReminderEnabled
		if s.DurationMinutes != nil {
			c.Duration = time.Duration(*s.DurationMinutes) * time.Minute
		}
		if s.Recurrence != nil {
			r := s.Recurrence
			c.Recurrence = &domain.Recurrence{Frequency: r.Frequency, Interval: r.Interval, Timezone: now.Location().String()}
			if r.Day != nil {
				c.Recurrence.Day = *r.Day
			}
			if r.Month != nil {
				c.Recurrence.Month = time.Month(*r.Month)
			}
			weekdays := map[string]time.Weekday{"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday, "wednesday": time.Wednesday, "thursday": time.Thursday, "friday": time.Friday, "saturday": time.Saturday}
			for _, w := range r.Weekdays {
				c.Recurrence.Weekdays = append(c.Recurrence.Weekdays, weekdays[w])
			}
			c.Recurrence.Anchor = now
			if c.Date != nil && c.Time != nil {
				c.Recurrence.Anchor = c.Date.At(*c.Time, now.Location())
			}
			// Existing recurrence engine performs only calendar arithmetic here.
			ResolveRecurrence(&c, now)
		}
	}
	if p.Changes != nil {
		if err := ApplyNormalizedChanges(&c, *p.Changes, now); err != nil {
			return c, err
		}
	}
	if c.Filter != nil {
		c.Filter.Query, c.Filter.Kind = c.Query, c.Kind
		c.PeriodStart, c.PeriodEnd = c.Filter.Start, c.Filter.End
		if c.Action != domain.ActionList && c.Action != domain.ActionSearch {
			c.Selector = &domain.EventSelector{All: c.Batch, Start: c.Filter.Start, End: c.Filter.End}
		}
	} else if c.Batch {
		c.Selector = &domain.EventSelector{All: true}
	}
	return c, c.Validate()
}

func applyNormalizedSchedule(c *domain.Command, date *TemporalMeaning, clock *ClockMeaning, delay *TemporalMeaning, now time.Time) error {
	if date != nil {
		r, err := ResolveTemporal(*date, now)
		if err != nil {
			return err
		}
		c.Date = r.Start
		c.ExactAt = nil
	}
	if clock != nil {
		c.Time = &domain.TimeOfDay{Hour: clock.Hour, Minute: clock.Minute}
		c.StartSecond = 0
		c.ExactAt = nil
	}
	if delay != nil {
		r, err := ResolveTemporal(*delay, now)
		if err != nil {
			return err
		}
		c.ExactAt = r.At
		c.Date = r.Start
		c.Time = &domain.TimeOfDay{Hour: r.At.Hour(), Minute: r.At.Minute()}
		c.StartSecond = r.At.Second()
	}
	return nil
}
func ApplyNormalizedChanges(c *domain.Command, ch PlanChanges, now time.Time) error {
	if err := ch.Validate(); err != nil {
		return err
	}
	if err := applyNormalizedSchedule(c, ch.Date, ch.Time, ch.Delay, now); err != nil {
		return err
	}
	if ch.Title != nil {
		c.Title = *ch.Title
	}
	if ch.ReminderMinutes != nil {
		c.ReminderMinutes = ch.ReminderMinutes
		yes := true
		c.ReminderEnabled = &yes
	}
	if ch.ReminderEnabled != nil {
		c.ReminderEnabled = ch.ReminderEnabled
	}
	if ch.Priority != nil || ch.Status != nil || ch.TimeShiftMinutes != nil {
		c.Changes = &domain.ExecutionChanges{TimeShiftMinutes: ch.TimeShiftMinutes}
		if ch.Priority != nil {
			v, err := domain.ParsePriority(*ch.Priority)
			if err != nil {
				return err
			}
			c.Changes.Priority = &v
		}
		if ch.Status != nil {
			v, err := domain.ParseStatus(*ch.Status)
			if err != nil {
				return err
			}
			c.Changes.Status = &v
		}
	}
	return nil
}

func MaterializeFilters(f PlanFilters, now time.Time) (domain.ExecutionFilter, error) {
	r := domain.ExecutionFilter{}
	if err := f.Validate(); err != nil {
		return r, err
	}
	if f.Status != nil {
		v := domain.StatusUnknown
		if *f.Status != "any" {
			v, _ = domain.ParseStatus(*f.Status)
		}
		r.Status = &v
	}
	if f.Priority != nil {
		v, _ := domain.ParsePriority(*f.Priority)
		r.Priority = &v
	}
	if f.Period != nil {
		t, err := ResolveTemporal(*f.Period, now)
		if err != nil {
			return r, err
		}
		r.Start, r.End = t.Start, t.End
	}
	if f.TimeFrom != nil {
		r.TimeFrom = &domain.TimeOfDay{Hour: f.TimeFrom.Hour, Minute: f.TimeFrom.Minute}
	}
	if f.TimeBefore != nil {
		r.TimeBefore = &domain.TimeOfDay{Hour: f.TimeBefore.Hour, Minute: f.TimeBefore.Minute}
	}
	for _, e := range f.Exclude {
		x, err := MaterializeFilters(PlanFilters{Status: e.Status, Priority: e.Priority, Period: e.Period}, now)
		if err != nil {
			return r, err
		}
		x.Query = semanticString(e.Query)
		x.Kind, _ = domain.ParseEventType(semanticString(e.Kind))
		r.Exclude = append(r.Exclude, x)
	}
	return r, nil
}
