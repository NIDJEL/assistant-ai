package nlp_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/nlp"
)

func russianNow(t *testing.T) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatal(err)
	}
	return time.Date(2026, 10, 14, 12, 30, 17, 0, loc)
}

func checkRussianCommand(t *testing.T, text string, now time.Time, intent nlp.Intent, title, date, clock string, second int, missing []nlp.Entity) {
	t.Helper()
	c := nlp.ParseCommand(text, now)
	if c.Intent != intent || c.Title != title || c.Error != "" {
		t.Fatalf("intent/text: %+v", c)
	}
	gotDate, gotClock := "", ""
	if c.Date != nil {
		gotDate = c.Date.String()
	}
	if c.Time != nil {
		gotClock = c.Time.String()
	}
	if gotDate != date || gotClock != clock || c.StartSecond != second {
		t.Fatalf("timestamp: %s %s:%02d; want %s %s:%02d (%+v)", gotDate, gotClock, c.StartSecond, date, clock, second, c)
	}
	if !reflect.DeepEqual(c.Missing, missing) || c.NeedsClarification() != (len(missing) != 0) {
		t.Fatalf("clarification: missing=%v, needed=%v; want %v", c.Missing, c.NeedsClarification(), missing)
	}
	if c.Original != text || c.Confidence < .8 {
		t.Fatalf("lost original or low confidence: %+v", c)
	}
}

func TestRussianRelativeCommands(t *testing.T) {
	now := russianNow(t)
	// Each unit is exercised before the directive, between directive/content,
	// and after the content. Expectations do not use the parser's unit registry.
	for _, tc := range []struct {
		phrase string
		offset time.Duration
	}{
		{"через секунду", time.Second},
		{"через 5 секунд", 5 * time.Second},
		{"через две секунды", 2 * time.Second},
		{"через минуту", time.Minute},
		{"через 1 минуту", time.Minute},
		{"через одну минуту", time.Minute},
		{"через две минуты", 2 * time.Minute},
		{"через 5 минут", 5 * time.Minute},
		{"через три минуты", 3 * time.Minute},
		{"через четыре минуты", 4 * time.Minute},
		{"через пять минут", 5 * time.Minute},
		{"через десять минут", 10 * time.Minute},
		{"через пятнадцать минут", 15 * time.Minute},
		{"через двадцать минут", 20 * time.Minute},
		{"через полчаса", 30 * time.Minute},
		{"через пол часа", 30 * time.Minute},
		{"через час", time.Hour},
		{"через часик", time.Hour},
		{"через один час", time.Hour},
		{"через 2 часа", 2 * time.Hour},
		{"через пару часов", 2 * time.Hour},
		{"через сутки", 24 * time.Hour},
		{"через день", 24 * time.Hour},
		{"через два дня", 48 * time.Hour},
		{"через неделю", 7 * 24 * time.Hour},
		{"через две недели", 14 * 24 * time.Hour},
	} {
		for _, format := range []string{"Напомни %s проверить духовку", "%s напомни проверить духовку", "Напомни проверить духовку %s"} {
			text := fmt.Sprintf(format, tc.phrase)
			t.Run(text, func(t *testing.T) {
				target := now.Add(tc.offset)
				checkRussianCommand(t, text, now, nlp.CreateReminder, "Проверить духовку", target.Format("2006-01-02"), target.Format("15:04"), target.Second(), nil)
			})
		}
	}
}

