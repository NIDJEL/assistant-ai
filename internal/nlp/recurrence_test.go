package nlp_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/nlp"
)

func TestExtendedRelativePermutations(t *testing.T) {
	now := russianNow(t)
	for _, tc := range []struct {
		phrase       string
		days, months int
		elapsed      time.Duration
		date         string
	}{
		{"через час и 20 минут", 0, 0, 80 * time.Minute, ""},
		{"через 1 час 30 минут", 0, 0, 90 * time.Minute, ""},
		{"через два часа пятнадцать минут", 0, 0, 135 * time.Minute, ""},
		{"через 2 дня и 3 часа", 2, 0, 3 * time.Hour, ""},
		{"через неделю и два дня", 9, 0, 0, ""},
		{"через 1 день 4 часа 30 минут", 1, 0, 270 * time.Minute, ""},
		{"через два часа и пять минут", 0, 0, 125 * time.Minute, ""},
		{"через 2 недели 3 дня 4 часа", 17, 0, 4 * time.Hour, ""},
		{"через месяц", 0, 1, 0, "2026-11-14"},
		{"через 2 месяца", 0, 2, 0, "2026-12-14"},
		{"через два месяца", 0, 2, 0, "2026-12-14"},
		{"через полгода", 0, 6, 0, "2027-04-14"},
		{"через год", 0, 12, 0, "2027-10-14"},
		{"через два года", 0, 24, 0, "2028-10-14"},
		{"через месяц и два дня", 2, 1, 0, "2026-11-16"},
		{"через год и три месяца", 0, 15, 0, "2028-01-14"},
		{"через месяц и час", 0, 1, time.Hour, "2026-11-14"},
		{"через год два дня 15 минут", 2, 12, 15 * time.Minute, "2027-10-16"},
		{"через полгода и неделю", 7, 6, 0, "2027-04-21"},
		{"через полтора часа и 10 минут", 0, 0, 100 * time.Minute, ""},
		{"через 1.5 часа 5 минут", 0, 0, 95 * time.Minute, ""},
		{"через 30 минут и 10 секунд", 0, 0, 30*time.Minute + 10*time.Second, ""},
		{"через три дня два часа одну минуту", 3, 0, 121 * time.Minute, ""},
		{"через две недели и один день", 15, 0, 0, ""},
		{"через 10 часов 50 минут", 0, 0, 650 * time.Minute, ""},
		{"через двадцать минут и пять секунд", 0, 0, 20*time.Minute + 5*time.Second, ""},
		{"через сутки и 2 часа", 1, 0, 2 * time.Hour, ""},
		{"через четыре месяца", 0, 4, 0, "2027-02-14"},
		{"через три года", 0, 36, 0, "2029-10-14"},
		{"через месяц неделю три дня", 10, 1, 0, "2026-11-24"},
		{"через час минуту секунду", 0, 0, time.Hour + time.Minute + time.Second, ""},
		{"через 2 дня 23 часа 45 минут", 2, 0, 23*time.Hour + 45*time.Minute, ""},
	} {
		for _, format := range []string{"Напомни %s проверить духовку", "%s напомни проверить духовку", "Напомни проверить духовку %s"} {
			text := fmt.Sprintf(format, tc.phrase)
			t.Run(text, func(t *testing.T) {
				want := now.AddDate(0, tc.months, tc.days).Add(tc.elapsed)
				if tc.date != "" && want.Format("2006-01-02") != tc.date {
					t.Fatal("bad fixture")
				}
				checkRussianCommand(t, text, now, nlp.CreateReminder, "Проверить духовку", want.Format("2006-01-02"), want.Format("15:04"), want.Second(), nil)
			})
		}
	}
}

