package nlp_test

import (
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
)

func TestConversationalCreateTitles(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Moscow")
	now := time.Date(2026, 10, 14, 2, 0, 0, 0, loc)
	for _, tc := range []struct{ message, title string }{
		{"Привет добавь Запись к врачу на завтра в 9:00 утра", "Запись к врачу"},
		{"Здравствуйте, пожалуйста, добавь запись к врачу завтра в 9:00", "Запись к врачу"},
		{"Добрый день! Добавь, пожалуйста, встречу с командой завтра в 9:00", "Встреча с командой"},
		{"Пожалуйста, добавь в календарь запись к врачу завтра в 9:00", "Запись к врачу"},
		{"Добавь запись к врачу завтра в 9:00, пожалуйста", "Запись к врачу"},
		{"Создай новое напоминание проверить почту завтра в 9:00", "Проверить почту"},
		{"Поставь задачу передать привет маме завтра в 9:00", "Передать привет маме"},
		{"Поставь задачу приветствие коллегам завтра в 9:00", "Приветствие коллегам"},
	} {
		t.Run(tc.message, func(t *testing.T) {
			cmd := nlp.Parse(tc.message, now)
			if cmd.Action != domain.ActionCreate || cmd.Title != tc.title {
				t.Fatalf("unexpected command: %+v", cmd)
			}
			if cmd.Date == nil || cmd.Date.String() != "2026-10-15" || cmd.Time == nil || cmd.Time.String() != "09:00" {
				t.Fatalf("title cleanup changed timing: %+v", cmd)
			}
		})
	}
}
