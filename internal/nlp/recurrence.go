package nlp

import (
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

var recurringWeekdays = []string{`понедельник(?:ам)?`, `вторник(?:ам)?`, `сред(?:ам|а|у)`, `четверг(?:ам)?`, `пятниц(?:ам|а|у)`, `суббот(?:ам|а|у)`, `воскресень(?:е|ям)`}

const recurrenceMarker = `\b(?:кажд(?:ый|ая|ую|ое|ые)|по\s+(?:будням|выходным))\b`

func extractRecurrence(text string, now time.Time, t *Temporal) {
	ruleCount := 0
	put := func(m *match, r domain.Recurrence) {
		if t.covered(m.Start) {
			return
		}
		if t.Recurrence != nil {
			t.Error = "Укажите одно правило повторения."
		}
		ruleCount++
		r.Timezone, r.Anchor = now.Location().String(), now.UTC()
		if r.Interval == 0 {
			r.Interval = 1
		}
		t.Recurrence = &r
		t.spans = append(t.spans, span{m.Start, m.End})
	}
	for _, m := range matches(`\b(?:каждый\s+месяц\s+(`+relativeAmount+`)\s+числа|(`+relativeAmount+`)\s+числа\s+каждого\s+месяца)\b`, text) {
		raw := m.Parts[1]
		if raw == "" {
			raw = m.Parts[2]
		}
		put(&m, domain.Recurrence{Frequency: "month", Day: int(reminderNumber(raw))})
	}
	for _, m := range matches(`\bкаждый\s+год\s+(`+relativeAmount+`)\s+(`+strings.Join(monthNames, "|")+`)\b`, text) {
		month := time.January
		for i, n := range monthNames {
			if n == m.Parts[2] {
				month = time.Month(i + 1)
			}
		}
		put(&m, domain.Recurrence{Frequency: "year", Day: int(reminderNumber(m.Parts[1])), Month: month})
	}
	for _, m := range matches(`\bкажд(?:ые|ый|ую)\s+(?:(`+relativeAmount+`)\s+)?(минут(?:у|ы)?|час(?:а|ов)?|день|дня|дней|недел(?:ю|и|ь))\b`, text) {
		freq := "day"
		unit := m.Parts[2]
		if strings.HasPrefix(unit, "минут") {
			freq = "minute"
		} else if strings.HasPrefix(unit, "час") {
			freq = "hour"
		} else if strings.HasPrefix(unit, "недел") {
			freq = "week"
		}
		count := 1.0
		if m.Parts[1] != "" {
			count = reminderNumber(m.Parts[1])
		}
		if count <= 0 || count != float64(int(count)) {
			t.Error = "Интервал повторения должен быть целым."
		}
		r := domain.Recurrence{Frequency: freq, Interval: int(count)}
		if freq == "week" {
			weekdayPattern := strings.Join(recurringWeekdays, "|")
			if extra := first(`^\s+по\s+(`+weekdayPattern+`)(?:\s+и\s+(`+weekdayPattern+`))*\b`, text[m.End:]); extra != nil {
				r.Weekdays = recurrenceDays(extra.Parts[0])
				m.End += extra.End
			} else {
				t.Error = "В какой день недели повторять?"
				r.Weekdays = nil
			}
		}
		put(&m, r)
	}
	weekdayPattern := strings.Join(recurringWeekdays, "|")
	for _, m := range matches(`\bкажд(?:ый|ую|ое)\s+(?:`+weekdayPattern+`)(?:\s*(?:,|и)\s*(?:`+weekdayPattern+`))*\b`, text) {
		put(&m, domain.Recurrence{Frequency: "week", Weekdays: recurrenceDays(m.Parts[0])})
	}
	for _, m := range matches(`\bпо\s+(будням|выходным)\b`, text) {
		days := []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}
		if m.Parts[1] == "выходным" {
			days = []time.Weekday{time.Saturday, time.Sunday}
		}
		put(&m, domain.Recurrence{Frequency: "week", Weekdays: days})
	}
	if t.Recurrence != nil {
		if err := t.Recurrence.Validate(false); err != nil {
			t.Error = err.Error()
		}
	}
	if has(recurrenceMarker, t.mask(text)) || has(`\bкаждого\s+месяца\b`, t.mask(text)) {
		t.Error = "Уточните правило повторения: частоту и день."
	}
	if ruleCount > 1 {
		t.Recurrence = nil
	}
}

func recurrenceDays(text string) []time.Weekday {
	result := []time.Weekday{}
	for i, p := range recurringWeekdays {
		if has(`\b(?:`+p+`)\b`, text) {
			result = append(result, time.Weekday((i+1)%7))
		}
	}
	return result
}

// ResolveRecurrence is also used after merging a reply into persisted pending.
func ResolveRecurrence(c *domain.Command, now time.Time) {
	if c.Recurrence == nil {
		return
	}
	r := *c.Recurrence
	if !r.Elapsed() {
		r.LocalTime = c.Time
	}
	c.Recurrence = &r
	if !r.Elapsed() && r.LocalTime == nil || r.Frequency == "week" && len(r.Weekdays) == 0 {
		c.Date = nil
		c.PeriodStart = nil
		c.PeriodEnd = nil
		return
	}
	if c.Error != "" {
		return
	}
	next, err := r.Next(now)
	if err != nil {
		c.Error = err.Error()
		return
	}
	loc, _ := time.LoadLocation(r.Timezone)
	next = next.In(loc)
	c.Date = domain.Day(next)
	c.Time = &domain.TimeOfDay{Hour: next.Hour(), Minute: next.Minute()}
	c.StartSecond = next.Second()
	c.PeriodStart, c.PeriodEnd = c.Date, c.Date.AddDays(1)
}
