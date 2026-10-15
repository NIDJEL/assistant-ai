package nlp

import (
	"math"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

// Components are consumed contiguously from each "через" marker. A conjunction
// belongs to the interval only when another unit follows it.
type relativeUnit struct {
	pattern      string
	scale        time.Duration
	days, months int
}

var relativeUnits = []relativeUnit{
	{`четверть\s+часа`, 15 * time.Minute, 0, 0},
	{`пол\s*часа`, 30 * time.Minute, 0, 0}, {`полгода`, 0, 0, 6},
	{`секунд(?:а|у|ы)?`, time.Second, 0, 0}, {`минут(?:а|у|ы)?`, time.Minute, 0, 0},
	{`час(?:а|ов|ик)?`, time.Hour, 0, 0}, {`сутки|суток|день|дня|дней`, 0, 1, 0},
	{`недел(?:я|ю|и|ь)`, 0, 7, 0}, {`месяц(?:а|ев)?`, 0, 0, 1}, {`года|год|лет`, 0, 0, 12},
}

const relativeAmount = `(?:` + reminderAmount + `|пару|пара)`

func extractRelative(text string, now time.Time, result *Temporal) []*domain.Date {
	var dates []*domain.Date
	var target *time.Time
	for _, marker := range matches(`\bчерез\s+`, text) {
		if result.covered(marker.Start) {
			continue
		}
		offset := marker.End
		days, months := 0, 0
		elapsed := time.Duration(0)
		seen := map[int]bool{}
		components := 0
		invalid := false
		for {
			consumed := false
			for index, unit := range relativeUnits {
				prefix := `^`
				if components > 0 {
					prefix += `\s+(?:и\s+)?`
				}
				m := first(prefix+`(?:(`+relativeAmount+`)\s+)?(?:`+unit.pattern+`)\b`, text[offset:])
				if m == nil {
					continue
				}
				count := 1.0
				if m.Parts[1] != "" {
					count = reminderNumber(m.Parts[1])
				}
				if count <= 0 || count > 366 || (unit.days != 0 || unit.months != 0) && count != math.Trunc(count) {
					invalid = true
					result.Error = "Укажите интервал от 1 до 366 дней (или часов/минут)."
				}
				if seen[index] {
					result.Error = "Единица интервала указана повторно. Уточните задержку."
				}
				seen[index] = true
				days += int(count) * unit.days
				months += int(count) * unit.months
				elapsed += time.Duration(count * float64(unit.scale))
				offset += m.End
				components++
				consumed = true
				break
			}
			if !consumed {
				break
			}
		}
		if components == 0 {
			result.Error = "Уточните интервал после «через»."
			continue
		}
		result.spans = append(result.spans, span{marker.Start, offset})
		if invalid {
			continue
		}
		next := domain.ShiftCalendar(now, months, days).Add(elapsed).Truncate(time.Second)
		if next.Year() > 9999 {
			result.Error = "Дата выходит за пределы календаря."
			continue
		}
		if target != nil && !target.Equal(next) {
			result.Error = "В запросе несколько разных интервалов. Когда напомнить?"
		}
		target = &next
		dates = append(dates, domain.Day(next))
		if elapsed != 0 {
			result.Time = &domain.TimeOfDay{Hour: next.Hour(), Minute: next.Minute()}
			result.StartSecond = next.Second()
		}
	}
	result.At = target
	result.Relative = target != nil
	return dates
}

func ParseDateTime(text string, now time.Time) Temporal {
	t := Extract(text, now)
	if t.At != nil && t.Time == nil {
		t.Time = &domain.TimeOfDay{Hour: t.At.Hour(), Minute: t.At.Minute()}
		t.StartSecond = t.At.Second()
	}
	return t
}

// A bare weekday supplied after choosing a week belongs to that week. Relative
// delays use the actual clock; explicit dates keep their value.
func ParseDateTimeReply(text string, now time.Time, previous domain.Command) Temporal {
	t := ParseDateTime(text, now)
	if previous.Recurrence != nil && previous.Recurrence.Frequency == "week" && len(previous.Recurrence.Weekdays) == 0 {
		if days := recurrenceDays(NormalizeText(text).Analysis); len(days) > 0 {
			rule := *previous.Recurrence
			rule.Weekdays = days
			t.Recurrence = &rule
		}
	}
	if previous.Date == nil && previous.PeriodStart != nil && t.weekday && !t.Relative && t.Error == "" {
		t = ParseDateTime(text, previous.PeriodStart.At(domain.TimeOfDay{}, now.Location()))
	}
	return t
}
