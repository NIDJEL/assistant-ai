package nlp

import "strings"

// ExtractExpectedTemporalSlot is deliberately limited to a known pending slot.
// It returns a grounded span; ApplyDialogueSlots still parses the entire span
// with the Go temporal grammar. No command/title text is rewritten here.
func ExtractExpectedTemporalSlot(message, expectedSlot string) (string, bool) {
	if !full(`date|time|relative_time|date_or_time|relative_time_or_date|weekday`, expectedSlot) || len([]rune(message)) > 500 || strings.ContainsAny(message, `«»"`) {
		return "", false
	}
	raw := strings.TrimSpace(message)
	trim := func(s string) string { return strings.Trim(s, " \t\r\n,.!?;:—–") }
	raw = trim(raw)
	// Only safe boundary discourse is ignored. Negation, alternatives and new
	// directives cannot be discarded, even when there is a valid time inside.
	for {
		next := replace(`(?i)^(?:а|ну|давай(?:-ка)?|пожалуй|хорошо|ладно|тогда|окей|да|пожалуйста|значит|наверное|можно|все-таки)(?:[\s,;:—–]+|$)`, raw, "")
		next = trim(next)
		if next == raw {
			break
		}
		raw = next
	}
	for {
		next := replace(`(?i)(?:[\s,;:—–]+)(?:пожалуйста|пожалуй|наверное|тогда|хорошо|ладно|окей)$`, raw, "")
		next = trim(next)
		if next == raw {
			break
		}
		raw = next
	}
	low := temporalAlias(raw)
	relative := has(`^через\s+`, low)
	date := has(`\b(?:сегодня|завтра|послезавтра|понедельник|вторник|среда|четверг|пятница|суббота|воскресенье|\d{4}-\d{2}-\d{2}|\d{1,2}\.\d{1,2}|`+strings.Join(monthNames, "|")+`)\b`, low)
	clock := has(`\b(?:в|к|на|около)\s+|\d{1,2}:\d{2}|\b(?:утром|днем|вечером|ночью|полдень|полночь)\b`, low) || full(numberPattern, low)
	switch expectedSlot {
	case "relative_time":
		return raw, relative
	case "date", "weekday":
		return raw, date || relative
	case "time":
		return raw, clock || relative
	default:
		return raw, date || clock || relative
	}
}