func TestRussianNaturalCommands(t *testing.T) {
	now := russianNow(t)
	dateMissing := []nlp.Entity{nlp.DateEntity, nlp.TimeEntity}
	timeMissing := []nlp.Entity{nlp.TimeEntity}
	for _, tc := range []struct {
		text, title, date, clock string
		second                   int
		intent                   nlp.Intent
		missing                  []nlp.Entity
	}{
		{"Напомни через минуту снять чайник с плиты", "Снять чайник с плиты", "2026-10-14", "12:31", 17, nlp.CreateReminder, nil},
		{"Напомни мне что через минуту нужно снять чайник с плиты", "Снять чайник с плиты", "2026-10-14", "12:31", 17, nlp.CreateReminder, nil},
		{"Напомни мне что через минуту и нужно снять чайник с плиты", "Снять чайник с плиты", "2026-10-14", "12:31", 17, nlp.CreateReminder, nil},
		{"Через минуту напомни снять чайник", "Снять чайник", "2026-10-14", "12:31", 17, nlp.CreateReminder, nil},
		{"Мне через минуту надо снять чайник напомни", "Снять чайник", "2026-10-14", "12:31", 17, nlp.CreateReminder, nil},
		{"Мне через минуту нужно выключить плиту напомни", "Выключить плиту", "2026-10-14", "12:31", 17, nlp.CreateReminder, nil},
		{"Мне завтра в 8 надо купить молоко напомни", "Купить молоко", "2026-10-15", "08:00", 0, nlp.CreateReminder, nil},
		{"Завтра в 8 купить молоко напомни", "Купить молоко", "2026-10-15", "08:00", 0, nlp.CreateReminder, nil},
		{"Поставь напоминание через 5 минут проверить духовку", "Проверить духовку", "2026-10-14", "12:35", 17, nlp.CreateReminder, nil},
		{"Через час напомни позвонить маме", "Позвонить маме", "2026-10-14", "13:30", 17, nlp.CreateReminder, nil},
		{"Напомни позвонить маме через час", "Позвонить маме", "2026-10-14", "13:30", 17, nlp.CreateReminder, nil},
		{"Напомни завтра купить молоко", "Купить молоко", "2026-10-15", "", 0, nlp.CreateReminder, timeMissing},
		{"Напомни завтра в 8 купить молоко", "Купить молоко", "2026-10-15", "08:00", 0, nlp.CreateReminder, nil},
		{"Завтра в 8 напомни купить молоко", "Купить молоко", "2026-10-15", "08:00", 0, nlp.CreateReminder, nil},
		{"Напомни купить молоко завтра в 8", "Купить молоко", "2026-10-15", "08:00", 0, nlp.CreateReminder, nil},
		{"Напомни купить молоко завтра в восемь", "Купить молоко", "2026-10-15", "08:00", 0, nlp.CreateReminder, nil},
		{"В пятницу напомни отправить отчет", "Отправить отчет", "2026-10-16", "", 0, nlp.CreateReminder, timeMissing},
		{"В пятницу в 18:30 напомни отправить отчет", "Отправить отчет", "2026-10-16", "18:30", 0, nlp.CreateReminder, nil},
		{"Через два дня напомни оплатить интернет", "Оплатить интернет", "2026-10-16", "12:30", 17, nlp.CreateReminder, nil},
		{"Через два дня в 10 утра напомни оплатить интернет", "Оплатить интернет", "2026-10-16", "10:00", 0, nlp.CreateReminder, nil},
		{"Через 2 часа напомни проверить сервер", "Проверить сервер", "2026-10-14", "14:30", 17, nlp.CreateReminder, nil},
		{"Напомни через полчаса выйти из дома", "Выйти из дома", "2026-10-14", "13:00", 17, nlp.CreateReminder, nil},
		{"Не дай забыть через час выключить компьютер", "Выключить компьютер", "2026-10-14", "13:30", 17, nlp.CreateReminder, nil},
		{"Мне надо не забыть завтра позвонить врачу", "Позвонить врачу", "2026-10-15", "", 0, nlp.CreateReminder, timeMissing},
		{"Нужно не забыть завтра в 10 позвонить врачу", "Позвонить врачу", "2026-10-15", "10:00", 0, nlp.CreateReminder, nil},
		{"Скажи мне через час выключить плиту", "Выключить плиту", "2026-10-14", "13:30", 17, nlp.CreateReminder, nil},
		{"Напомнить завтра в 10 проверить почту", "Проверить почту", "2026-10-15", "10:00", 0, nlp.CreateReminder, nil},
		{"Напоминание завтра в 10 проверить почту", "Проверить почту", "2026-10-15", "10:00", 0, nlp.CreateReminder, nil},
		{"  НАПОМНИ   МНЕ\tчерез минуту\nотправить отчёт  ", "Отправить отчёт", "2026-10-14", "12:31", 17, nlp.CreateReminder, nil},
		{"Поставь напоминание на пятницу в 18:30 забрать документы", "Забрать документы", "2026-10-16", "18:30", 0, nlp.CreateReminder, nil},
		{"В пятницу вечером напомни отправить отчёт", "Отправить отчёт", "2026-10-16", "18:00", 0, nlp.CreateReminder, nil},
		{"В пятницу напомни вечером отправить отчёт", "Отправить отчёт", "2026-10-16", "18:00", 0, nlp.CreateReminder, nil},
		{"Напомни завтра утром в 8 купить молоко", "Купить молоко", "2026-10-15", "08:00", 0, nlp.CreateReminder, nil},
		{"Напомни завтра вечером в 8 купить молоко", "Купить молоко", "2026-10-15", "20:00", 0, nlp.CreateReminder, nil},
		{"Напомни сегодня вечером купить молоко", "Купить молоко", "2026-10-14", "18:00", 0, nlp.CreateReminder, nil},
		{"Напомни сегодня утром купить молоко", "Купить молоко", "2026-10-14", "09:00", 0, nlp.CreateReminder, nil},
		{"Напомни сегодня днем купить молоко", "Купить молоко", "2026-10-14", "14:00", 0, nlp.CreateReminder, nil},
		{"Напомни сегодня ночью купить молоко", "Купить молоко", "2026-10-14", "23:00", 0, nlp.CreateReminder, nil},
		{"Напомни завтра утром купить молоко", "Купить молоко", "2026-10-15", "09:00", 0, nlp.CreateReminder, nil},
		{"Напомни завтра днём купить молоко", "Купить молоко", "2026-10-15", "14:00", 0, nlp.CreateReminder, nil},
		{"Напомни завтра вечером купить молоко", "Купить молоко", "2026-10-15", "18:00", 0, nlp.CreateReminder, nil},
		{"Напомни послезавтра в 20 купить молоко", "Купить молоко", "2026-10-16", "20:00", 0, nlp.CreateReminder, nil},
		{"Напомни в понедельник в 8 утра купить молоко", "Купить молоко", "2026-10-19", "08:00", 0, nlp.CreateReminder, nil},
		{"Напомни во вторник в 8 вечера купить молоко", "Купить молоко", "2026-10-20", "20:00", 0, nlp.CreateReminder, nil},
		{"Напомни в среду в 20:00 купить молоко", "Купить молоко", "2026-10-14", "20:00", 0, nlp.CreateReminder, nil},
		{"Напомни в четверг в 20 30 купить молоко", "Купить молоко", "2026-10-15", "20:30", 0, nlp.CreateReminder, nil},
		{"Напомни в субботу в половину девятого купить молоко", "Купить молоко", "2026-10-17", "08:30", 0, nlp.CreateReminder, nil},
		{"Напомни в воскресенье около 8 купить молоко", "Купить молоко", "2026-10-18", "08:00", 0, nlp.CreateReminder, nil},
		{"Напомни в эту пятницу примерно в 8 купить молоко", "Купить молоко", "2026-10-16", "08:00", 0, nlp.CreateReminder, nil},
		{"Напомни в следующую пятницу в 8 купить молоко", "Купить молоко", "2026-10-23", "08:00", 0, nlp.CreateReminder, nil},
		{"Напомни 15 октября в 8 купить молоко", "Купить молоко", "2026-10-15", "08:00", 0, nlp.CreateReminder, nil},
		{"Напомни 15 октября 2026 в 8 купить молоко", "Купить молоко", "2026-10-15", "08:00", 0, nlp.CreateReminder, nil},
		{"Напомни 15.10 в 8 купить молоко", "Купить молоко", "2026-10-15", "08:00", 0, nlp.CreateReminder, nil},
		{"Напомни 15.10.2026 в 8 купить молоко", "Купить молоко", "2026-10-15", "08:00", 0, nlp.CreateReminder, nil},
		{"Напомни купить молоко", "Купить молоко", "", "", 0, nlp.CreateReminder, dateMissing},
		{"Напомни на следующей неделе купить молоко", "Купить молоко", "", "", 0, nlp.CreateReminder, []nlp.Entity{nlp.DayEntity, nlp.TimeEntity}},
		{"Напомни завтра в 10 объяснить мне что нужно для визы", "Объяснить мне что нужно для визы", "2026-10-15", "10:00", 0, nlp.CreateReminder, nil},
		{"Напомни завтра в 10 узнать что делать дальше", "Узнать что делать дальше", "2026-10-15", "10:00", 0, nlp.CreateReminder, nil},
		{"Напомни завтра в 10 позвонить чтобы согласовать отпуск", "Позвонить чтобы согласовать отпуск", "2026-10-15", "10:00", 0, nlp.CreateReminder, nil},
		{"Напомни завтра в 10 передать привет маме", "Передать привет маме", "2026-10-15", "10:00", 0, nlp.CreateReminder, nil},
		{"Напомни завтра в 10 удалить старые файлы", "Удалить старые файлы", "2026-10-15", "10:00", 0, nlp.CreateReminder, nil},
		{"Напомни завтра в 10 перенести встречу", "Перенести встречу", "2026-10-15", "10:00", 0, nlp.CreateReminder, nil},
		{"Добавь задачу проверить почту", "Проверить почту", "", "", 0, nlp.CreateTask, nil},
		{"Поставь дело проверить почту завтра", "Проверить почту", "2026-10-15", "", 0, nlp.CreateTask, nil},
		{"Создай встречу с Олей завтра в 10", "Встреча с Олей", "2026-10-15", "10:00", 0, nlp.CreateEvent, nil},
		{"Создай событие проверить почту завтра в 10", "Проверить почту", "2026-10-15", "10:00", 0, nlp.CreateEvent, nil},
		{"Создай мероприятие обсуждение релиза завтра в 10", "Обсуждение релиза", "2026-10-15", "10:00", 0, nlp.CreateEvent, nil},
		{"Завтра в 10 нужно проверить почту", "Проверить почту", "2026-10-15", "10:00", 0, nlp.CreateTask, nil},
		{"Напомни завтра в 10 о встрече с Олей", "Встреча с Олей", "2026-10-15", "10:00", 0, nlp.CreateReminder, nil},
		{"Создай встречу с Олей завтра в 10 и напомни за час", "Встреча с Олей", "2026-10-15", "10:00", 0, nlp.CreateEvent, nil},
		{"Добавь задачу проверить почту завтра в 10 и напомни за час", "Проверить почту", "2026-10-15", "10:00", 0, nlp.CreateTask, nil},
	} {
		t.Run(tc.text, func(t *testing.T) {
			checkRussianCommand(t, tc.text, now, tc.intent, tc.title, tc.date, tc.clock, tc.second, tc.missing)
		})
	}
}

