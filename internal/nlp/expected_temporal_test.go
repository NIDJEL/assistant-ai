package nlp

import (
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

func TestExpectedTemporalSpansAndSafety(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 30, 17, 0, time.UTC)
	for _, tc := range []struct {
		message, slot, raw string
		delay              time.Duration
	}{
		{"А давай-ка через 20 минут", "relative_time", "через 20 минут", 20 * time.Minute},
		{"Ну, пожалуй, через 15 минут", "relative_time_or_date", "через 15 минут", 15 * time.Minute},
		{"Хорошо, минут через десять", "relative_time", "минут через десять", 10 * time.Minute},
		{"Пожалуйста, через пару часов", "relative_time", "через пару часов", 2 * time.Hour},
		{"Ладно, давай часов в пять", "time", "часов в пять", 0},
		{"Ну тогда завтра вечером", "date", "завтра вечером", 0},
		{"Можно в 17:25, пожалуйста", "time", "в 17:25", 0},
	} {
		t.Run(tc.message, func(t *testing.T) {
			raw, ok := ExtractExpectedTemporalSlot(tc.message, tc.slot)
			if !ok || raw != tc.raw {
				t.Fatalf("%q %v", raw, ok)
			}
			cmd, err := ApplyDialogueSlots(domain.Command{Action: domain.ActionCreate, Kind: domain.EventReminder, Title: "Проверить фильтр", Duration: time.Hour}, SlotUpdate{TemporalExpression: &raw}, tc.message, now)
			if err != nil {
				t.Fatal(err)
			}
			if tc.delay > 0 && (cmd.ExactAt == nil || !cmd.ExactAt.Equal(now.Add(tc.delay))) {
				t.Fatal(cmd)
			}
		})
	}
	for _, tc := range []struct{ message, slot string }{
		{"в 17", "title"}, {"в 17", "relative_time"}, {"завтра", "time"}, {"через 9 минут", "delete"},
		{"Не надо через 9 минут", "relative_time"}, {"Создай задачу через 9 минут", "relative_time"},
		{"Через 9 минут или через 18 минут", "relative_time"}, {"Через 9 минут, но не создавай", "relative_time"},
		{"Хорошо, через 9 минут и удали архив", "relative_time"}, {"Цитирую: через 9 минут", "relative_time"},
	} {
		raw, ok := ExtractExpectedTemporalSlot(tc.message, tc.slot)
		if ok {
			_, err := ApplyDialogueSlots(domain.Command{Action: domain.ActionCreate, Duration: time.Hour}, SlotUpdate{TemporalExpression: &raw}, tc.message, now)
			if err == nil {
				t.Errorf("unsafe/incorrect slot accepted: %q -> %q", tc.message, raw)
			}
		}
	}
}
