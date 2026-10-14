package nlp

import (
	"strconv"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

const reminderAmount = `(?:\d+(?:[.,]\d{1,2})?|полтора|полторы|(?:двадцать|тридцать|сорок|пятьдесят|шестьдесят|семьдесят|восемьдесят|девяносто)(?:\s+(?:один|одну|два|две|три|четыре|пять|шесть|семь|восемь|девять))?|` + numberPattern + `)`
const leadPattern = `\bза\s+(?:полчаса|(?:(` + reminderAmount + `)\s*)?(час(?:а|ов)?|ч|минут(?:у|ы)?|мин|день|дня|дней|сутки|суток)(?:\s*(?:и\s+)?(` + reminderAmount + `)\s*(?:минут(?:у|ы)?|мин))?)\b`
const noReminderPattern = `\b(?:без\s+(?:напоминан\w*|уведомлен\w*)|не\s+(?:надо|нужно)\s+(?:напоминать|уведомлять)|(?:напоминан\w*|уведомлен\w*)\s+не\s+нужно)\b`
const onTimePattern = `\b(?:в\s+(?:самый\s+)?момент\s+(?:начала|события)|к\s+началу|без\s+опережения)\b`

func reminderNumber(text string) float64 {
	if n, err := strconv.ParseFloat(strings.ReplaceAll(text, ",", "."), 64); err == nil {
		return n
	}
	if text == "полтора" || text == "полторы" {
		return 1.5
	}
	n := number(text)
	for _, part := range strings.Fields(text) {
		switch part {
		case "тридцать":
			n += 30
		case "сорок":
			n += 40
		case "пятьдесят":
			n += 50
		case "шестьдесят":
			n += 60
		case "семьдесят":
			n += 70
		case "восемьдесят":
			n += 80
		case "девяносто":
			n += 90
		}
	}
	return n
}

func reminderLead(text string) (*int, *bool, string) {
	if has(noReminderPattern, text) {
		enabled := false
		return nil, &enabled, ""
	}
	if has(onTimePattern, text) {
		n, enabled := 0, true
		return &n, &enabled, ""
	}
	var result *int
	for _, m := range matches(leadPattern, text) {
		n := 30.0
		if m.Parts[0] != "за полчаса" {
			n = 1
			if m.Parts[1] != "" {
				n = reminderNumber(m.Parts[1])
			}
			u := m.Parts[2]
			if strings.HasPrefix(u, "час") || u == "ч" {
				n *= 60
			}
			if strings.HasPrefix(u, "д") || strings.HasPrefix(u, "сут") {
				n *= 1440
			}
			if m.Parts[3] != "" {
				n += reminderNumber(m.Parts[3])
			}
		}
		minutes := int(n)
		if n != float64(minutes) {
			return nil, nil, "Укажите интервал напоминания в целых минутах."
		}
		if minutes < 0 || minutes > 10080 {
			return nil, nil, "Укажите напоминание от 0 минут до 7 дней до события."
		}
		if result != nil && *result != minutes {
			return nil, nil, "Вы указали несколько интервалов напоминания. За сколько минут или часов напомнить?"
		}
		result = &minutes
	}
	if result != nil {
		enabled := true
		return result, &enabled, ""
	}
	return nil, nil, ""
}

// ReminderChoice accepts answers to the lead-time question, without treating
// "за час" as an event date, duration, or a new command.
func ReminderChoice(text string) (*int, *bool, bool, string) {
	text = strings.Trim(strings.ReplaceAll(strings.ToLower(text), "ё", "е"), " ,.!?:;-")
	text = replace(`^напомни(?:ть)?\s+`, text, "")
	if full(`нет|не надо|не нужно|без уведомлений|без напоминания`, text) {
		b := false
		return nil, &b, true, ""
	}
	if full(`в момент(?: начала| события)?|сразу|вовремя`, text) {
		n, b := 0, true
		return &n, &b, true, ""
	}
	if n, err := strconv.Atoi(text); err == nil {
		if n < 0 || n > 10080 {
			return nil, nil, true, "Укажите от 0 до 10080 минут."
		}
		b := true
		return &n, &b, true, ""
	}
	text = replace(`\s+до\s+(?:начала|события|встречи)\s*$`, text, "")
	if !strings.HasPrefix(text, "за ") {
		text = "за " + text
	}
	if !full(leadPattern, text) {
		return nil, nil, false, ""
	}
	n, enabled, err := reminderLead(text)
	return n, enabled, true, err
}

func withoutReminderSettings(text string) string {
	for _, pattern := range []string{leadPattern, noReminderPattern, onTimePattern, `\bдо\s+(?:начала|события)\b`} {
		ms := matches(`(?i)`+pattern, scheduleOnly(text))
		for i := len(ms) - 1; i >= 0; i-- {
			m := ms[i]
			text = text[:m.Start] + " " + text[m.End:]
		}
	}
	return text
}

const notificationClause = `(?i)\b(?:напомни\w*|не\s+забудь|(?:поставь|поставить|добавь|добавить|создай|создать|установи|включи)[\s,]+(?:(?:пожалуйста|мне)[\s,]+)*(?:напоминание|напоминалку|уведомление|оповещение))\b`

func creationPlan(text string, now time.Time) domain.Command {
	low := strings.ReplaceAll(strings.ToLower(scheduleOnly(text)), "ё", "е")
	var minutes *int
	var enabled *bool
	leadError := ""
	clean := text
	if has(notificationClause, low) || has(`\b(?:напоминан\w*|уведомлен\w*|оповещен\w*)\b`, low) || has(`\bза\s+.*\s+до\s+(?:начала|события|встречи)\b`, low) {
		minutes, enabled, leadError = reminderLead(low)
		clean = withoutReminderSettings(text)
	}
	main, notification := clean, ""
	separate := false
	if clause := first(notificationClause, scheduleOnly(clean)); clause != nil {
		main, notification = clean[:clause.Start], clean[clause.End:]
		separate = true
		if strings.Trim(main, " ,.!?:;-") == "" {
			// "Напомни в 12:00 о паре завтра в 12:50" has notification time first.
			for _, separator := range matches(`(?i)\b(?:о\s+том\s+что|что|о|об|про)\b`, notification) {
				left, right := notification[:separator.Start], notification[separator.End:]
				if Extract(left, now).Time != nil && Extract(right, now).Time != nil {
					main, notification = right, left
					break
				}
			}
		}
	}
	mt, nt := Extract(scheduleOnly(main), now), Extract(scheduleOnly(notification), now)
	if strings.Trim(main, " ,.!?:;-") == "" {
		mt.Duration = nt.Duration
	}
	// A clause containing no event content cannot define a separate event time.
	if mt.Time == nil {
		mt.Time = nt.Time
		nt.Time = nil
	}
	if mt.Date == nil {
		mt.Date = nt.Date
	}
	if mt.Start == nil {
		mt.Start, mt.End = nt.Start, nt.End
	}
	if mt.Duration == 0 {
		mt.Duration = time.Hour
	}
	errText := mt.Error
	if errText == "" {
		errText = nt.Error
	}
	if leadError != "" {
		errText = leadError
	}
	if extra := Extract(Extract(scheduleOnly(main), now).Clean(scheduleOnly(main)), now); extra.Time != nil {
		errText = "Вижу несколько времён события. Уточните время начала и за сколько минут напомнить."
	}
	if extra := Extract(Extract(scheduleOnly(notification), now).Clean(scheduleOnly(notification)), now); extra.Time != nil {
		errText = "Вижу несколько времён в просьбе о напоминании. Уточните время события и за сколько минут напомнить."
	}
	if mt.Time != nil && nt.Time != nil {
		day := mt.Date
		if day == nil {
			day = domain.Day(now)
		}
		alertDay := nt.Date
		if alertDay == nil {
			alertDay = day
		}
		difference := int(day.At(*mt.Time, now.Location()).Sub(alertDay.At(*nt.Time, now.Location())).Minutes())
		if difference < 0 || difference > 10080 {
			errText = "Время уведомления должно быть не позже события и не раньше чем за 7 дней. За сколько минут или часов напомнить?"
		} else if minutes != nil && *minutes != difference {
			errText = "Время уведомления не совпадает с указанным интервалом. За сколько минут или часов напомнить?"
		} else {
			minutes = &difference
			b := true
			enabled = &b
		}
	}
	mainTitle := cleanCreationTitle(main, now)
	contentTitle := cleanCreationTitle(notification, now)
	title := mainTitle
	if separate && contentTitle != "" && !purposeOnly(notification) {
		// A concrete reminder subject is stronger than a conversational preface.
		if title == "" || has(`(?i)\b(?:что|чтобы)\s+`, notification) {
			title = contentTitle
		}
	}
	if title == "" {
		title = contentTitle
	}
	if purposeOnly(notification) && mainTitle == "" {
		title = cleanCreationTitle(replace(`(?i)^.*?\bчтобы\s+(?:я\s+)?`, notification, ""), now)
	}
	if !usefulTitle(title) {
		title = ""
	}
	return domain.Command{Title: title, Date: mt.Date, Time: mt.Time, PeriodStart: mt.Start, PeriodEnd: mt.End, Duration: mt.Duration, ReminderMinutes: minutes, ReminderEnabled: enabled, Error: errText}
}

func purposeOnly(text string) bool {
	return has(`(?i)\b(?:чтобы\s+(?:я\s+)?не\s+(?:опозда\w*|забы\w*|пропусти\w*)|чтоб\s+не\s+забы\w*)\b`, text)
}

func cleanCreationTitle(text string, now time.Time) string {
	text = trimCourtesy(text)
	if explicit := first(`(?i)\b(?:с\s+названием|назови|название)\s*[«"]([^»"]+)[»"]`, text); explicit != nil {
		return strings.TrimSpace(explicit.Parts[1])
	}
	if quotes := matches(`[«"]([^»"]+)[»"]`, text); len(quotes) == 1 {
		return strings.TrimSpace(quotes[0].Parts[1])
	}
	// Extract schedules repeatedly so no second clock leaks into a title.
	for i := 0; i < 12; i++ {
		next := Extract(text, now).Clean(text)
		if next == text {
			break
		}
		text = next
	}
	text = createTitle(text)
	text = replace(`(?i)^\s*в\s+(?:(?:мой|наш)\s+)?календарь\b[\s,.:;-]*`, text, "")
	text = replace(`(?i)^\s*(?:и\s+)?(?:у\s+меня|мне|я)\b[\s,.:;-]*`, text, "")
	text = replace(`(?i)^\s*(?:нужно|надо|будет|будут|запланирована|запланирован|состоится)\b[\s,.:;-]*`, text, "")
	text = replace(`(?i)^\s*(?:о\s+том|про\s+то)\s*,?\s*что\s+`, text, "")
	text = replace(`(?i)^\s*(?:что|чтобы|о|об|про)\b[\s,.:;-]*`, text, "")
	text = replace(`(?i)^\s*(?:у\s+меня|мне|я)\b[\s,.:;-]*`, text, "")
	text = replace(`(?i)\s+чтобы\s+.*$`, text, "")
	text = replace(`(?i)\b(?:и|на|в|во|к|до)\s*$`, text, "")
	text = replace(`(?i)^\s*(?:на|в|во|к)\s*[,.:;-]+\s*`, text, "")
	text = strings.Trim(spaces(text), " ,.!?:;-")
	for _, pair := range [][2]string{{"встречу", "Встреча"}, {"встрече", "Встреча"}, {"пару", "Пара"}, {"паре", "Пара"}, {"лекцию", "Лекция"}, {"лекции", "Лекция"}, {"тренировку", "Тренировка"}, {"тренировке", "Тренировка"}, {"экзамене", "Экзамен"}, {"приеме", "Прием"}, {"совещании", "Совещание"}, {"собеседовании", "Собеседование"}} {
		text = replace(`(?i)^`+pair[0]+`\b`, text, pair[1])
	}
	return text
}

func scheduleOnly(text string) string {
	for _, quoted := range matches(`[«"]([^»"]+)[»"]`, text) {
		text = text[:quoted.Start] + strings.Repeat(" ", quoted.End-quoted.Start) + text[quoted.End:]
	}
	return text
}

func usefulTitle(title string) bool {
	if !has(`[\p{L}]`, title) || full(`(?i)(?:это|об этом|событие|уведомление|напоминание|оповещение|задача|пожалуйста|напомни|не забыть|мне|на|в|к|ну|эм|ммм)`, title) {
		return false
	}
	return !has(`(?i)^(?:на|в|к)\s+\d|^чтобы\b`, title)
}
