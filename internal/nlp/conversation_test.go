package nlp_test

import (
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
)

func TestConversationalIntent(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Moscow")
	now := time.Date(2026, 10, 14, 2, 17, 0, 0, loc)
	for _, tc := range []struct {
		text   string
		action domain.Action
		kind   domain.EventType
		title  string
	}{
		{"мне сегодня надо в универ к 12:50 на пару Оставь пожалуйста уведомление в календарь что у меня пара", domain.ActionCreate, domain.EventReminder, "Пара"},
		{"Поставь пожалуйста уведомление в календарь что у меня пара сегодня к 12:50", domain.ActionCreate, domain.EventReminder, "Пара"},
		{"Напомни завтра в 10:00 что нужно отправить отчет", domain.ActionCreate, domain.EventReminder, "Отправить отчет"},
		{"Добавь задачу узнать когда будет экзамен", domain.ActionCreate, domain.EventTask, "Узнать когда будет экзамен"},
		{"Напомни завтра в 10:00 удалить старые файлы", domain.ActionCreate, domain.EventReminder, "Удалить старые файлы"},
		{"Напомни завтра в 10:00 перенести встречу", domain.ActionCreate, domain.EventReminder, "Перенести встречу"},
		{"Можешь, пожалуйста, записать в календарь встречу с Олей завтра в 10:00", domain.ActionCreate, domain.EventMeeting, "Встреча с Олей"},
		{"Не забудь завтра в 10:00 позвонить маме", domain.ActionCreate, domain.EventReminder, "Позвонить маме"},
		{"Оставь напоминание позвонить маме завтра в 10:00", domain.ActionCreate, domain.EventReminder, "Позвонить маме"},
		{"Поставь задачу оставить посылку у двери", domain.ActionCreate, domain.EventTask, "Оставить посылку у двери"},
		{"Покажи что нужно купить завтра", domain.ActionList, domain.EventUnknown, ""},
		{"Когда была встреча с Олей", domain.ActionSearch, domain.EventMeeting, ""},
		{"Найди задачу удалить файлы", domain.ActionSearch, domain.EventTask, ""},
		{"Перенеси напоминание позвонить маме на завтра", domain.ActionUpdate, domain.EventReminder, ""},
	} {
		t.Run(tc.text, func(t *testing.T) {
			cmd := nlp.Parse(tc.text, now)
			if cmd.Action != tc.action || cmd.Kind != tc.kind || cmd.Title != tc.title || cmd.Error != "" {
				t.Fatalf("unexpected command: %+v", cmd)
			}
		})
	}
	for _, text := range []string{"Не добавляй встречу завтра", "Не удали задачу #42", "Не надо удалить задачу #42", "Не ставь напоминание завтра", "Оставь посылку у двери", "Оставь задачу без изменений", "Оставь встречу завтра как есть"} {
		cmd := nlp.Parse(text, now)
		if cmd.Action != domain.ActionHelp {
			t.Fatalf("unintended action for %q: %+v", text, cmd)
		}
	}
}

func TestSpokenClock(t *testing.T) {
	for _, tc := range []struct{ text, clock string }{
		{"сегодня в двенадцать пятьдесят", "12:50"},
		{"завтра к девяти", ""},
		{"завтра в девять тридцать", "09:30"},
		{"завтра в двадцать один", "21:00"},
		{"завтра в двадцать один сорок пять", "21:45"},
		{"завтра в 12 50", "12:50"},
		{"завтра в девять ноль пять утра", "09:05"},
		{"завтра в три пятнадцать дня", "15:15"},
		{"завтра в 23:59", "23:59"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			v := nlp.Extract(tc.text, time.Now())
			if tc.clock == "" {
				if v.Time != nil {
					t.Fatal(v.Time)
				}
				return
			}
			if v.Time == nil || v.Time.String() != tc.clock {
				t.Fatalf("unexpected time: %+v", v)
			}
		})
	}
}