func TestRussianOperationIntents(t *testing.T) {
	for _, tc := range []struct {
		text   string
		intent nlp.Intent
		query  string
	}{
		{"Перенеси задачу проверить почту на завтра", nlp.Reschedule, "проверить почту"},
		{"Сдвинь встречу с Олей на завтра в 10", nlp.Reschedule, "Олей"},
		{"Заверши задачу проверить почту", nlp.CompleteTask, "проверить почту"},
		{"Отметь задачу проверить почту как выполненную", nlp.CompleteTask, "проверить почту"},
		{"Удали задачу проверить почту", nlp.DeleteTask, "проверить почту"},
		{"Восстанови задачу проверить почту", nlp.RestoreTask, "проверить почту"},
		{"Верни задачу проверить почту из корзины", nlp.RestoreTask, "проверить почту"},
		{"Что у меня завтра", nlp.QuerySchedule, ""},
		{"Покажи расписание на следующей неделе", nlp.QuerySchedule, ""},
		{"Непонятный текст завтра в 10", nlp.Unknown, ""},
	} {
		t.Run(tc.text, func(t *testing.T) {
			c := nlp.ParseCommand(tc.text, russianNow(t))
			if c.Intent != tc.intent || c.Error != "" {
				t.Fatalf("unexpected intent: %+v", c)
			}
			if tc.query != "" && c.Query != tc.query {
				t.Fatalf("query: %q, want %q", c.Query, tc.query)
			}
		})
	}
}

