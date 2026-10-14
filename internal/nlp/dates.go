package nlp

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

var numbers = map[string]float64{"один": 1, "одну": 1, "одна": 1, "два": 2, "две": 2, "три": 3, "четыре": 4, "пять": 5, "шесть": 6, "семь": 7, "восемь": 8, "девять": 9, "десять": 10, "одиннадцать": 11, "двенадцать": 12, "тринадцать": 13, "четырнадцать": 14, "пятнадцать": 15, "шестнадцать": 16, "семнадцать": 17, "восемнадцать": 18, "девятнадцать": 19, "двадцать": 20, "полчаса": .5, "пару": 2, "пара": 2}

const numberPattern = `(?:\d+|двадцать\s+(?:один|два|три)|одиннадцать|двенадцать|тринадцать|четырнадцать|пятнадцать|шестнадцать|семнадцать|восемнадцать|девятнадцать|двадцать|полчаса|один|одну|одна|два|две|три|четыре|пять|шесть|семь|восемь|девять|десять)`

var weekdays = []string{`понедельник`, `вторник`, `сред[ау]`, `четверг`, `пятниц[ау]`, `суббот[ау]`, `воскресенье`}
var monthNames = []string{"января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"}

type span struct{ start, end int }
type Temporal struct {
	Recurrence  *domain.Recurrence
	Date        *domain.Date
	Time        *domain.TimeOfDay
	Start, End  *domain.Date
	Duration    time.Duration
	Error       string
	At          *time.Time
	StartSecond int
	Relative    bool
	spans       []span
	weekday     bool
}

