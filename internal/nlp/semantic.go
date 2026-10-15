package nlp

// SemanticCommand is the language boundary, not a database or execution DTO.
// All temporal values are expressions; only Go is allowed to resolve them.
import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

type SemanticCommand struct {
	Status                 string           `json:"status"`
	Action                 *string          `json:"action"`
	Kind                   *string          `json:"kind"`
	Title                  *string          `json:"title"`
	Query                  *string          `json:"query"`
	DateExpression         *string          `json:"date_expression,omitempty"`
	TimeExpression         *string          `json:"time_expression,omitempty"`
	RelativeTimeExpression *string          `json:"relative_time_expression,omitempty"`
	PeriodExpression       *string          `json:"period_expression,omitempty"`
	DurationExpression     *string          `json:"duration_expression,omitempty"`
	RecurrenceExpression   *string          `json:"recurrence_expression,omitempty"`
	ReminderExpression     *string          `json:"reminder_expression,omitempty"`
	ReminderTimeExpression *string          `json:"reminder_time_expression,omitempty"`
	NotificationContext    bool             `json:"notification_context,omitempty"`
	Target                 *SemanticTarget  `json:"target"`
	Changes                *SemanticChanges `json:"changes"`
	Clarification          *string          `json:"clarification,omitempty"`
}

type SemanticTarget struct {
	Query            *string `json:"query"`
	DateExpression   *string `json:"date_expression,omitempty"`
	TimeExpression   *string `json:"time_expression,omitempty"`
	PeriodExpression *string `json:"period_expression,omitempty"`
	All              bool    `json:"all"`
}

type SemanticChanges struct {
	Title                  *string `json:"title"`
	DateExpression         *string `json:"date_expression,omitempty"`
	TimeExpression         *string `json:"time_expression,omitempty"`
	RelativeTimeExpression *string `json:"relative_time_expression,omitempty"`
}

func SemanticSchema() map[string]any {
	text := func(max int) map[string]any {
		return map[string]any{"type": []string{"string", "null"}, "minLength": 1, "maxLength": max}
	}
	expr := func() map[string]any { return text(300) }
	optional := func(properties map[string]any) []string {
		keys := make([]string, 0, len(properties))
		for key := range properties {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		return keys
	}
	targetProperties := map[string]any{
		"query": text(2000), "all": map[string]any{"type": "boolean"},
		"date_expression": expr(), "time_expression": expr(), "period_expression": expr(),
	}
	changesProperties := map[string]any{
		"title": text(500), "date_expression": expr(), "time_expression": expr(),
		"relative_time_expression": expr(),
	}
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"status", "action", "kind", "title", "query", "target", "changes", "date_expression", "time_expression", "relative_time_expression", "period_expression", "duration_expression", "recurrence_expression", "reminder_expression", "reminder_time_expression", "notification_context", "clarification"},
		"properties": map[string]any{
			"status": map[string]any{"enum": []string{"ok", "ambiguous", "unknown"}},
			"action": map[string]any{"enum": []any{"create", "list", "search", "update", "complete", "delete", "help", nil}},
			"kind":   map[string]any{"enum": []any{"task", "meeting", "reminder", nil}},
			"title":  text(500), "query": text(2000),
			"date_expression": expr(), "time_expression": expr(), "relative_time_expression": expr(),
			"period_expression": expr(), "duration_expression": expr(), "recurrence_expression": expr(),
			"reminder_expression": expr(), "reminder_time_expression": expr(),
			"notification_context": map[string]any{"type": "boolean"},
			"clarification":        text(500),
			"target": map[string]any{
				"type": []string{"object", "null"}, "additionalProperties": false,
				"required":   optional(targetProperties),
				"properties": targetProperties,
			},
			"changes": map[string]any{
				"type": []string{"object", "null"}, "additionalProperties": false,
				"required":   optional(changesProperties),
				"properties": changesProperties,
			},
		},
	}
}

// DecodeSemantic applies the same strict shape on live AND cached responses.
// In particular null/missing required objects, duplicate keys, IDs and trailing
// JSON must not bypass the schema merely because a provider constrained generation.
func DecodeSemantic(raw []byte) (SemanticCommand, error) {
	var result SemanticCommand
	if err := validateJSONShape(raw, SemanticSchema()); err != nil {
		return result, err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&result); err != nil {
		return result, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return result, errors.New("trailing semantic JSON")
	}
	return result, result.Validate()
}

