package assistant

import (
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"strings"
	"time"
)

func russianCases(n int, one, few, many string) string {
	if n%100 >= 11 && n%100 <= 14 {
		return many
	}
	if n%10 == 1 {
		return one
	}
	if n%10 >= 2 && n%10 <= 4 {
		return few
	}
	return many
}

func readKindLabel(kind domain.EventType, n int) string {
	if kind == domain.EventMeeting {
		return russianCases(n, "встреча", "встречи", "встреч")
	}
	if kind == domain.EventReminder {
		return russianCases(n, "напоминание", "напоминания", "напоминаний")
	}
	if kind == domain.EventTask {
		return russianCases(n, "задача", "задачи", "задач")
	}
	return russianCases(n, "дело", "дела", "дел")
}

func readPeriodLabel(c domain.Command, now time.Time) string {
	if c.PeriodStart != nil && c.PeriodEnd != nil && c.PeriodEnd.Equal(c.PeriodStart.AddDays(1).Time) {
		if c.PeriodStart.Equal(domain.Day(now).Time) {
			return "Сегодня"
		}
		if c.PeriodStart.Equal(domain.Day(now).AddDays(1).Time) {
			return "Завтра"
		}
	}
	return "В ваших планах"
}

func relativeDescription(e domain.Event, now time.Time) string {
	text := describe(e)
	if e.Date != nil {
		day := ""
		if e.Date.Equal(domain.Day(now).Time) {
			day = "сегодня"
		}
		if e.Date.Equal(domain.Day(now).AddDays(1).Time) {
			day = "завтра"
		}
		if day != "" {
			text = strings.Replace(text, e.Date.Format("02.01.2006"), day, 1)
		}
	}
	return text
}
