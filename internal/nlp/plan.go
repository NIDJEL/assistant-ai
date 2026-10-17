package nlp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"unicode/utf8"
)

// SemanticPlan is the provider-neutral language boundary. It has no database
// IDs, absolute timestamps, SQL, tools, repository or execution permissions.
type SemanticPlan struct {
	Status        string         `json:"status"`
	DialogueAct   string         `json:"dialogue_act"`
	Operation     *string        `json:"operation"`
	Kind          *string        `json:"kind"`
	Title         *string        `json:"title"`
	Query         *string        `json:"query"`
	Filters       *PlanFilters   `json:"filters"`
	Target        *PlanTarget    `json:"target"`
	Schedule      *PlanSchedule  `json:"schedule"`
	Changes       *PlanChanges   `json:"changes"`
	Evidence      []PlanEvidence `json:"evidence"`
	Clarification *string        `json:"clarification"`
	ChatResponse  *string        `json:"chat_response"`
}

// Field is a dotted DTO path (array indices are zero based). Text is copied
// verbatim from the current original message, never from dialogue history.
type PlanEvidence struct {
	Field string `json:"field"`
	Text  string `json:"text"`
}

type PlanFilters struct {
	Status     *string          `json:"status"`
	Priority   *string          `json:"priority"`
	Period     *TemporalMeaning `json:"period"`
	TimeFrom   *ClockMeaning    `json:"time_from"`
	TimeBefore *ClockMeaning    `json:"time_before"`
	Exclude    []PlanExclusion  `json:"exclude"`
}
type PlanExclusion struct {
	Query    *string          `json:"query"`
	Kind     *string          `json:"kind"`
	Status   *string          `json:"status"`
	Priority *string          `json:"priority"`
	Period   *TemporalMeaning `json:"period"`
}
type PlanTarget struct {
	Reference *string `json:"reference"`
	Ordinal   *int    `json:"ordinal"`
	Query     *string `json:"query"`
}
type PlanSchedule struct {
	Date            *TemporalMeaning `json:"date"`
	Time            *ClockMeaning    `json:"time"`
	Delay           *TemporalMeaning `json:"delay"`
	DurationMinutes *int             `json:"duration_minutes"`
	ReminderMinutes *int             `json:"reminder_minutes"`
	ReminderEnabled *bool            `json:"reminder_enabled"`
	Recurrence      *PlanRecurrence  `json:"recurrence"`
}
type PlanRecurrence struct {
	Frequency string   `json:"frequency"`
	Interval  int      `json:"interval"`
	Weekdays  []string `json:"weekdays"`
	Day       *int     `json:"day"`
	Month     *int     `json:"month"`
}
type PlanChanges struct {
	Title            *string          `json:"title"`
	Date             *TemporalMeaning `json:"date"`
	Time             *ClockMeaning    `json:"time"`
	Delay            *TemporalMeaning `json:"delay"`
	TimeShiftMinutes *int             `json:"time_shift_minutes"`
	Priority         *string          `json:"priority"`
	Status           *string          `json:"status"`
	ReminderMinutes  *int             `json:"reminder_minutes"`
	ReminderEnabled  *bool            `json:"reminder_enabled"`
}