func number(value string) float64 {
	if n, err := strconv.Atoi(value); err == nil {
		return float64(n)
	}
	total := 0.0
	for _, w := range strings.Fields(value) {
		total += numbers[w]
	}
	return total
}
func (t Temporal) Clean(text string) string {
	spans := append([]span(nil), t.spans...)
	sort.Slice(spans, func(i, j int) bool { return spans[i].start > spans[j].start })
	for _, s := range spans {
		if s.end <= len(text) {
			text = text[:s.start] + " " + text[s.end:]
		}
	}
	return strings.Trim(spaces(text), " ,.!?;:-")
}
func (t Temporal) covered(i int) bool {
	for _, s := range t.spans {
		if s.start <= i && i < s.end {
			return true
		}
	}
	return false
}
func Extract(text string, now time.Time) Temporal {
	text = strings.ReplaceAll(strings.ToLower(text), "ё", "е")
	result := Temporal{Duration: time.Hour}
	day := domain.Day(now)
	dates := []*domain.Date{}
	remember := func(m *match, date *domain.Date) {
		if result.covered(m.Start) {
			return
		}
		dates = append(dates, date)
		result.spans = append(result.spans, span{m.Start, m.End})
	}
	extractRecurrence(text, now, &result)
	dates = append(dates, extractRelative(text, now, &result)...)
	for _, m := range matches(`\b(\d{4}-\d{2}-\d{2}|\d{1,2}\.\d{1,2}(?:\.\d{4})?)\b`, text) {
		if result.covered(m.Start) {
			continue
		}
		raw := m.Parts[1]
		var date *domain.Date
		var err error
		if strings.Contains(raw, "-") {
			date, err = domain.ParseDate(raw)
		} else {
			bits := strings.Split(raw, ".")
			d, _ := strconv.Atoi(bits[0])
			mo, _ := strconv.Atoi(bits[1])
			year := now.Year()
			if len(bits) == 3 {
				year, _ = strconv.Atoi(bits[2])
			}
			date, err = domain.ParseDate(fmt.Sprintf("%04d-%02d-%02d", year, mo, d))
			if err == nil && len(bits) == 2 && date.Before(day.Time) {
				date, err = domain.ParseDate(fmt.Sprintf("%04d-%02d-%02d", year+1, mo, d))
			}
		}
		if err != nil {
			result.spans = append(result.spans, span{m.Start, m.End})
			result.Error = "Такой даты нет в календаре. Укажите дату в формате ДД.ММ.ГГГГ."
		} else {
			remember(&m, date)
		}
	}
	for _, m := range matches(`\b(\d{1,2})\s+(`+strings.Join(monthNames, "|")+`)(?:\s+(\d{4})(?:\s+года)?)?\b`, text) {
		if result.covered(m.Start) {
			continue
		}
		d, _ := strconv.Atoi(m.Parts[1])
		month := 0
		for i, n := range monthNames {
			if n == m.Parts[2] {
				month = i + 1
			}
		}
		year := now.Year()
		if m.Parts[3] != "" {
			year, _ = strconv.Atoi(m.Parts[3])
		}
		date, err := domain.ParseDate(fmt.Sprintf("%04d-%02d-%02d", year, month, d))
		if err == nil && m.Parts[3] == "" && date.Before(day.Time) {
			date, err = domain.ParseDate(fmt.Sprintf("%04d-%02d-%02d", year+1, month, d))
		}
		if err != nil {
			result.spans = append(result.spans, span{m.Start, m.End})
			result.Error = "Такой даты нет в календаре. Уточните день и месяц."
		} else {
			remember(&m, date)
		}
	}
	offsets := map[string]int{"сегодня": 0, "завтра": 1, "послезавтра": 2, "вчера": -1, "позавчера": -2}
	for _, m := range matches(`\b(послезавтра|позавчера|сегодня|завтра|вчера)\b`, text) {
		remember(&m, day.AddDays(offsets[m.Parts[1]]))
	}
	weekday := (int(now.Weekday()) + 6) % 7
	for target, pattern := range weekdays {
		if m := first(`\b(?:(?:в|во|на|до)\s+)?(?:(следующ\w*|эт\w*)\s+)?(`+pattern+`)\b`, text); m != nil {
			offset := (target - weekday + 7) % 7
			if strings.HasPrefix(m.Parts[1], "следующ") {
				offset = 7 - weekday + target
			}
			remember(m, day.AddDays(offset))
			result.weekday = m.Parts[1] == ""
		}
	}
	if m := first(`\b(?:(?:на|за|до конца)\s+)?(?:(следующ\w*|эт\w*|текущ\w*|прошл\w*)\s+)?недел[юие]\b`, text); m != nil && !result.covered(m.Start) {
		monday := day.AddDays(-weekday)
		shift := 0
		if strings.HasPrefix(m.Parts[1], "следующ") {
			shift = 7
		} else if strings.HasPrefix(m.Parts[1], "прошл") {
			shift = -7
		}
		result.Start, result.End = monday.AddDays(shift), monday.AddDays(shift+7)
		result.spans = append(result.spans, span{m.Start, m.End})
		if strings.Contains(m.Parts[0], "до конца") {
			dates = append(dates, monday.AddDays(6))
		}
	}
	if m := first(`\b(?:(?:на|за)\s+)?выходны[ех]\b`, text); m != nil && !result.covered(m.Start) {
		offset := (5 - weekday + 7) % 7
		if weekday == 6 {
			offset = -1
		}
		saturday := day.AddDays(offset)
		result.Start, result.End = saturday, saturday.AddDays(2)
		result.spans = append(result.spans, span{m.Start, m.End})
	}
	if m := first(`\b(?:за|в)\s+(\d{4})\s*(?:год\w*)?`, text); m != nil && !result.covered(m.Start) {
		y, _ := strconv.Atoi(m.Parts[1])
		if y >= 1900 && y < 9999 {
			result.Start, _ = domain.ParseDate(fmt.Sprintf("%04d-01-01", y))
			result.End, _ = domain.ParseDate(fmt.Sprintf("%04d-01-01", y+1))
			result.spans = append(result.spans, span{m.Start, m.End})
		}
	}
	for _, m := range matches(`\b(`+relativeAmount+`)\s+числа\s+следующего\s+месяца\b`, text) {
		d := int(reminderNumber(m.Parts[1]))
		base := domain.ShiftCalendar(now, 1, 0)
		date, err := domain.ParseDate(fmt.Sprintf("%04d-%02d-%02d", base.Year(), base.Month(), d))
		if err != nil {
			result.Error = "Такой даты нет в следующем месяце."
			result.spans = append(result.spans, span{m.Start, m.End})
		} else {
			remember(&m, date)
		}
	}
	unique := map[string]bool{}
	for _, date := range dates {
		unique[date.String()] = true
	}
	if len(unique) > 1 {
		result.Error = "В запросе несколько разных дат. Укажите одну дату события."
	}
	if len(dates) > 0 {
		result.Date = dates[0]
		if result.Start == nil {
			result.Start, result.End = result.Date, result.Date.AddDays(1)
		}
	}
	if m := first(`\b(?:на|длительностью)\s+(?:(`+numberPattern+`)\s*)?(час\w*|минут\w*)\b`, text); m != nil && !result.covered(m.Start) {
		raw := m.Parts[1]
		if raw == "" {
			raw = "один"
		}
		minutes := number(raw)
		if strings.HasPrefix(m.Parts[2], "час") {
			minutes *= 60
		}
		result.Duration = time.Duration(minutes) * time.Minute
		result.spans = append(result.spans, span{m.Start, m.End})
		if result.Duration < 5*time.Minute || result.Duration > 24*time.Hour {
			result.Error = "Длительность встречи должна быть от 5 минут до 24 часов."
			result.Duration = time.Hour
		}
	}
	clockText := result.mask(text)
	clock := first(`\b(?:(?:примерно\s+)?(?:в|к|около)\s+)?(\d{1,2}):(\d{2})(?:\s*(утра|вечера|дня|ночи))?\b`, clockText)
	hour, minute := 0, 0
	meridiem := ""
	if clock != nil {
		hour, _ = strconv.Atoi(clock.Parts[1])
		minute, _ = strconv.Atoi(clock.Parts[2])
		meridiem = clock.Parts[3]
	} else if spoken, h, m, period := spokenClock(clockText); spoken != nil && !result.covered(spoken.Start) && !result.covered(spoken.End-1) {
		clock, hour, minute, meridiem = spoken, h, m, period
	} else {
		clock = first(`\b(?:примерно\s+)?(?:в|к|около)\s+(`+numberPattern+`)(?:\s+(утра|вечера|дня|ночи))?\b`, clockText)
		if clock != nil && (result.covered(clock.Start) || result.covered(clock.End-1)) {
			clock = nil
		}
		if clock != nil {
			hour = int(number(clock.Parts[1]))
			meridiem = clock.Parts[2]
		}
	}
	if clock != nil {
		if (meridiem == "вечера" || meridiem == "дня") && hour < 12 {
			hour += 12
		} else if (meridiem == "ночи" || meridiem == "утра") && hour == 12 {
			hour = 0
		}
		if part := first(`\b(?:утром|днем|вечером|ночью)\b`, clockText); part != nil {
			if meridiem == "" && (part.Parts[0] == "вечером" || part.Parts[0] == "днем") && hour < 12 {
				hour += 12
			}
			result.spans = append(result.spans, span{part.Start, part.End})
		}
		result.spans = append(result.spans, span{clock.Start, clock.End})
		if hour > 23 || minute > 59 {
			result.Error = "Время должно быть от 00:00 до 23:59. Уточните время."
		} else {
			result.Time = &domain.TimeOfDay{Hour: hour, Minute: minute}
			result.StartSecond = 0
		}
	}
	if clock == nil {
		if part := first(`\b(?:утром|днем|вечером|ночью)\b`, clockText); part != nil {
			hours := map[string]int{"утром": 9, "днем": 14, "вечером": 18, "ночью": 23}
			result.Time = &domain.TimeOfDay{Hour: hours[part.Parts[0]]}
			result.StartSecond = 0
			result.spans = append(result.spans, span{part.Start, part.End})
		}
	}
	if clock == nil && full(numberPattern+`(?:\s*(утра|вечера|дня|ночи))?[.!]?`, strings.TrimSpace(text)) {
		bare := Extract("в "+strings.TrimSpace(text), now)
		result.Time, result.Error = bare.Time, bare.Error
	}
	if result.Date != nil && result.Time != nil && !(result.Relative && clock == nil && result.At != nil && result.Time.Hour == result.At.Hour() && result.Time.Minute == result.At.Minute() && result.StartSecond == result.At.Second()) {
		at := result.Date.At(*result.Time, now.Location()).Add(time.Duration(result.StartSecond) * time.Second)
		if result.Relative {
			at = domain.LocalInstant(result.Date.Year(), result.Date.Month(), result.Date.Day(), result.Time.Hour, result.Time.Minute, result.StartSecond, now.Location())
		}
		result.At = &at
	}
	return result
}

// Mask consumed spans without changing offsets or matching nested dates/times.
func (t Temporal) mask(text string) string {
	for _, s := range t.spans {
		if s.end <= len(text) {
			text = text[:s.start] + strings.Repeat(" ", s.end-s.start) + text[s.end:]
		}
	}
	return text
}
