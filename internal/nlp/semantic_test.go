package nlp_test

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
)

func TestSemanticMandatoryRegressionMapping(t *testing.T) {
	file, err := os.Open("../../testdata/semantic/structured_regressions.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	loc, _ := time.LoadLocation("Europe/Moscow")
	now := time.Date(2026, 10, 15, 12, 30, 17, 0, loc)
	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var tc struct {
			ID, Text string
			Semantic nlp.SemanticCommand `json:"semantic_expected"`
			Expected struct {
				Action       string
				Kind         *string
				Title, Query string
				Date, Time   *string
				PeriodStart  *string `json:"period_start"`
				PeriodEnd    *string `json:"period_end"`
				Second       int     `json:"start_second"`
				Selector     *domain.EventSelector
			}
		}
		if err := json.Unmarshal(scanner.Bytes(), &tc); err != nil {
			t.Fatal(err)
		}
		t.Run(tc.ID, func(t *testing.T) {
			c, err := nlp.MapSemantic(tc.Semantic, tc.Text, now)
			if err != nil {
				t.Fatal(err)
			}
			if c.Action.String() != tc.Expected.Action || c.Title != tc.Expected.Title || c.Query != tc.Expected.Query || c.StartSecond != tc.Expected.Second || c.Error != "" {
				t.Fatalf("unexpected semantic mapping: %+v", c)
			}
			if tc.Expected.Kind != nil && c.Kind.String() != *tc.Expected.Kind {
				t.Fatalf("kind: %+v", c)
			}
			if tc.Expected.Date != nil && (c.Date == nil || c.Date.String() != *tc.Expected.Date) {
				t.Fatalf("date: %+v", c)
			}
			if tc.Expected.Time != nil && (c.Time == nil || c.Time.String() != *tc.Expected.Time) {
				t.Fatalf("time: %+v", c)
			}
			if tc.Expected.PeriodStart != nil && (c.PeriodStart == nil || c.PeriodStart.String() != *tc.Expected.PeriodStart) {
				t.Fatalf("period: %+v", c)
			}
			if tc.Expected.Selector != nil {
				a, _ := json.Marshal(c.Selector)
				b, _ := json.Marshal(tc.Expected.Selector)
				if string(a) != string(b) {
					t.Fatalf("target scope changed: got %s want %s", a, b)
				}
			}
		})
		count++
	}
	if scanner.Err() != nil || count != 13 {
		t.Fatalf("invalid corpus: count=%d err=%v", count, scanner.Err())
	}
}

func TestSemanticRejectsInventionsAndMixedRoles(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Moscow")
	now := time.Date(2026, 10, 15, 12, 30, 17, 0, loc)
	ptr := func(s string) *string { return &s }
	for _, s := range []nlp.SemanticCommand{
		{Status: "ok", Action: ptr("create"), Kind: ptr("task"), Title: ptr("Проверить почту"), DateExpression: ptr("2026-10-16")},
		{Status: "ok", Action: ptr("update"), Target: &nlp.SemanticTarget{Query: ptr("Отчет")}, Changes: &nlp.SemanticChanges{Title: ptr("")}},
		{Status: "ok", Action: ptr("delete"), Target: &nlp.SemanticTarget{Query: ptr("Отчет")}, DateExpression: ptr("завтра")},
		{Status: "ok", Action: ptr("update"), Target: &nlp.SemanticTarget{Query: ptr("Отчет"), All: true}, Changes: &nlp.SemanticChanges{DateExpression: ptr("завтра")}},
		{Status: "ambiguous", Action: ptr("delete"), Target: &nlp.SemanticTarget{All: true}},
	} {
		if _, err := nlp.MapSemantic(s, "Добавь задачу проверить почту завтра", now); err == nil {
			t.Fatalf("invalid semantic accepted: %+v", s)
		}
	}
}

func TestSemanticMalformedStrictJSON(t *testing.T) {
	for _, raw := range []string{
		`null`, `{}`, `[]`, `{"status":"ok"}`, `{"status":"unknown","status":"ok"}`,
		`{"action":"delete","target":{"target_id":42}}`,
	} {
		if _, err := nlp.DecodeSemantic([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestSemanticRecurrenceAndTimezoneRemainDeterministic(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Berlin")
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, loc)
	p := func(s string) *string { return &s }
	s := nlp.SemanticCommand{Status: "ok", Action: p("create"), Kind: p("reminder"), Title: p("Пить воду"), TimeExpression: p("в 9"), RecurrenceExpression: p("каждый день")}
	c, err := nlp.MapSemantic(s, "Напомни каждый день в 9 пить воду", now)
	if err != nil || c.Recurrence == nil || c.Recurrence.Timezone != "Europe/Berlin" {
		t.Fatal(c, err)
	}
	next, err := c.Recurrence.Next(now)
	if err != nil || next.In(loc).Hour() != 9 || !strings.Contains(next.In(loc).Format(time.RFC3339), "+01:00") {
		t.Fatal(next, err)
	}
}

func TestSemanticTemporalGrammarAndOriginalConflictValidation(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Moscow")
	now := time.Date(2026, 10, 15, 12, 30, 17, 0, loc)
	p := func(s string) *string { return &s }
	for _, tc := range []struct{ expr, clock string }{
		{"в полдень", "12:00"}, {"в полночь", "00:00"}, {"на одиннадцать", "11:00"},
	} {
		s := nlp.SemanticCommand{Status: "ok", Action: p("create"), Kind: p("meeting"), Title: p("Обсуждение"), DateExpression: p("завтра"), TimeExpression: p(tc.expr)}
		c, err := nlp.MapSemantic(s, "Добавь встречу Обсуждение завтра "+tc.expr, now)
		if err != nil || c.Time == nil || c.Time.String() != tc.clock {
			t.Fatal(c, err)
		}
	}
	for _, text := range []string{"Напомни сегодня завтра в 10 проверить почту", "Напомни через час и через два часа проверить почту"} {
		s := nlp.SemanticCommand{Status: "ok", Action: p("create"), Kind: p("reminder"), Title: p("Проверить почту")}
		c, err := nlp.MapSemantic(s, text, now)
		if err != nil || c.Error == "" {
			t.Fatalf("conflicting source was accepted: %+v %v", c, err)
		}
	}
}