func (p SemanticPlan) Validate() error {
	p = CanonicalizePlan(p)
	bad := errors.New("incompatible semantic plan")
	// Also validate programmatic providers, not just JSON decoding providers.
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if err = validatePlanShape(raw, SemanticPlanSchema()); err != nil {
		return err
	}
	if p.Status != "ok" {
		if p.Operation != nil || p.Kind != nil || p.Title != nil || p.Query != nil || p.Filters != nil || p.Target != nil || p.Schedule != nil || p.Changes != nil || p.ChatResponse != nil {
			return bad
		}
		return nil
	}
	op := semanticString(p.Operation)
	if op == "" {
		return bad
	}
	if p.DialogueAct == "confirmation" || p.DialogueAct == "cancellation" {
		return errors.New("model cannot authorize confirmation or cancellation")
	}
	if op == "chat" {
		if p.DialogueAct != "command" || p.Kind != nil || p.Title != nil || p.Query != nil || p.Filters != nil || p.Target != nil || p.Schedule != nil || p.Changes != nil || p.ChatResponse == nil {
			return bad
		}
		return nil
	}
	if p.ChatResponse != nil {
		return bad
	}
	if p.Filters != nil {
		if err := p.Filters.Validate(); err != nil {
			return err
		}
	}
	if p.Target != nil {
		t := p.Target
		switch semanticString(t.Reference) {
		case "last":
			if t.Ordinal != nil || t.Query != nil {
				return bad
			}
		case "ordinal":
			if t.Ordinal == nil || *t.Ordinal < 1 || *t.Ordinal > 30 || t.Query != nil {
				return bad
			}
		case "query":
			if t.Query == nil || t.Ordinal != nil {
				return bad
			}
		default:
			return bad
		}
	}
	if p.Schedule != nil {
		if err := p.Schedule.Validate(); err != nil {
			return err
		}
	}
	if p.Changes != nil {
		if err := p.Changes.Validate(); err != nil {
			return err
		}
	}
	if p.DialogueAct == "slot_reply" {
		if op != "update" || p.Kind != nil || p.Title != nil || p.Query != nil || p.Filters != nil || p.Target != nil || p.Schedule != nil || p.Changes == nil {
			return bad
		}
		return nil
	}
	if p.DialogueAct == "choice" {
		if op != "update" || p.Kind != nil || p.Title != nil || p.Query != nil || p.Filters != nil || p.Schedule != nil || p.Changes != nil || p.Target == nil || p.Target.Query != nil {
			return bad
		}
		return nil
	}
	if p.DialogueAct == "refinement" && !oneOf(op, "list", "search") {
		return bad
	}
	if p.DialogueAct == "correction" {
		if op != "update" || p.Target != nil || p.Title != nil || p.Query != nil || p.Filters != nil || p.Schedule != nil || p.Changes == nil {
			return bad
		}
		return nil
	}
	switch op {
	case "create":
		if p.DialogueAct != "command" || p.Title == nil || p.Query != nil || p.Filters != nil || p.Target != nil || p.Changes != nil || p.Kind == nil || *p.Kind == "any" {
			return bad
		}
	case "list", "search":
		if p.Title != nil || p.Changes != nil || p.Schedule != nil || op == "list" && (p.Query != nil || p.Target != nil) || op == "search" && p.Query == nil && p.DialogueAct != "refinement" && p.Target == nil {
			return bad
		}
		if p.Target != nil && (p.DialogueAct != "command" || p.Query != nil || p.Filters != nil || semanticString(p.Target.Reference) == "query") {
			return bad
		}
	case "update", "delete", "complete", "batch_update", "batch_delete", "batch_complete":
		if p.DialogueAct != "command" || p.Title != nil || p.Schedule != nil {
			return bad
		}
		batch := strings.HasPrefix(op, "batch_")
		if batch && p.Target != nil || !batch && (p.Target == nil || p.Query != nil) {
			return bad
		}
		if batch && p.Kind == nil && p.Query == nil && p.Filters == nil {
			return errors.New("unbounded batch requires explicit any kind")
		}
		if oneOf(op, "update", "batch_update") {
			if p.Changes == nil {
				return bad
			}
		} else if p.Changes != nil {
			return bad
		}
	default:
		return bad
	}
	return nil
}

