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
	if has(`\b(?:напомни\w*|напоминан\w*)`, text) {
		return domain.EventReminder
	}
	if has(`\bзадач\w*\s+и\s+встреч\w*`, text) {
		return domain.EventUnknown
	}
	if has(`\bзадач\w*`, text) {
		return domain.EventTask
	}
	if has(meetingPattern, text) {
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
	low := strings.TrimSpace(strings.ReplaceAll(strings.ToLower(text), "ё", "е"))
	t := Extract(low, now)
	cleaned := t.Clean(text)
	kind := inferKind(low)
	cmd := domain.Command{Action: domain.ActionHelp, Kind: kind, Date: t.Date, Time: t.Time, PeriodStart: t.Start, PeriodEnd: t.End, Duration: t.Duration, Error: t.Error, Provider: "rules", Confidence: .2}
	if target := first(`(?:#|номер\s*|№\s*)(\d+)`, low); target != nil {
		id, err := strconv.ParseInt(target.Parts[1], 10, 64)
		if err == nil && id > 0 {
			cmd.TargetID = &id
		} else {
			cmd.Error = "Некорректный номер записи."
		}
		cleaned = strings.TrimSpace(replace(`(?:#|номер\s*|№\s*)\d+`, cleaned, ""))
	}
	switch {
	case has(`\b(?:удали\w*|удалить|отмени\w*|убери\w*)`, low):
		cmd.Action = domain.ActionDelete
	case has(`\b(?:выполнен\w*|выполнил\w*|сделан\w*|заверши\w*|закончи\w*)`, low):
		cmd.Action = domain.ActionComplete
	case has(`\b(?:перенеси\w*|перенести|перенос\w*|сдвинь|измени\w*)`, low):
		cmd.Action = domain.ActionUpdate
	case has(`\b(?:найди|найти|отыщи|поищи|поиск|когда|раньше|когда-то|была|были|истори\w*)`, low):
		cmd.Action = domain.ActionSearch
	case has(`\b(?:что|какие|покажи|показать|список|планы|запланировано|есть\s+ли|расписание)\b`, low):
		cmd.Action = domain.ActionList
	case has(`\b(?:напомни\w*|поставь|поставить|добавь|добавить|создай|создать|запиши|записать|нужно|надо|купить|подготовить|отправить|позвонить|проверить|запланируй\w*)\b`, low) || (kind == domain.EventMeeting && t.Date != nil):
		cmd.Action = domain.ActionCreate
	}
	if cmd.Action == domain.ActionCreate {
		if kind == domain.EventUnknown {
			cmd.Kind = domain.EventTask
		}
		cleaned = replace(`(?i)\b(?:напомни\w*|поставь|поставить|добавь|добавить|создай|создать|запиши|записать|запланируй\w*)\s*(?:мне\s*)?(?:(?:новую?\s+)?(?:задачу|напоминание)\s*)?(?:чтобы\s*)?`, cleaned, "")
		cleaned = replace(`(?i)\b(?:мне|у меня|нужно|надо)\b`, cleaned, "")
		cleaned = replace(`(?i)\b(?:до|на|в|во|к)\s*$`, cleaned, "")
		cmd.Title = strings.Trim(spaces(cleaned), " ,.!?:;-")
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
	fragment := strings.TrimSpace(t.Clean(low))
	switch fragment {
	case "в", "во", "на", "к", "до", "за":
		fragment = ""
	}
	pureTemporal := (t.Date != nil || t.Time != nil) && fragment == ""
	if cmd.Action != domain.ActionHelp || cmd.Error != "" || pureTemporal || full(`#?\d+|да(?:[, ]+(?:пожалуйста|конечно|удали|сохрани|все равно))?|все равно|сохранить|нет|отмена|подтверждаю|не надо|стоп`, low) {
		cmd.Confidence = .95
	}
	return cmd
}
