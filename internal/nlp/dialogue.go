package nlp

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

// DialogueContext is an allowlist for the language boundary. It deliberately
// contains no record IDs, user profile, repository structs or credentials.
type DialogueContext struct {
	LastAffectedCount    int                     `json:"last_affected_count"`
	Filters              *domain.ExecutionFilter `json:"filters,omitempty"`
	LastResultCount      int                     `json:"last_result_count"`
	Pending              *DialoguePending        `json:"pending,omitempty"`
	LastAction           string                  `json:"last_action,omitempty"`
	LastKind             string                  `json:"last_kind,omitempty"`
	Query                string                  `json:"query,omitempty"`
	PeriodStart          string                  `json:"period_start,omitempty"`
	PeriodEnd            string                  `json:"period_end,omitempty"`
	Candidates           []string                `json:"candidates,omitempty"`
	LastUserMessage      string                  `json:"last_user_message,omitempty"`
	LastAssistantMessage string                  `json:"last_assistant_message,omitempty"`
}

type DialoguePending struct {
	ConfirmationState string `json:"confirmation_state,omitempty"`
	Action            string `json:"action"`
	Kind              string `json:"kind"`
	Title             string `json:"title,omitempty"`
	Date              string `json:"date,omitempty"`
	Time              string `json:"time,omitempty"`
	ExpectedSlot      string `json:"expected_slot"`
}

// A turn describes language, never an executable action or a database ID.
type ContextualTurn struct {
	Status        string             `json:"status"`
	Type          string             `json:"turn_type"`
	Command       *SemanticCommand   `json:"command"`
	Slots         *SlotUpdate        `json:"slot_update"`
	Reference     *DialogueReference `json:"reference"`
	Confirmation  *bool              `json:"confirmation"`
	Clarification *string            `json:"clarification"`
	ChatResponse  *string            `json:"chat_response"`
}

type SlotUpdate struct {
	Title              *string `json:"title"`
	TemporalExpression *string `json:"temporal_expression"`
	ReminderExpression *string `json:"reminder_expression"`
	PeriodExpression   *string `json:"period_expression"`
	Kind               *string `json:"kind"`
}

type DialogueReference struct {
	Relation string  `json:"relation"`
	Query    *string `json:"query"`
	Action   *string `json:"action"`
}

// Only this typed validation failure permits recovery from the grounded message.
// JSON shape errors, extra fields and inconsistent payloads remain rejected.
var ErrEmptySlotUpdate = errors.New("empty slot update")