func (f PlanFilters) Validate() error {
	if f.Status != nil && !oneOf(*f.Status, "open", "done", "any") || f.Priority != nil && !oneOf(*f.Priority, "low", "normal", "high") {
		return errors.New("invalid semantic filter enum")
	}
	if f.Period != nil {
		if err := f.Period.Validate(); err != nil {
			return err
		}
		if f.Period.Type == "relative_delay" {
			return errors.New("delay is not a search period")
		}
	}
	for _, c := range []*ClockMeaning{f.TimeFrom, f.TimeBefore} {
		if c != nil {
			if err := c.Validate(); err != nil {
				return err
			}
		}
	}
	if f.TimeFrom != nil && f.TimeBefore != nil && f.TimeFrom.Hour*60+f.TimeFrom.Minute >= f.TimeBefore.Hour*60+f.TimeBefore.Minute {
		return errors.New("empty clock filter")
	}
	if len(f.Exclude) > 20 {
		return errors.New("too many exclusions")
	}
	for _, e := range f.Exclude {
		if e.Kind != nil && !oneOf(*e.Kind, "task", "meeting", "reminder", "any") {
			return errors.New("invalid exclusion kind")
		}
		if err := (PlanFilters{Status: e.Status, Priority: e.Priority}).Validate(); err != nil {
			return err
		}
		if e.Query == nil && e.Kind == nil && e.Status == nil && e.Priority == nil && e.Period == nil {
			return errors.New("empty exclusion")
		}
		if e.Period != nil {
			if err := (PlanFilters{Period: e.Period}).Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}
func validateSchedule(date *TemporalMeaning, clock *ClockMeaning, delay *TemporalMeaning, reminder *int, enabled *bool) error {
	if date != nil {
		if err := date.Validate(); err != nil {
			return err
		}
		if !oneOf(date.Type, "relative_day", "weekday", "calendar_date") {
			return errors.New("schedule needs one calendar date")
		}
	}
	if clock != nil {
		if err := clock.Validate(); err != nil {
			return err
		}
	}
	if delay != nil {
		if err := delay.Validate(); err != nil {
			return err
		}
		if delay.Type != "relative_delay" || date != nil || clock != nil {
			return errors.New("delay conflicts with date/clock")
		}
	}
	if reminder != nil && (*reminder < 0 || *reminder > 525600 || enabled != nil && !*enabled) {
		return errors.New("invalid reminder policy")
	}
	return nil
}
func (s PlanSchedule) Validate() error {
	if err := validateSchedule(s.Date, s.Time, s.Delay, s.ReminderMinutes, s.ReminderEnabled); err != nil {
		return err
	}
	if s.DurationMinutes != nil && (*s.DurationMinutes < 5 || *s.DurationMinutes > 1440) {
		return errors.New("invalid duration")
	}
	if r := s.Recurrence; r != nil {
		if s.Delay != nil || !oneOf(r.Frequency, "minute", "hour", "day", "week", "month", "year") || r.Interval < 1 || r.Interval > 366 {
			return errors.New("invalid recurrence")
		}
		if oneOf(r.Frequency, "minute", "hour") && (s.Date != nil || s.Time != nil) {
			return errors.New("elapsed recurrence has no civil clock")
		}
		if r.Frequency != "week" && len(r.Weekdays) > 0 || !oneOf(r.Frequency, "month", "year") && r.Day != nil || r.Frequency != "year" && r.Month != nil {
			return errors.New("incompatible recurrence")
		}
		seen := map[string]bool{}
		for _, w := range r.Weekdays {
			if !oneOf(w, "monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday") || seen[w] {
				return errors.New("invalid recurrence weekday")
			}
			seen[w] = true
		}
		if r.Frequency == "week" && len(r.Weekdays) == 0 || oneOf(r.Frequency, "month", "year") && (r.Day == nil || *r.Day < 1 || *r.Day > 31) || r.Frequency == "year" && (r.Month == nil || *r.Month < 1 || *r.Month > 12) {
			return errors.New("incomplete recurrence")
		}
	}
	return nil
}
func (c PlanChanges) Validate() error {
	if c.Status != nil && !oneOf(*c.Status, "open", "done") {
		return errors.New("invalid change status")
	}
	if err := validateSchedule(c.Date, c.Time, c.Delay, c.ReminderMinutes, c.ReminderEnabled); err != nil {
		return err
	}
	if c.TimeShiftMinutes != nil && (*c.TimeShiftMinutes == 0 || *c.TimeShiftMinutes < -525600 || *c.TimeShiftMinutes > 525600 || c.Date != nil || c.Time != nil || c.Delay != nil) {
		return errors.New("invalid or conflicting time shift")
	}
	if c.Title == nil && c.Date == nil && c.Time == nil && c.Delay == nil && c.TimeShiftMinutes == nil && c.Priority == nil && c.Status == nil && c.ReminderMinutes == nil && c.ReminderEnabled == nil {
		return errors.New("empty changes")
	}
	return nil
}

// Schema is derived from the DTO: every object disallows extra properties and
// requires all its fields. Optional values are explicitly nullable.
func SemanticPlanSchema() map[string]any { return planTypeSchema(reflect.TypeFor[SemanticPlan](), "") }
func planTypeSchema(t reflect.Type, field string) map[string]any {
	nullable := t.Kind() == reflect.Pointer
	if nullable {
		t = t.Elem()
	}
	s := map[string]any{}
	switch t.Kind() {
	case reflect.Struct:
		props := map[string]any{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := f.Tag.Get("json")
			props[name] = planTypeSchema(f.Type, name)
			required = append(required, name)
		}
		s = map[string]any{"type": "object", "additionalProperties": false, "properties": props, "required": required}
	case reflect.Slice:
		s = map[string]any{"type": []string{"array", "null"}, "items": planTypeSchema(t.Elem(), field), "maxItems": 30}
	case reflect.String:
		s = map[string]any{"type": "string", "minLength": 1, "maxLength": 500}
		values := map[string][]string{
			"status": {"ok", "ambiguous", "unknown"}, "dialogue_act": {"command", "refinement", "correction", "slot_reply", "choice", "confirmation", "cancellation"},
			"operation": {"create", "list", "search", "update", "delete", "complete", "batch_update", "batch_delete", "batch_complete", "chat"},
			"kind":      {"task", "meeting", "reminder", "any"}, "priority": {"low", "normal", "high"}, "reference": {"last", "ordinal", "query"},
			"type": {"calendar_period", "relative_day", "relative_delay", "weekday", "calendar_date", "rolling_period"},
			"unit": {"second", "minute", "hour", "day", "week", "month", "quarter", "year"}, "direction": {"next_or_current", "next_week", "previous_week"},
			"weekday": {"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}, "weekdays": {"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"},
			"frequency": {"minute", "hour", "day", "week", "month", "year"},
		}[field]
		if field == "status" && nullable {
			values = []string{"open", "done", "any"}
		}
		if len(values) > 0 {
			e := []any{}
			for _, v := range values {
				e = append(e, v)
			}
			if nullable {
				e = append(e, nil)
			}
			s["enum"] = e
		}
	case reflect.Int:
		s = map[string]any{"type": "integer", "minimum": -31536000, "maximum": 31536000}
	case reflect.Bool:
		s = map[string]any{"type": "boolean"}
	}
	if nullable {
		s["type"] = []string{s["type"].(string), "null"}
	}
	return s
}

func DecodeSemanticPlan(raw []byte) (SemanticPlan, error) {
	return DecodeSemanticPlanSource(raw, "")
}

// Source-aware canonicalization runs after strict JSON shape checking but
// before compatibility validation, including short timer durations.
func DecodeSemanticPlanSource(raw []byte, source string) (SemanticPlan, error) {
	var p SemanticPlan
	schema := SemanticPlanSchema()
	// Only an empty search title is a neutral sentinel. Duplicate keys and all
	// other strict shape rules are still checked by validatePlanShape.
	var header struct {
		Operation *string `json:"operation"`
	}
	if json.Unmarshal(raw, &header) == nil && semanticString(header.Operation) == "search" {
		schema["properties"].(map[string]any)["title"].(map[string]any)["minLength"] = 0
	}
	if err := validatePlanShape(raw, schema); err != nil {
		return p, err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return p, err
	}
	p, _ = CanonicalizePlanSource(p, source)
	return p, p.Validate()
}

func validatePlanShape(raw []byte, schema map[string]any) error {
	if len(raw) > 65536 {
		return errors.New("semantic plan too large")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var read func(int) (any, error)
	read = func(depth int) (any, error) {
		if depth > 24 {
			return nil, errors.New("semantic plan too deep")
		}
		t, err := d.Token()
		if err != nil {
			return nil, err
		}
		switch t {
		case json.Delim('{'):
			m := map[string]any{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return nil, e
				}
				key, ok := k.(string)
				if !ok {
					return nil, errors.New("invalid key")
				}
				if _, exists := m[key]; exists {
					return nil, errors.New("duplicate semantic field")
				}
				v, e := read(depth + 1)
				if e != nil {
					return nil, e
				}
				m[key] = v
			}
			_, err = d.Token()
			return m, err
		case json.Delim('['):
			a := []any{}
			for d.More() {
				v, e := read(depth + 1)
				if e != nil {
					return nil, e
				}
				a = append(a, v)
			}
			_, err = d.Token()
			return a, err
		default:
			return t, nil
		}
	}
	v, err := read(0)
	if err != nil {
		return err
	}
	if _, err = d.Token(); err != io.EOF {
		return errors.New("trailing semantic JSON")
	}
	return checkPlanValue(v, schema)
}
func checkPlanValue(v any, s map[string]any) error {
	bad := errors.New("semantic JSON violates strict schema")
	allowed := []string{}
	switch t := s["type"].(type) {
	case string:
		allowed = []string{t}
	case []string:
		allowed = t
	}
	kind := "null"
	switch v.(type) {
	case map[string]any:
		kind = "object"
	case []any:
		kind = "array"
	case string:
		kind = "string"
	case bool:
		kind = "boolean"
	case json.Number:
		kind = "integer"
	}
	if !oneOf(kind, allowed...) {
		return bad
	}
	if enum, ok := s["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			if reflect.DeepEqual(v, e) {
				found = true
			}
		}
		if !found {
			return bad
		}
	}
	switch x := v.(type) {
	case map[string]any:
		props := s["properties"].(map[string]any)
		for _, k := range s["required"].([]string) {
			if _, ok := x[k]; !ok {
				return fmt.Errorf("missing semantic field %s", k)
			}
		}
		for k, val := range x {
			sub, ok := props[k]
			if !ok {
				return fmt.Errorf("unknown semantic field %s", k)
			}
			if err := checkPlanValue(val, sub.(map[string]any)); err != nil {
				return fmt.Errorf("%s: %w", k, err)
			}
		}
	case []any:
		if len(x) > s["maxItems"].(int) {
			return bad
		}
		for _, val := range x {
			if err := checkPlanValue(val, s["items"].(map[string]any)); err != nil {
				return err
			}
		}
	case string:
		if strings.TrimSpace(x) == "" && s["minLength"] != 0 || utf8.RuneCountInString(x) > s["maxLength"].(int) {
			return bad
		}
	case json.Number:
		n, err := x.Int64()
		if err != nil || n < int64(s["minimum"].(int)) || n > int64(s["maximum"].(int)) {
			return bad
		}
	}
	return nil
}