func TestRussianAmbiguities(t *testing.T) {
	for _, text := range []string{
		"Напомни через 0 минут проверить почту",
		"Напомни через 367 часов проверить почту",
		"Напомни через час через два часа проверить почту",
		"Напомни через час и двадцать минут и через 10 минут проверить почту",
		"Напомни завтра в 25:00 проверить почту",
		"Напомни завтра в 10:99 проверить почту",
		"Напомни 31 февраля в 10 проверить почту",
		"Напомни сегодня завтра в 10 проверить почту",
		"Напомни завтра в 10 и в 12 проверить почту",
		"Не добавляй встречу завтра",
		"Не надо удалить задачу #42",
	} {
		t.Run(text, func(t *testing.T) {
			c := nlp.ParseCommand(text, russianNow(t))
			if !c.NeedsClarification() || c.Error == "" {
				t.Fatalf("ambiguity/negation was accepted: %+v", c)
			}
		})
	}
}

func TestRussianTimezoneAndCalendarUnits(t *testing.T) {
	for _, zone := range []string{"Europe/Moscow", "Asia/Vladivostok", "America/New_York"} {
		t.Run(zone, func(t *testing.T) {
			loc, err := time.LoadLocation(zone)
			if err != nil {
				t.Fatal(err)
			}
			// New York changes its DST offset overnight on this date.
			now := time.Date(2026, 10, 31, 23, 59, 45, 0, loc)
			for _, tc := range []struct {
				text   string
				target time.Time
			}{
				{"Напомни через 30 секунд проверить почту", now.Add(30 * time.Second)},
				{"Напомни через минуту проверить почту", now.Add(time.Minute)},
				{"Напомни через день проверить почту", now.AddDate(0, 0, 1)},
				{"Напомни через неделю проверить почту", now.AddDate(0, 0, 7)},
			} {
				c := nlp.ParseCommand(tc.text, now)
				if c.NeedsClarification() || c.Date == nil || c.Time == nil {
					t.Fatalf("incomplete command: %+v", c)
				}
				at := c.Date.At(*c.Time, loc).Add(time.Duration(c.StartSecond) * time.Second)
				if !at.Equal(tc.target) {
					t.Fatalf("%s: %v, want %v", tc.text, at, tc.target)
				}
			}
		})
	}
}
