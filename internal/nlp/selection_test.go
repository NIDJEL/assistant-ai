package nlp_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
)

func TestMutationSelectors(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-10-14T03:12:00+03:00")
	for _, tc := range []struct {
		text, query, start, end, clock, destination, destinationTime string
		all, exact                                                   bool
	}{
		{"Удали пожалуйста все записи на завтра", "", "2026-10-15", "2026-10-16", "", "2026-10-15", "", true, false},
		{"Удали всё на завтра", "", "2026-10-15", "2026-10-16", "", "2026-10-15", "", true, false},
		{"А перенеси её на завтра", "", "", "", "", "2026-10-15", "", false, false},
		{"Удали все задачи на эту неделю", "", "2026-10-12", "2026-10-19", "", "", "", true, false},
		{"Удали дело по отчету завтра в 15:00", "отчету", "2026-10-15", "2026-10-16", "15:00", "2026-10-15", "15:00", false, false},
		{"Удали уведомление позвонить маме завтра", "позвонить маме", "2026-10-15", "2026-10-16", "", "2026-10-15", "", false, false},
		{"Удали завтрашнюю запись «Проверить отчёт за 2026-10-16 в 12:00»", "Проверить отчёт за 2026-10-16 в 12:00", "2026-10-15", "2026-10-16", "", "2026-10-15", "", false, true},
		{"Перенеси встречу с Олей на завтра в 15:00", "Олей", "", "", "", "2026-10-15", "15:00", false, false},
		{"Перенеси встречу с Олей завтра в 15:00", "Олей", "", "", "", "2026-10-15", "15:00", false, false},
		{"Перенеси завтрашнюю встречу с Олей на пятницу в 17:00", "Олей", "2026-10-15", "2026-10-16", "", "2026-10-16", "17:00", false, false},
		{"Перенеси встречу с Олей с завтра в 10:00 на пятницу в 17:00", "Олей", "2026-10-15", "2026-10-16", "10:00", "2026-10-16", "17:00", false, false},
		{"Удали дело «Заказ #42»", "Заказ #42", "", "", "", "", "", false, true},
		{"Удали дело удалить временные файлы", "удалить временные файлы", "", "", "", "", "", false, false},
	} {
		t.Run(tc.text, func(t *testing.T) {
			cmd := nlp.Parse(tc.text, now)
			str := func(d interface{ String() string }) string {
				if d == nil {
					return ""
				}
				return d.String()
			}
			date := func(d *domain.Date) string {
				if d == nil {
					return ""
				}
				return str(d)
			}
			clock := func(d *domain.TimeOfDay) string {
				if d == nil {
					return ""
				}
				return str(d)
			}
			if cmd.Error != "" || cmd.Query != tc.query || cmd.Selector == nil {
				t.Fatalf("unexpected command: %+v", cmd)
			}
			sel := cmd.Selector
			if date(sel.Start) != tc.start || date(sel.End) != tc.end || clock(sel.Time) != tc.clock || date(cmd.Date) != tc.destination || clock(cmd.Time) != tc.destinationTime || sel.All != tc.all || sel.ExactTitle != tc.exact {
				t.Fatalf("wrong scope/destination: %+v selector=%+v", cmd, sel)
			}
			if tc.query == "Заказ #42" && cmd.TargetID != nil {
				t.Fatal("quoted title became an ID")
			}
			raw, err := json.Marshal(cmd)
			if err != nil {
				t.Fatal(err)
			}
			var roundtrip domain.Command
			if err := json.Unmarshal(raw, &roundtrip); err != nil || roundtrip.Selector == nil || date(roundtrip.Selector.Start) != tc.start || roundtrip.Selector.All != tc.all {
				t.Fatalf("selector lost on restart: %s %v", raw, err)
			}
		})
	}
}

func TestSelectionExceptionsAreNeverIgnored(t *testing.T) {
	for _, text := range []string{"Удали все записи на завтра кроме пары", "Удали все задачи, но не отчет", "Удали все напоминания за исключением врача"} {
		if cmd := nlp.Parse(text, time.Now()); cmd.Error == "" {
			t.Fatalf("ignored an exception: %+v", cmd)
		}
	}
}

func TestMutationWithTwoClocksRequiresClarification(t *testing.T) {
	for _, text := range []string{"Удали запись завтра в 15:00 или в 16:00", "Перенеси встречу с Олей на завтра в 15:00 или в 16:00"} {
		if cmd := nlp.Parse(text, time.Now()); cmd.Error == "" {
			t.Fatalf("silently chose a clock: %+v", cmd)
		}
	}
}
