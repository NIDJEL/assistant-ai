package nlu

import (
	"fmt"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

// ValidationIssue never contains source text or provider diagnostic bodies.
type ValidationIssue struct {
	Code  string `json:"code"`
	Field string `json:"field"`
}

func (v ValidationIssue) Error() string { return fmt.Sprintf("%s at %s", v.Code, v.Field) }

func ValidateAt(op Operation, now time.Time) error {
	a, err := op.Args()
	if err != nil {
		return ValidationIssue{"invalid_contract", "operation"}
	}
	bad := func(code, field string) error { return ValidationIssue{code, field} }
	clock := func(value *string) error {
		if value != nil {
			if _, e := domain.ParseTime(*value); e != nil {
				return bad("invalid_time", "arguments.time")
			}
		}
		return nil
	}
	date := func(value *Temporal, concrete bool) error {
		if value != nil {
			if _, _, _, e := value.Resolve(now); e != nil {
				return bad("invalid_date", "arguments.date")
			}
			if concrete && (value.Type == "calendar_period" || value.Type == "rolling_period" || value.Type == "date_range") {
				return bad("specific_day_required", "arguments.date")
			}
		}
		return nil
	}
	ints := func(duration, reminder *int) error {
		if duration != nil && (*duration < 5 || *duration > 1440) {
			return bad("invalid_duration", "arguments.duration_minutes")
		}
		if reminder != nil && (*reminder < 0 || *reminder > 525600) {
			return bad("invalid_reminder", "arguments.reminder_minutes")
		}
		return nil
	}
	changes := func(c Changes) error {
		if c.Occurrence != nil {
			if c.Scope != "occurrence" {
				return bad("invalid_occurrence_scope", "arguments.changes.occurrence")
			}
			if e := date(c.Occurrence, false); e != nil {
				return e
			}
		}
		if c.Scope != "" && c.Scope != "series" && c.Scope != "occurrence" || c.Scope == "occurrence" && (c.Recurrence != nil || c.ClearRecurrence) {
			return bad("invalid_recurrence_scope", "arguments.changes.scope")
		}
		if c.ClearRecurrence && c.Recurrence != nil {
			return bad("incompatible_recurrence", "arguments.changes")
		}
		if c.Recurrence != nil {
			if _, e := c.Recurrence.Bind(now, nil, nil); e != nil {
				return bad("invalid_recurrence", "arguments.recurrence")
			}
		}
		if err := clock(c.Time); err != nil {
			return err
		}
		if err := date(c.Date, true); err != nil {
			return err
		}
		if c.ShiftMinutes != nil && (c.Date != nil || c.Time != nil || *c.ShiftMinutes == 0 || *c.ShiftMinutes < -525600 || *c.ShiftMinutes > 525600) {
			return bad("invalid_shift", "arguments.changes")
		}
		if c.Title != nil && (strings.TrimSpace(*c.Title) == "" || len([]rune(*c.Title)) > 500) {
			return bad("invalid_title", "arguments.title")
		}
		if c.Status != nil && *c.Status != "open" && *c.Status != "done" {
			return bad("invalid_status", "arguments.status")
		}
		return ints(c.DurationMinutes, c.ReminderMinutes)
	}
	filter := func(f Filter) error {
		if f.Date != nil && f.Period != nil {
			return bad("ambiguous_period", "arguments.filter")
		}
		if f.Kind != "" && f.Kind != "any" {
			if _, e := domain.ParseEventType(f.Kind); e != nil {
				return bad("invalid_kind", "arguments.kind")
			}
		}
		if e := date(f.Date, false); e != nil {
			return e
		}
		return date(f.Period, false)
	}
	switch v := a.(type) {
	case *Create:
		if v.Recurrence != nil {
			if _, e := v.Recurrence.Bind(now, nil, nil); e != nil {
				return bad("invalid_recurrence", "arguments.recurrence")
			}
		}
		if len([]rune(v.Title)) > 500 {
			return bad("invalid_title", "arguments.title")
		}
		if e := clock(v.Time); e != nil {
			return e
		}
		if e := date(v.Date, true); e != nil {
			return e
		}
		return ints(v.DurationMinutes, v.ReminderMinutes)
	case *Continue:
		return changes(v.Changes)
	case *SchedulingReply:
		return changes(v.Changes)
	case *Update:
		return changes(v.Changes)
	case *Read:
		return filter(v.Filter)
	case *BatchUpdate:
		if v.Changes.Scope == "occurrence" {
			return bad("single_target_required", "arguments.changes.scope")
		}
		if e := filter(v.Filter); e != nil {
			return e
		}
		for _, f := range v.Exclude {
			if e := filter(f); e != nil {
				return e
			}
		}
		return changes(v.Changes)
	case *Batch:
		if e := filter(v.Filter); e != nil {
			return e
		}
		for _, f := range v.Exclude {
			if e := filter(f); e != nil {
				return e
			}
		}
	case *Social:
		if e := date(v.Date, true); e != nil {
			return e
		}
		if e := clock(v.Time); e != nil {
			return e
		}
		return ints(v.DurationMinutes, nil)
	case *ResumeAction:
		return ValidateAt(v.Operation, now)
	}
	return nil
}