func validateJSONShape(raw []byte, schema map[string]any) error {
	// Token walking catches duplicate keys that encoding/json would overwrite.
	d := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		t, err := d.Token()
		if err != nil {
			return err
		}
		if t == json.Delim('{') {
			keys := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return err
				}
				k, ok := key.(string)
				if !ok || keys[k] {
					return errors.New("duplicate semantic JSON key")
				}
				keys[k] = true
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = d.Token()
			return err
		}
		if t == json.Delim('[') {
			return errors.New("arrays are not semantic commands")
		}
		return nil
	}
	if err := walk(); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing semantic JSON")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return errors.New("semantic JSON must be an object")
	}
	var check func(map[string]json.RawMessage, map[string]any) error
	check = func(obj map[string]json.RawMessage, s map[string]any) error {
		for _, k := range s["required"].([]string) {
			if _, ok := obj[k]; !ok {
				return fmt.Errorf("missing semantic field %s", k)
			}
		}
		properties := s["properties"].(map[string]any)
		for k, v := range obj {
			field, ok := properties[k]
			if !ok {
				return fmt.Errorf("unknown semantic field %s", k)
			}
			if k == "all" || k == "notification_context" {
				if string(v) != "true" && string(v) != "false" {
					return fmt.Errorf("%s must be boolean", k)
				}
			}
			sub := field.(map[string]any)
			if _, nested := sub["properties"]; nested && string(v) != "null" {
				var child map[string]json.RawMessage
				if err := json.Unmarshal(v, &child); err != nil || child == nil {
					return fmt.Errorf("%s must be object or null", k)
				}
				if err := check(child, sub); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return check(object, schema)
}

func semanticString(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func (s SemanticCommand) Validate() error {
	if s.Status != "ok" && s.Status != "ambiguous" && s.Status != "unknown" {
		return errors.New("invalid semantic status")
	}
	for p, max := range map[*string]int{
		s.Title: 500, s.Query: 2000, s.DateExpression: 300, s.TimeExpression: 300,
		s.RelativeTimeExpression: 300, s.PeriodExpression: 300, s.DurationExpression: 300,
		s.RecurrenceExpression: 300, s.ReminderExpression: 300, s.ReminderTimeExpression: 300, s.Clarification: 500,
	} {
		if p != nil && (semanticString(p) == "" || utf8.RuneCountInString(*p) > max) {
			return errors.New("empty or overlong semantic string")
		}
	}
	if s.Action != nil && !full(`create|list|search|update|complete|delete|help`, *s.Action) {
		return errors.New("invalid semantic action")
	}
	if s.Kind != nil && !full(`task|meeting|reminder`, *s.Kind) {
		return errors.New("invalid semantic kind")
	}
	if s.Target != nil {
		for _, p := range []*string{s.Target.Query, s.Target.DateExpression, s.Target.TimeExpression, s.Target.PeriodExpression} {
			max := 300
			if p == s.Target.Query {
				max = 2000
			}
			if p != nil && (semanticString(p) == "" || utf8.RuneCountInString(*p) > max) {
				return errors.New("invalid target string")
			}
		}
	}
	if s.Changes != nil {
		for _, p := range []*string{s.Changes.Title, s.Changes.DateExpression, s.Changes.TimeExpression, s.Changes.RelativeTimeExpression} {
			max := 300
			if p == s.Changes.Title {
				max = 500
			}
			if p != nil && (semanticString(p) == "" || utf8.RuneCountInString(*p) > max) {
				return errors.New("invalid changes string")
			}
		}
	}
	if s.Status != "ok" {
		if s.Action != nil || s.Kind != nil || s.Title != nil || s.Query != nil || s.Target != nil || s.Changes != nil || s.DateExpression != nil || s.TimeExpression != nil || s.RelativeTimeExpression != nil || s.PeriodExpression != nil || s.RecurrenceExpression != nil || s.DurationExpression != nil || s.ReminderExpression != nil || s.ReminderTimeExpression != nil || s.NotificationContext {
			return errors.New("non-ok semantic output contains command data")
		}
		return nil
	}
	action := semanticString(s.Action)
	if action == "" {
		return errors.New("ok semantic command needs action")
	}
	mutation := action == "update" || action == "delete" || action == "complete"
	if mutation {
		if s.Target == nil || s.Title != nil || s.Query != nil || s.DateExpression != nil || s.TimeExpression != nil || s.RelativeTimeExpression != nil || s.PeriodExpression != nil || s.RecurrenceExpression != nil || s.DurationExpression != nil || s.ReminderExpression != nil || s.ReminderTimeExpression != nil || s.NotificationContext {
			return errors.New("mutation must separate target and changes")
		}
	} else if s.Target != nil || s.Changes != nil {
		return errors.New("target/changes only belong to mutations")
	}
	if action == "update" {
		if s.Changes == nil || (s.Changes.Title == nil && s.Changes.DateExpression == nil && s.Changes.TimeExpression == nil && s.Changes.RelativeTimeExpression == nil) || s.Target.All {
			return errors.New("update needs one target and nonempty changes")
		}
		if s.Changes.Title != nil && strings.EqualFold(semanticString(s.Changes.Title), semanticString(s.Target.Query)) {
			return errors.New("rename target and new title are identical")
		}
	} else if s.Changes != nil {
		return errors.New("changes only belong to update")
	}
	if mutation && s.Target.All && action != "delete" {
		return errors.New("bulk action is supported only for delete")
	}
	if action == "create" && (s.Kind == nil || s.Query != nil) {
		return errors.New("create needs kind and no query")
	}
	if action != "create" && s.Title != nil {
		return errors.New("top-level title only belongs to create")
	}
	if action != "search" && s.Query != nil {
		return errors.New("top-level query only belongs to search")
	}
	if action != "create" && (s.DurationExpression != nil || s.RecurrenceExpression != nil || s.ReminderExpression != nil || s.ReminderTimeExpression != nil || s.NotificationContext) {
		return errors.New("creation settings on non-create command")
	}
	return nil
}

// MapSemantic never calls Parse/ParseCommand, nor an intent/title parser.
// Each temporal role is resolved independently by existing deterministic helpers.
func MapSemantic(s SemanticCommand, original string, now time.Time) (domain.Command, error) {
	c := domain.Command{Action: domain.ActionHelp, Provider: "semantic", Duration: time.Hour, Confidence: 1}
	if err := s.Validate(); err != nil {
		return c, err
	}
	if s.Status != "ok" {
		c.Error = "Не распознал команду планировщику. Уточните запрос. Ничего не изменено."
		if s.Status == "ambiguous" {
			c.Error = semanticString(s.Clarification)
			if c.Error == "" {
				c.Error = "Уточните действие, название и время. Ничего не изменено."
			}
		}
		return c, nil
	}
	actions := map[string]domain.Action{"create": domain.ActionCreate, "list": domain.ActionList, "search": domain.ActionSearch, "update": domain.ActionUpdate, "complete": domain.ActionComplete, "delete": domain.ActionDelete, "help": domain.ActionHelp}
	c.Action = actions[semanticString(s.Action)]
	c.Kind, _ = domain.ParseEventType(semanticString(s.Kind))
	c.Title, c.Query = semanticString(s.Title), semanticString(s.Query)
	var err error
	apply := func(date, clock, relative, period *string) {
		if err != nil {
			return
		}
		var t Temporal
		t, err = semanticTemporal(date, clock, relative, period, original, now)
		if err == nil {
			c.Date, c.Time, c.StartSecond = t.Date, t.Time, t.StartSecond
			c.PeriodStart, c.PeriodEnd, c.Error = t.Start, t.End, t.Error
			if t.Relative {
				c.ExactAt = t.At
			}
		}
	}
	apply(s.DateExpression, s.TimeExpression, s.RelativeTimeExpression, s.PeriodExpression)
	if s.Target != nil {
		t := s.Target
		c.Query = semanticString(t.Query)
		var temporal Temporal
		temporal, err = semanticTemporal(t.DateExpression, t.TimeExpression, nil, t.PeriodExpression, original, now)
		if err == nil {
			c.Selector = &domain.EventSelector{All: t.All, Start: temporal.Start, End: temporal.End, Time: temporal.Time}
			if temporal.Error != "" {
				c.Error = temporal.Error
			}
			if c.Action != domain.ActionUpdate {
				c.Date, c.Time, c.PeriodStart, c.PeriodEnd = temporal.Date, temporal.Time, temporal.Start, temporal.End
			}
		}
		// IDs are deliberately absent from the model schema.
		if ids := matches(`(?:#|№\s*|номер\s*)(\d+)`, NormalizeText(original).Analysis); len(ids) > 0 {
			if len(ids) != 1 {
				return c, errors.New("multiple explicit target IDs")
			}
			id, parseErr := parseSemanticID(ids[0].Parts[1])
			if parseErr != nil {
				return c, parseErr
			}
			c.TargetID = &id
			if c.Query == ids[0].Parts[1] || c.Query == "#"+ids[0].Parts[1] || c.Query == "№"+ids[0].Parts[1] {
				c.Query = ""
			}
		}
		if c.Query == "" && c.TargetID == nil && !t.All && temporal.Start == nil && temporal.Time == nil && c.Kind == domain.EventUnknown {
			return c, errors.New("mutation target is missing")
		}
		if (c.Action == domain.ActionUpdate || c.Action == domain.ActionComplete) && c.Query == "" && c.TargetID == nil {
			return c, errors.New("update/complete need an explicit target query or ID")
		}
		if s.Changes != nil && err == nil {
			oldError := c.Error
			c.Title = semanticString(s.Changes.Title)
			apply(s.Changes.DateExpression, s.Changes.TimeExpression, s.Changes.RelativeTimeExpression, nil)
			if oldError != "" {
				c.Error = oldError
			}
		}
	}
	if err != nil {
		return c, err
	}
	if s.DurationExpression != nil {
		if !groundedExpression(*s.DurationExpression, original) {
			return c, errors.New("ungrounded duration expression")
		}
		raw := semanticString(s.DurationExpression)
		if !has(`(?i)^(?:на|длительностью)\b`, raw) {
			raw = "на " + raw
		}
		t := Extract(raw, now)
		if t.Error != "" || t.Clean(raw) != "" {
			return c, errors.New("unsupported duration expression")
		}
		c.Duration = t.Duration
	}
	if s.RecurrenceExpression != nil {
		if !groundedExpression(*s.RecurrenceExpression, original) {
			return c, errors.New("ungrounded recurrence expression")
		}
		t := Extract(semanticString(s.RecurrenceExpression), now)
		if t.Recurrence == nil {
			return c, errors.New("unsupported recurrence expression")
		}
		c.Recurrence = t.Recurrence
		if t.Error != "" {
			c.Error = t.Error
		}
		if c.Kind == domain.EventMeeting {
			c.Error = "Повторение поддерживается для задач и напоминаний"
		}
		ResolveRecurrence(&c, now)
	}
	if s.ReminderExpression != nil {
		if !groundedExpression(*s.ReminderExpression, original) {
			return c, errors.New("ungrounded reminder expression")
		}
		minutes, enabled, recognized, question := ReminderChoice(*s.ReminderExpression)
		if !recognized {
			return c, errors.New("unsupported reminder expression")
		}
		c.ReminderMinutes, c.ReminderEnabled = minutes, enabled
		if question != "" {
			c.Error = question
		}
	}
	c.NotificationContext = s.NotificationContext
	if s.ReminderTimeExpression != nil {
		if s.ReminderExpression != nil || !groundedExpression(*s.ReminderTimeExpression, original) {
			return c, errors.New("conflicting or ungrounded notification time")
		}
		alarm := ParseDateTime(clockExpression(*s.ReminderTimeExpression), now)
		if alarm.Time == nil || alarm.Error != "" || c.Time == nil {
			return c, errors.New("notification time needs event time")
		}
		lead := (c.Time.Hour-alarm.Time.Hour)*60 + c.Time.Minute - alarm.Time.Minute
		if lead < 0 {
			c.Error = "Время уведомления должно быть не позже начала события."
		} else {
			enabled := true
			c.ReminderMinutes, c.ReminderEnabled = &lead, &enabled
		}
	}
	if c.Action == domain.ActionList || c.Action == domain.ActionSearch {
		c.Date, c.Time = nil, nil
	}
	if c.Action == domain.ActionUpdate && c.Time != nil && c.Date == nil {
		// Updating only the clock retains the selected record's existing date.
		c.ExactAt = nil
	}
	if c.Action == domain.ActionCreate && c.Error == "" {
		c.Error = validateSemanticCreationSource(original, now)
	}
	return c, c.Validate()
}