func TestStructuredRecurrencePermutations(t *testing.T) {
	now := russianNow(t)
	for _, tc := range []struct {
		phrase, freq, date, clock    string
		interval, second, day, month int
		weekdays                     []time.Weekday
	}{
		{"каждый день в 9", "day", "2026-10-15", "09:00", 1, 0, 0, 0, nil},
		{"каждый день в 09:00", "day", "2026-10-15", "09:00", 1, 0, 0, 0, nil},
		{"каждый день в 18", "day", "2026-10-14", "18:00", 1, 0, 0, 0, nil},
		{"каждый понедельник в 10", "week", "2026-10-19", "10:00", 1, 0, 0, 0, []time.Weekday{time.Monday}},
		{"каждый вторник и четверг в 18:00", "week", "2026-10-15", "18:00", 1, 0, 0, 0, []time.Weekday{time.Tuesday, time.Thursday}},
		{"по будням в 8:30", "week", "2026-10-15", "08:30", 1, 0, 0, 0, []time.Weekday{1, 2, 3, 4, 5}},
		{"по выходным в 11", "week", "2026-10-17", "11:00", 1, 0, 0, 0, []time.Weekday{6, 0}},
		{"каждые 2 часа", "hour", "2026-10-14", "14:30", 2, 17, 0, 0, nil},
		{"каждые два часа", "hour", "2026-10-14", "14:30", 2, 17, 0, 0, nil},
		{"каждые 30 минут", "minute", "2026-10-14", "13:00", 30, 17, 0, 0, nil},
		{"каждые тридцать минут", "minute", "2026-10-14", "13:00", 30, 17, 0, 0, nil},
		{"каждый месяц 15 числа в 9", "month", "2026-10-15", "09:00", 1, 0, 15, 0, nil},
		{"15 числа каждого месяца в 9", "month", "2026-10-15", "09:00", 1, 0, 15, 0, nil},
		{"каждую неделю по пятницам в 18", "week", "2026-10-16", "18:00", 1, 0, 0, 0, []time.Weekday{time.Friday}},
		{"каждый год 15 октября в 9", "year", "2026-10-15", "09:00", 1, 0, 15, 10, nil},
		{"каждую среду в 18", "week", "2026-10-14", "18:00", 1, 0, 0, 0, []time.Weekday{time.Wednesday}},
		{"каждое воскресенье в 11", "week", "2026-10-18", "11:00", 1, 0, 0, 0, []time.Weekday{time.Sunday}},
		{"каждые два дня в 10", "day", "2026-10-16", "10:00", 2, 0, 0, 0, nil},
		{"каждые 2 недели по пятницам в 18", "week", "2026-10-16", "18:00", 2, 0, 0, 0, []time.Weekday{time.Friday}},
		{"каждый месяц 31 числа в 9", "month", "2026-10-31", "09:00", 1, 0, 31, 0, nil},
	} {
		for _, format := range []string{"Напомни %s проверить встречи", "%s напомни проверить встречи", "Напомни проверить встречи %s"} {
			text := fmt.Sprintf(format, tc.phrase)
			t.Run(text, func(t *testing.T) {
				checkRussianCommand(t, text, now, nlp.CreateReminder, "Проверить встречи", tc.date, tc.clock, tc.second, nil)
				r := nlp.Parse(text, now).Recurrence
				if r == nil || r.Frequency != tc.freq || r.Interval != tc.interval || r.Day != tc.day || int(r.Month) != tc.month || !reflect.DeepEqual(r.Weekdays, tc.weekdays) || r.Timezone != "Europe/Moscow" || !r.Anchor.Equal(now) {
					t.Fatalf("wrong rule: %+v", r)
				}
				if err := r.Validate(true); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestRecurringMissingAndConflictingEntities(t *testing.T) {
	now := russianNow(t)
	for _, text := range []string{"каждый понедельник напомни отправить отчет", "15 числа каждого месяца напомни оплатить интернет", "каждый год 15 октября напомни поздравить друга", "напомни каждый день пить воду"} {
		t.Run(text, func(t *testing.T) {
			c := nlp.ParseCommand(text, now)
			if c.Recurrence == nil || c.Intent != nlp.CreateReminder || !reflect.DeepEqual(c.Missing, []nlp.Entity{nlp.TimeEntity}) || c.Error != "" {
				t.Fatal(c)
			}
		})
	}
	for _, text := range []string{"напомни каждый день и по выходным в 9 пить воду", "напомни каждые 0 часов размяться", "напомни каждый месяц 32 числа в 9 оплатить", "напомни каждый год 31 февраля в 9 оплатить", "напомни каждый день в 9 и в 10 пить воду", "напомни каждый день через час пить воду", "напомни через 1.5 месяца проверить", "напомни через -2 часа проверить", "не напомни каждый день в 9 пить воду", "не создавай задачу каждый день в 9 проверить", "напомни каждые полтора часа размяться", "напомни каждый день завтра в 9 пить воду"} {
		t.Run(text, func(t *testing.T) {
			c := nlp.ParseCommand(text, now)
			if c.Error == "" {
				t.Fatal("ambiguous command accepted", c)
			}
		})
	}
	for _, tc := range []struct {
		text, title string
		intent      nlp.Intent
	}{
		{"напомни каждый день пить воду в 10 утра", "Пить воду", nlp.CreateReminder},
		{"каждый понедельник в 9 напомни проверить задачи", "Проверить задачи", nlp.CreateReminder},
		{"по будням в 8:30 напомни выйти на работу", "Выйти на работу", nlp.CreateReminder},
		{"каждые два часа напомни размяться", "Размяться", nlp.CreateReminder},
		{"создай задачу каждый день в 9 проверить встречи", "Проверить встречи", nlp.CreateTask},
		{"создай встречу каждый день в 9 с командой", "Встреча с командой", nlp.CreateEvent},
		{"Гена, напомни каждый день в 9 пить воду", "Пить воду", nlp.CreateReminder},
	} {
		t.Run(tc.text, func(t *testing.T) {
			c := nlp.ParseCommand(tc.text, now)
			if c.Intent != tc.intent || c.Title != tc.title {
				t.Fatal(c)
			}
			if tc.intent == nlp.CreateEvent && c.Error == "" {
				t.Fatal("unsupported recurring meeting accepted")
			}
		})
	}
	c := nlp.Parse("напомни 15 числа следующего месяца в 9 оплатить", now)
	if c.Date == nil || c.Date.String() != "2026-11-15" || c.Title != "Оплатить" {
		t.Fatal(c)
	}
}