func ContextualSchema() map[string]any {
	str := func(max int) map[string]any {
		return map[string]any{"type": []string{"string", "null"}, "minLength": 1, "maxLength": max}
	}
	object := func(props map[string]any) map[string]any {
		keys := []string{}
		for k := range props {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return map[string]any{"type": []string{"object", "null"}, "additionalProperties": false, "required": keys, "properties": props}
	}
	command := SemanticSchema()
	command["type"] = []string{"object", "null"}
	slots := object(map[string]any{"title": str(500), "temporal_expression": str(300), "reminder_expression": str(300), "period_expression": str(300), "kind": map[string]any{"enum": []any{"task", "meeting", "reminder", nil}}})
	ref := object(map[string]any{"relation": map[string]any{"enum": []string{"last", "first", "second", "third", "fourth", "fifth", "previous", "matching"}}, "query": str(500), "action": map[string]any{"enum": []any{"update", "delete", "complete", nil}}})
	return map[string]any{"type": "object", "additionalProperties": false,
		"required": []string{"status", "turn_type", "command", "slot_update", "reference", "confirmation", "clarification", "chat_response"},
		"properties": map[string]any{
			"status":    map[string]any{"enum": []string{"ok", "ambiguous", "unknown"}},
			"turn_type": map[string]any{"enum": []string{"command", "slot_reply", "reference", "confirmation", "cancellation", "choice", "refinement", "chat"}},
			"command":   command, "slot_update": slots, "reference": ref,
			"confirmation": map[string]any{"type": []string{"boolean", "null"}}, "clarification": str(500), "chat_response": str(500),
		}}
}

func DecodeContextualTurn(raw []byte) (ContextualTurn, error) {
	var t ContextualTurn
	if err := validateJSONShape(raw, ContextualSchema()); err != nil {
		return t, err
	}
	if err := json.Unmarshal(raw, &t); err != nil {
		return t, err
	}
	return t, t.Validate()
}

func (t ContextualTurn) Validate() error {
	if !full(`ok|ambiguous|unknown`, t.Status) || !full(`command|slot_reply|reference|confirmation|cancellation|choice|refinement|chat`, t.Type) {
		return errors.New("invalid dialogue classification")
	}
	for _, p := range []*string{t.Clarification, t.ChatResponse} {
		if p != nil && (strings.TrimSpace(*p) == "" || utf8.RuneCountInString(*p) > 500) {
			return errors.New("invalid dialogue text")
		}
	}
	if t.Status != "ok" {
		if t.Command != nil || t.Slots != nil || t.Reference != nil || t.Confirmation != nil || t.ChatResponse != nil {
			return errors.New("non-ok dialogue contains action data")
		}
		return nil
	}
	if t.Slots != nil {
		u := t.Slots
		if u.Title == nil && u.TemporalExpression == nil && u.ReminderExpression == nil && u.PeriodExpression == nil && u.Kind == nil {
			if t.Type == "slot_reply" && t.Command == nil && t.Reference == nil && t.Confirmation == nil && t.ChatResponse == nil {
				return ErrEmptySlotUpdate
			}
			return errors.New("empty slot update")
		}
		for _, p := range []*string{u.Title, u.TemporalExpression, u.ReminderExpression, u.PeriodExpression, u.Kind} {
			limit := 300
			if p == u.Title {
				limit = 500
			}
			if p != nil && (strings.TrimSpace(*p) == "" || utf8.RuneCountInString(*p) > limit) {
				return errors.New("invalid slot text")
			}
		}
		if u.Kind != nil && !full(`task|meeting|reminder`, *u.Kind) {
			return errors.New("invalid slot kind")
		}
	}
	if t.Reference != nil {
		if t.Reference.Action != nil && !full(`update|delete|complete`, *t.Reference.Action) {
			return errors.New("invalid reference action")
		}
		if !full(`last|first|second|third|fourth|fifth|previous|matching`, t.Reference.Relation) {
			return errors.New("invalid reference")
		}
		if (t.Reference.Relation == "matching") != (t.Reference.Query != nil) {
			return errors.New("matching needs query only")
		}
		if t.Reference.Query != nil && (strings.TrimSpace(*t.Reference.Query) == "" || utf8.RuneCountInString(*t.Reference.Query) > 500) {
			return errors.New("invalid reference query")
		}
	}
	if t.Command != nil {
		c := *t.Command
		if c.Status != "ok" {
			return errors.New("non-ok command in ok dialogue")
		}
		if err := c.Validate(); err != nil {
			return err
		}
	}
	valid := false
	if t.Type == "slot_reply" && t.Slots == nil && t.Command == nil && t.Reference == nil && t.Confirmation == nil && t.ChatResponse == nil {
		return ErrEmptySlotUpdate
	}
	switch t.Type {
	case "command":
		valid = t.Command != nil && t.Slots == nil && t.Reference == nil
	case "slot_reply", "refinement":
		valid = t.Slots != nil && t.Command == nil && t.Reference == nil
	case "reference":
		valid = t.Reference != nil && t.Command == nil && t.Reference.Action != nil && ((*t.Reference.Action == "update") == (t.Slots != nil))
	case "choice":
		valid = t.Reference != nil && t.Reference.Action == nil && t.Command == nil && t.Slots == nil
	case "confirmation":
		valid = t.Confirmation != nil && t.Command == nil && t.Slots == nil && t.Reference == nil
	case "cancellation", "chat":
		valid = t.Command == nil && t.Slots == nil && t.Reference == nil
	}
	if !valid || t.Type != "confirmation" && t.Confirmation != nil || t.Type != "chat" && t.ChatResponse != nil {
		return errors.New("inconsistent dialogue payload")
	}
	return nil
}

// ApplyDialogueSlots only resolves supported fields with the Go grammars.
// Expressions must be grounded in this message, not a model-invented date.
func ApplyDialogueSlots(c domain.Command, u SlotUpdate, message string, now time.Time) (domain.Command, error) {
	if u.TemporalExpression != nil {
		raw := *u.TemporalExpression
		if !groundedExpression(raw, message) {
			return c, errors.New("ungrounded dialogue time")
		}
		raw = temporalAlias(raw)
		raw = regex(`^на\s+(\d{1,2}(?::\d{2})?)$`).ReplaceAllString(raw, "в $1")
		t := ParseDateTimeReply(raw, now, c)
		if t.Error != "" {
			return c, errors.New(t.Error)
		}
		if t.Date == nil && t.Time == nil && t.Recurrence == nil || strings.Trim(t.Clean(raw), " ,.!?:;-") != "" {
			return c, errors.New("unsupported dialogue time")
		}
		if t.Date != nil {
			c.Date = t.Date
		}
		if t.Time != nil {
			c.Time, c.StartSecond = t.Time, t.StartSecond
		}
		if t.Recurrence != nil {
			c.Recurrence = t.Recurrence
		}
		c.ExactAt = nil
		if t.Relative {
			c.ExactAt = t.At
		}
	}
	if u.ReminderExpression != nil {
		if !groundedExpression(*u.ReminderExpression, message) {
			return c, errors.New("ungrounded dialogue reminder")
		}
		m, e, ok, question := ReminderChoice(*u.ReminderExpression)
		if !ok || question != "" {
			return c, errors.New("unsupported dialogue reminder")
		}
		c.ReminderMinutes, c.ReminderEnabled = m, e
	}
	if u.PeriodExpression != nil {
		t, err := semanticTemporal(nil, nil, nil, u.PeriodExpression, message, now)
		if err != nil {
			return c, err
		}
		if t.Error != "" {
			return c, errors.New(t.Error)
		}
		c.PeriodStart, c.PeriodEnd = t.Start, t.End
	}
	if u.Title != nil {
		c.Title = strings.TrimSpace(*u.Title)
	}
	if u.Kind != nil {
		c.Kind, _ = domain.ParseEventType(*u.Kind)
	}
	ResolveRecurrence(&c, now)
	return c, c.Validate()
}
