package nlp

import (
	"context"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

const meetingPattern = `\b(?:встреч[ауи]|созвон\w*|совещан\w*|демо\b|интервью\b)`

func inferKind(text string) domain.EventType {
	if has(reminderCue, text) {
		return domain.EventReminder
	}
	if has(`\bзадач\w*\s+и\s+встреч\w*`, text) {
		return domain.EventUnknown
	}
	if has(`\b(?:задач\w*|дело)\b`, text) {
		return domain.EventTask
	}
	if has(meetingPattern, text) || has(`\b(?:событи\w*|мероприят\w*)\b`, text) {
		return domain.EventMeeting
	}
	return domain.EventUnknown
}

type Parser struct{}

func (Parser) Interpret(ctx context.Context, text string, now time.Time) (domain.Command, error) {
	if err := ctx.Err(); err != nil {
		return domain.Command{}, err
	}
	return Parse(text, now), nil
}
func Parse(text string, now time.Time) domain.Command {
	return ParseCommand(text, now).Command
}

func parseCommand(text string, now time.Time) domain.Command {
	text = normalizeConversation(text)
	low := strings.TrimSpace(strings.ReplaceAll(strings.ToLower(text), "ё", "е"))
	_, action, kind, negated := DetectIntent(text)
	t := Extract(low, now)
	cleaned := t.Clean(text)
	cmd := domain.Command{Action: action, Kind: kind, Date: t.Date, Time: t.Time, StartSecond: t.StartSecond, PeriodStart: t.Start, PeriodEnd: t.End, Duration: t.Duration, Error: t.Error, Provider: "rules", Confidence: .2}
	if target := first(`(?:#|номер\s*|№\s*)(\d+)`, low); target != nil {
		id, err := strconv.ParseInt(target.Parts[1], 10, 64)
		if err == nil && id > 0 {
			cmd.TargetID = &id
		} else {
			cmd.Error = "Некорректный номер записи."
		}
		cleaned = strings.TrimSpace(replace(`(?:#|номер\s*|№\s*)\d+`, cleaned, ""))
	}
	if negated {
		cmd.Error = "Понял: это действие выполнять не нужно. Сформулируйте, что нужно сделать."
	} else if cmd.Action == domain.ActionHelp && (has(`\b(?:нужно|надо|купить|подготовить|отправить|позвонить|проверить)\b`, low) || (kind == domain.EventMeeting && t.Date != nil)) {
		cmd.Action = domain.ActionCreate
	}
	if cmd.Action == domain.ActionCreate {
		if cmd.Kind == domain.EventUnknown {
			cmd.Kind = domain.EventTask
		}
		plan := creationPlan(text, now)
		cmd.Title, cmd.Date, cmd.Time = plan.Title, plan.Date, plan.Time
		cmd.PeriodStart, cmd.PeriodEnd, cmd.Duration = plan.PeriodStart, plan.PeriodEnd, plan.Duration
		cmd.ReminderMinutes, cmd.ReminderEnabled, cmd.Error = plan.ReminderMinutes, plan.ReminderEnabled, plan.Error
		cmd.StartSecond = plan.StartSecond
		cmd.NotificationContext = plan.NotificationContext
		cmd.ExactAt = plan.ExactAt
		cmd.Recurrence = plan.Recurrence
		if cmd.Recurrence != nil {
			if cmd.Kind == domain.EventMeeting {
				cmd.Error = "Повторение поддерживается для задач и напоминаний"
			}
			ResolveRecurrence(&cmd, now)
		}
		cmd.Title = replace(`(?i)^встречу\b`, cmd.Title, "Встреча")
		runes := []rune(cmd.Title)
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
			cmd.Title = string(runes)
		}
	} else {
		cleaned = replace(`(?i)\b(?:найди|найти|отыщи|поищи|поиск|когда-то|когда|раньше|была|был|были|ставил|ставила|я|у меня|мои|мою|мне|задач\w*|напоминан\w*|событи\w*|выполнен\w*|выполнил\w*|сделан\w*|заверши\w*|закончи\w*|отметь|как|удали\w*|удалить|отмени\w*|убери\w*|перенеси\w*|перенести|измени\w*|а|ее|его|их|это|эту|этот|на|про|по|пожалуйста)\b`, cleaned, " ")
		cmd.Query = strings.Trim(spaces(cleaned), " ,.!?:;-")
	}
	if cmd.Action == domain.ActionDelete || cmd.Action == domain.ActionUpdate || cmd.Action == domain.ActionComplete || cmd.Action == domain.ActionRestore {
		planSelection(&cmd, text, now)
	}
	fragment := strings.TrimSpace(t.Clean(low))
	switch fragment {
	case "в", "во", "на", "к", "до", "за":
		fragment = ""
	}
	pureTemporal := (t.Date != nil || t.Time != nil) && fragment == ""
	_, _, reminderAnswer, _ := ReminderChoice(low)
	if cmd.Action != domain.ActionHelp || cmd.Error != "" || pureTemporal || reminderAnswer || full(`#?\d+|да(?:[, ]+(?:пожалуйста|конечно|удали|сохрани|все равно))?|все равно|сохранить|нет|отмена|подтверждаю|не надо|стоп`, low) {
		cmd.Confidence = .95
	}
	return cmd
}

// Conversational wrappers are removed only at the edges. Words such as
// "привет" inside "передать привет маме" remain part of the actual task.
func trimCourtesy(text string) string {
	text = strings.Trim(strings.TrimSpace(text), " ,.!?:;-")
	for {
		next := replace(`(?i)^(?:привет|приветствую|здравствуй(?:те)?|доброе\s+утро|добрый\s+(?:день|вечер)|пожалуйста)\b[\s,.!?:;-]*`, text, "")
		if next == text {
			break
		}
		text = next
	}
	text = replace(`(?i)[\s,.!?:;-]+пожалуйста\b[\s,.!?:;-]*$`, text, "")
	return strings.Trim(text, " ,.!?:;-")
}
