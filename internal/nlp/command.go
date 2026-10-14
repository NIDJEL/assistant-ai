package nlp

import (
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

type Intent string

const (
	CreateReminder Intent = "create_reminder"
	CreateTask     Intent = "create_task"
	CreateEvent    Intent = "create_event"
	Reschedule     Intent = "reschedule"
	CompleteTask   Intent = "complete_task"
	DeleteTask     Intent = "delete_task"
	RestoreTask    Intent = "restore_task"
	QuerySchedule  Intent = "query_schedule"
	Unknown        Intent = "unknown"
)

type Entity string

const (
	ActionText          Entity = "text"
	DateEntity          Entity = "date"
	TimeEntity          Entity = "time"
	DayEntity           Entity = "day_in_period"
	RecurrenceDayEntity Entity = "recurrence_day"
)

// Original is preserved for display; Text and Analysis have identical byte
// offsets for Russian casing and е/ё, so entity spans can be removed from Text.
type NormalizedText struct{ Original, Text, Analysis string }

func NormalizeText(text string) NormalizedText {
	n := NormalizedText{Original: text, Text: spaces(text)}
	n.Analysis = strings.ReplaceAll(strings.ToLower(n.Text), "ё", "е")
	return n
}

type ParsedCommand struct {
	domain.Command
	Intent   Intent
	Original string
	Missing  []Entity
}

func (c ParsedCommand) NeedsClarification() bool {
	return c.Intent == Unknown || c.Error != "" || len(c.Missing) != 0
}

func commandIntent(c domain.Command) Intent {
	switch c.Action {
	case domain.ActionCreate:
		switch c.Kind {
		case domain.EventReminder:
			return CreateReminder
		case domain.EventMeeting:
			return CreateEvent
		default:
			return CreateTask
		}
	case domain.ActionUpdate:
		return Reschedule
	case domain.ActionComplete:
		return CompleteTask
	case domain.ActionDelete:
		return DeleteTask
	case domain.ActionRestore:
		return RestoreTask
	case domain.ActionList, domain.ActionSearch:
		return QuerySchedule
	default:
		return Unknown
	}
}

// Recomputed after merging a reply with a persisted command. Optional reminder
// settings never count as a missing date or time.
func MissingEntities(c domain.Command) []Entity {
	var missing []Entity
	if c.Action != domain.ActionCreate {
		return missing
	}
	if !usefulTitle(c.Title) || full(`(?i)встреча|задача|напоминание`, c.Title) {
		missing = append(missing, ActionText)
	}
	if c.Recurrence != nil && c.Recurrence.Frequency == "week" && len(c.Recurrence.Weekdays) == 0 {
		missing = append(missing, RecurrenceDayEntity)
	}
	if c.Recurrence == nil && c.Date == nil && c.PeriodStart != nil {
		missing = append(missing, DayEntity)
	} else if c.Recurrence == nil && c.Date == nil && (c.Kind == domain.EventReminder || c.Kind == domain.EventMeeting || c.Time != nil) {
		missing = append(missing, DateEntity)
	}
	if c.Time == nil && (c.Kind == domain.EventReminder || c.Kind == domain.EventMeeting || c.Recurrence != nil) {
		missing = append(missing, TimeEntity)
	}
	return missing
}

func ParseCommand(text string, now time.Time) ParsedCommand {
	n := NormalizeText(text)
	c := parseCommand(n.Text, now)
	return ParsedCommand{Command: c, Intent: commandIntent(c), Original: n.Original, Missing: MissingEntities(c)}
}

const reminderDirective = `(?:напомни(?:ть)?|не\s+(?:дай\s+забыть|забудь)|(?:нужно|надо)\s+не\s+забыть|скажи\s+мне)`
const reminderCue = `\b(?:` + reminderDirective + `|напоминан\w*|напоминалк\w*|уведомлен\w*|оповещен\w*)\b`
const createVerb = `(?:поставь|поставить|добавь|добавить|создай|создать|запиши|записать|запланируй\w*|назначь|назначить|установи)`

// Explicit creation of a task/event owns its following notification settings.
// Otherwise a reminder cue outranks incidental subject words such as "встречу".
func DetectIntent(text string) (Intent, domain.Action, domain.EventType, bool) {
	low := NormalizeText(scheduleOnly(normalizeConversation(text))).Analysis
	action, negated := actionFromSpeech(low)
	kind := inferKind(low)
	if !negated && action == domain.ActionHelp && has(reminderCue, low) {
		action = domain.ActionCreate
	}
	if action == domain.ActionCreate {
		if m := first(`\b`+createVerb+`[\s,]+(?:(?:пожалуйста|мне|новую|новое|новый)[\s,]+)*(задач\w*|дело|встреч[ауи]|событи\w*|мероприят\w*)\b`, low); m != nil {
			cue := first(reminderCue, low)
			if cue == nil || m.Start < cue.Start {
				kind = domain.EventTask
				if has(`встреч|событи|мероприят`, m.Parts[1]) {
					kind = domain.EventMeeting
				}
			}
		}
		if kind == domain.EventUnknown {
			kind = domain.EventTask
		}
	}
	return commandIntent(domain.Command{Action: action, Kind: kind}), action, kind, negated
}
