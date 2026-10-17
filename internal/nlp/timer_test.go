package nlp

import (
	"testing"
	"time"
)

func TestTimerIntervalsAreNotMeetingDurations(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 30, 17, 0, time.UTC)
	for _, tc := range []struct {
		text, interval string
		minutes        int
	}{
		{"Поставь таймер на 10 минут чтобы я вытащил пиццу из духовки", "на 10 минут", 10},
		{"Засеки 5 минут проверить чай", "5 минут", 5},
		{"Поставь таймер минут на 12", "минут на 12", 12},
		{"Таймер на полчаса", "на полчаса", 30},
		{"На 17 минут поставь таймер проверить бойлер", "на 17 минут", 17},
		{"23 минуты таймер на вентиляцию", "23 минуты", 23},
		{"Засеки минут одиннадцать и проверь насос", "минут одиннадцать", 11},
		{"Гена, на пару часов включи таймер проверить котел", "на пару часов", 120},
	} {
		t.Run(tc.text, func(t *testing.T) {
			s := SemanticCommand{Status: "ok", Action: semanticTestString("create"), Kind: semanticTestString("reminder"), Title: semanticTestString("Проверить"), RelativeTimeExpression: &tc.interval}
			c, err := MapSemantic(s, tc.text, now)
			if err != nil || c.ExactAt == nil || !c.ExactAt.Equal(now.Add(time.Duration(tc.minutes)*time.Minute)) {
				t.Fatal(c, err)
			}
		})
	}
	text := "Создай встречу на 30 минут завтра в 10"
	s := SemanticCommand{Status: "ok", Action: semanticTestString("create"), Kind: semanticTestString("meeting"), Title: semanticTestString("Встреча"), DateExpression: semanticTestString("завтра"), TimeExpression: semanticTestString("в 10"), DurationExpression: semanticTestString("на 30 минут")}
	c, err := MapSemantic(s, text, now)
	if err != nil || c.Duration != 30*time.Minute || c.ExactAt != nil || c.Time.String() != "10:00" {
		t.Fatal(c, err)
	}
	bad := s
	bad.DurationExpression = nil
	bad.RelativeTimeExpression = semanticTestString("на 30 минут")
	if _, err := MapSemantic(bad, text, now); err == nil {
		t.Fatal("meeting duration accepted as countdown")
	}
}
func semanticTestString(s string) *string { return &s }
