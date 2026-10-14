package nlp

import (
	"strings"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

const createDirective = `\b(?:` + reminderDirective + `|` + createVerb + `)\b`

// Repair only the planner construction. "Оставь посылку у двери" is content,
// not a misspelled instruction to create an event.
func normalizeConversation(text string) string {
	text = trimCourtesy(text)
	text = replace(`(?i)^гена[\s,.:;-]+`, text, "")
	ms := matches(`(?i)\bоставь([\s,]+(?:(?:пожалуйста|мне)[\s,]+)*(?:напоминание|уведомление|задачу|событие|встречу|в\s+(?:(?:мой|наш)\s+)?календарь)\b)`, text)
	for i := len(ms) - 1; i >= 0; i-- {
		m := ms[i]
		if has(`(?i)\b(?:без\s+изменений|как\s+есть|на\s+месте|в\s+покое)\b`, text[m.End:]) {
			continue
		}
		text = text[:m.Start] + "поставь" + m.Parts[1] + text[m.End:]
	}
	return text
}

func actionFromSpeech(text string) (domain.Action, bool) {
	type directive struct {
		pattern string
		action  domain.Action
	}
	patterns := []directive{
		{createDirective, domain.ActionCreate},
		{`\b(?:восстанови(?:ть)?|верни|вернуть)\b`, domain.ActionRestore},
		{`\b(?:удали\w*|удалить|отмени\w*|убери\w*)\b`, domain.ActionDelete},
		{`\b(?:выполнен\w*|выполнил\w*|сделан\w*|заверши\w*|закончи\w*|отметь)\b`, domain.ActionComplete},
		{`\b(?:перенеси\w*|перенести|перенос\w*|сдвинь|измени\w*)\b`, domain.ActionUpdate},
		{`\b(?:найди|найти|отыщи|поищи|поиск|когда|раньше|когда-то|была|были|истори\w*)\b`, domain.ActionSearch},
		{`\b(?:что|какие|покажи|показать|список|планы|запланировано|есть\s+ли|расписание)\b`, domain.ActionList},
	}
	position := len(text) + 1
	action := domain.ActionHelp
	for _, d := range patterns {
		if m := first(d.pattern, text); m != nil && m.Start < position {
			position, action = m.Start, d.action
		}
	}
	// Question words and infinitives may be the reminder's action text, even
	// when "напомни" follows that content.
	if m := first(createDirective, text); m != nil && action != domain.ActionCreate {
		prefix := text[:m.Start]
		if !has(`\b(?:удали\w*|отмени\w*|убери\w*|перенеси\w*|перенести|сдвинь|измени\w*|восстанови\w*|верни|заверши\w*|закончи\w*|отметь|найди|найти|отыщи|поищи|покажи|показать)\b`, prefix) {
			position, action = m.Start, domain.ActionCreate
		}
	}
	if m := first(`\bне\s+(?:добавляй|создавай|ставь|записывай|напоминай|удаляй|отменяй|переноси|меняй)\b`, text); m != nil && m.Start < position {
		return domain.ActionHelp, true
	}
	if m := first(`\bоставь\b`, text); m != nil && m.Start < position {
		return domain.ActionHelp, true
	}
	if position > len(text) {
		return action, false
	}
	// A negated request must never become a mutation or reach the LLM fallback.
	prefix := strings.TrimRight(text[:position], " ,.!?:;-")
	if full(`(?:.*\s)?не`, prefix) || has(`\bне\s+(?:надо|нужно)\b`, prefix) {
		return domain.ActionHelp, true
	}
	return action, false
}

func createTitle(text string) string {
	// A request may follow a spoken explanation. Prefer its content, but retain
	// the explanation when the request has no content of its own.
	if m := first(`(?i)`+createDirective, text); m != nil {
		before, after := text[:m.Start], text[m.End:]
		after = replace(`(?i)^[\s,.:;-]*(?:(?:пожалуйста|мне)[\s,.:;-]*)*`, after, "")
		after = replace(`(?i)^в\s+(?:(?:мой|наш)\s+)?календарь\b[\s,.:;-]*`, after, "")
		after = replace(`(?i)^(?:(?:новую|новое|новый)\s+)?(?:задачу|напоминание|напоминалку|уведомление|событие|мероприятие|дело)\b[\s,.:;-]*`, after, "")
		after = replace(`(?i)^в\s+(?:(?:мой|наш)\s+)?календарь\b[\s,.:;-]*`, after, "")
		after = replace(`(?i)^(?:(?:о\s+том|про\s+то)[\s,]+)?(?:что|чтобы)\b[\s,.:;-]*`, after, "")
		if strings.Trim(after, " ,.!?:;-") != "" {
			text = after
		} else {
			text = before
		}
	}
	text = replace(`(?i)^(?:(?:можешь|можете|мог\s+бы|могли\s+бы|давай|слушай|гена)[\s,.:;-]+)+`, text, "")
	// Only introductory words are removed. "объяснить мне что нужно для
	// визы" must retain its inner words.
	for {
		next := replace(`(?i)^[\s,.:;-]*(?:и|что|чтобы|мне|у\s+меня|я|нужно|надо|напоминание|напоминалку)\b[\s,.:;-]*`, text, "")
		if next == text {
			break
		}
		text = next
	}
	text = trimCourtesy(text)
	text = replace(`(?i)\b(?:до|на|в|во|к)\s*$`, text, "")
	return strings.Trim(spaces(text), " ,.!?:;-")
}
