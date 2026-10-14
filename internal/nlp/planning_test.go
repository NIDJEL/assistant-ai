package nlp_test

import (
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
)

func TestEventSubjectAndNotificationTime(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Moscow")
	now := time.Date(2026, 10, 14, 2, 0, 0, 0, loc)
	for _, tc := range []struct {
		text, title, clock, date string
		minutes                  int
	}{
		{"у меня завтра пара в 12:50 Поставь пожалуйста уведомление на 12:00 чтобы я не опоздал на пару", "Пара", "12:50", "2026-10-15", 50},
		{"У меня завтра прием у стоматолога в 15:00. Напомни за час", "Прием у стоматолога", "15:00", "2026-10-15", 60},
		{"Добавь встречу с командой завтра в 15:00 и напомни за 15 минут", "Встреча с командой", "15:00", "2026-10-15", 15},
		{"Напомни за полчаса о встрече с Олей завтра в 15:00", "Встреча с Олей", "15:00", "2026-10-15", 30},
		{"Напомни в 12:00 о паре завтра в 12:50", "Пара", "12:50", "2026-10-15", 50},
		{"Завтра экзамен в 09:00, поставь уведомление сегодня в 21:00", "Экзамен", "09:00", "2026-10-15", 720},
		{"Поставь задачу проверить почту завтра в 10:00 и напомни за час", "Проверить почту", "10:00", "2026-10-15", 60},
		{"Добавь встречу с названием «Релиз Ориона» завтра в 10:00, напомни за 2 часа", "Релиз Ориона", "10:00", "2026-10-15", 120},
		{"Добавь задачу «Проверить отчёт за час в 12:00» завтра в 10:00, напомни за 15 минут", "Проверить отчёт за час в 12:00", "10:00", "2026-10-15", 15},
	} {
		t.Run(tc.text, func(t *testing.T) {
			cmd := nlp.Parse(tc.text, now)
			if cmd.Action != domain.ActionCreate || cmd.Title != tc.title || cmd.Date == nil || cmd.Date.String() != tc.date || cmd.Time == nil || cmd.Time.String() != tc.clock || cmd.ReminderMinutes == nil || *cmd.ReminderMinutes != tc.minutes || cmd.Error != "" {
				t.Fatalf("unexpected plan: %+v", cmd)
			}
		})
	}
}

func TestAmbiguousSchedulesAndMissingSubjects(t *testing.T) {
	for _, text := range []string{
		"У меня завтра пара в 12:50, поставь уведомление на 13:00",
		"У меня завтра пара в 12:50, напомни за час на 12:00",
		"Напомни завтра в 10:00 и в 12:00 позвонить маме",
	} {
		cmd := nlp.Parse(text, time.Now())
		if cmd.Error == "" {
			t.Fatalf("ambiguous times were guessed: %+v", cmd)
		}
	}
	cmd := nlp.Parse("Поставь пожалуйста уведомление завтра в 12:00", time.Now())
	if cmd.Title != "" {
		t.Fatal("missing subject became a title", cmd)
	}
	task := nlp.Parse("Поставь задачу подготовить отчёт за два часа", time.Now())
	if task.ReminderMinutes != nil || task.Title != "Подготовить отчёт за два часа" {
		t.Fatal("task content became notification settings", task)
	}
}

func TestReminderChoices(t *testing.T) {
	for _, tc := range []struct {
		text    string
		minutes int
	}{{"за час", 60}, {"За 30 минут", 30}, {"за полчаса", 30}, {"за полтора часа", 90}, {"за 1 час 30 минут", 90}, {"за 1ч30мин", 90}, {"за 30мин", 30}, {"за 1,5 часа", 90}, {"за два часа и пятнадцать минут", 135}, {"в момент события", 0}, {"60", 60}, {"напомни за час до начала", 60}, {"за день", 1440}} {
		n, enabled, ok, err := nlp.ReminderChoice(tc.text)
		if !ok || n == nil || *n != tc.minutes || enabled == nil || !*enabled || err != "" {
			t.Fatalf("%q: %v %v %v %q", tc.text, n, enabled, ok, err)
		}
	}
}
