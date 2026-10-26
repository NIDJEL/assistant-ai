package assistant

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

func evidence(field, text string) nlp.PlanEvidence { return nlp.PlanEvidence{Field: field, Text: text} }
func relativeDay(n int) *nlp.TemporalMeaning {
	return &nlp.TemporalMeaning{Type: "relative_day", Offset: integer(n)}
}

func TestEvidenceRejectionRevokesOlderPreview(t *testing.T) {
	s, p, now := planService(t)
	seedPlanEvent(t, s, "Запись", "2026-10-18", "09:00", domain.EventMeeting, now)
	v := simplePlan("batch_update")
	v.Kind = pointer("meeting")
	v.Changes = &nlp.PlanChanges{Time: &nlp.ClockMeaning{Hour: 18}}
	v.Evidence = []nlp.PlanEvidence{evidence("batch.scope", "Все встречи"), evidence("changes.time", "18:00")}
	p.plans["Все встречи перенеси на 18:00"] = v
	if r := say(t, s, "Все встречи перенеси на 18:00", now); !r.Pending {
		t.Fatal(r)
	}
	v.Changes = &nlp.PlanChanges{Date: relativeDay(1)}
	v.Evidence[1] = evidence("changes.date", "послезавтра")
	p.plans["Все встречи перенеси на послезавтра"] = v
	r := say(t, s, "Все встречи перенеси на послезавтра", now.Add(time.Second))
	if r.Pending {
		t.Fatal(r)
	}
	r = say(t, s, "да", now.Add(2*time.Second))
	if r.Action == domain.ActionUpdate || readAll(t, s)[0].StartTime.String() != "09:00" {
		t.Fatal(r)
	}
}

func TestUnverifiedPersistedPreviewCannotBeConfirmed(t *testing.T) {
	s, p, now := planService(t)
	seedPlanEvent(t, s, "Запись", "2026-10-18", "09:00", domain.EventMeeting, now)
	v := simplePlan("batch_delete")
	v.Kind = pointer("meeting")
	v.Evidence = []nlp.PlanEvidence{evidence("batch.scope", "Все встречи")}
	p.plans["Все встречи удали"] = v
	if r := say(t, s, "Все встречи удали", now); !r.Pending {
		t.Fatal(r)
	}
	if err := s.Store.Write(t.Context(), func(tx repository.Tx) error {
		st, err := tx.State(t.Context(), "dialogue")
		if err != nil {
			return err
		}
		st.Pending.EvidenceVersion = 0
		return tx.SaveState(t.Context(), "dialogue", st, now)
	}); err != nil {
		t.Fatal(err)
	}
	// A harmless model clarification must not bless an old unverified snapshot.
	p.plans["Расскажи подробнее"] = nlp.SemanticPlan{Status: "ambiguous", DialogueAct: "command", Clarification: pointer("Что уточнить?")}
	say(t, s, "Расскажи подробнее", now.Add(time.Millisecond))
	r := say(t, s, "да", now.Add(time.Second))
	if r.Pending || r.Action != domain.ActionClarify || len(readAll(t, s)) != 1 {
		t.Fatal(r)
	}
}

// These plans pass the actual strict JSON decoder. They are deliberately wrong
// semantically. Exercise the service, preview, persisted state and a later yes.
func TestAdversarialSemanticEvidenceNoMutations(t *testing.T) {
	type result struct {
		ID, Message, Field, Reason string
		JSONValid                  bool
		MutationCount              int
	}
	var results []result
	for variant := 0; variant < 6; variant++ {
		for _, category := range []string{"day_destination", "day_filter", "ordinal", "shift", "exclusion", "delay", "clock", "weekday"} {
			t.Run(fmt.Sprintf("%s_%d", category, variant), func(t *testing.T) {
				s, p, now := planService(t)
				seedPlanEvent(t, s, "Встреча с Иваном", "2026-10-18", "09:00", domain.EventMeeting, now)
				seedPlanEvent(t, s, "Встреча с Петром", "2026-10-18", "12:00", domain.EventMeeting, now)
				v := simplePlan("batch_update")
				v.Kind = pointer("meeting")
				v.Changes = &nlp.PlanChanges{Time: &nlp.ClockMeaning{Hour: 18}}
				text := "Все встречи перенеси на 18:00"
				v.Evidence = []nlp.PlanEvidence{evidence("batch.scope", "Все встречи"), evidence("changes.time", "18:00")}
				switch category {
				case "day_destination":
					lex := []string{"послезавтра", "завтра", "позавчера", "вчера", "сегодня", "послезавтра"}[variant]
					bad := []int{1, 2, -1, 0, 1, 3}[variant]
					text = "Все встречи перенеси на " + lex
					v.Changes = &nlp.PlanChanges{Date: relativeDay(bad)}
					v.Evidence[1] = evidence("changes.date", lex)
				case "day_filter":
					text = "Все встречи послезавтра перенеси на 18:00"
					v.Filters = &nlp.PlanFilters{Period: relativeDay(1 + variant%2*2)}
					v.Evidence = append(v.Evidence, evidence("filters.period", "послезавтра"))
				case "ordinal":
					find := simplePlan("list")
					p.plans["Покажи встречи"] = find
					say(t, s, "Покажи встречи", now.Add(-time.Second))
					text = "Вторая встреча: перенеси на 18:00"
					v.Operation = pointer("update")
					v.Target = &nlp.PlanTarget{Reference: pointer("ordinal"), Ordinal: integer(1)}
					v.Evidence = []nlp.PlanEvidence{evidence("target.reference", "Вторая"), evidence("changes.time", "18:00")}
				case "shift":
					text = "Все встречи сдвинь на два часа"
					v.Changes = &nlp.PlanChanges{TimeShiftMinutes: integer(60 + variant)}
					v.Evidence[1] = evidence("changes.time_shift_minutes", "на два часа")
				case "exclusion":
					text = "Все встречи перенеси на 18:00, кроме встречи с Иваном"
					v.Filters = &nlp.PlanFilters{Exclude: []nlp.PlanExclusion{{Query: pointer("Пётр")}}}
					v.Evidence = append(v.Evidence, evidence("filters.exclude.0.query", "Иваном"))
				case "delay":
					text = fmt.Sprintf("Все встречи перенеси через %d минут", 20+variant)
					v.Changes = &nlp.PlanChanges{Delay: &nlp.TemporalMeaning{Type: "relative_delay", Unit: pointer("minute"), Value: integer(21 + variant)}}
					v.Evidence[1] = evidence("changes.delay", fmt.Sprintf("через %d минут", 20+variant))
				case "clock":
					v.Changes.Time.Hour = 17 - variant
				case "weekday":
					text = "Все встречи перенеси на пятницу"
					v.Changes = &nlp.PlanChanges{Date: &nlp.TemporalMeaning{Type: "weekday", Weekday: pointer("thursday")}}
					v.Evidence[1] = evidence("changes.date", "пятницу")
				}
				raw, _ := json.Marshal(v)
				decoded, err := nlp.DecodeSemanticPlan(raw)
				if err != nil {
					t.Fatalf("adversary must be valid JSON plan: %v", err)
				}
				p.plans[text] = decoded
				before := readAll(t, s)
				var observation DialogueObservation
				s.ObserveTurn = func(o DialogueObservation) { observation = o }
				r := say(t, s, text, now)
				caught := observation
				if r.Action != domain.ActionClarify || r.Pending || caught.EvidenceReason == "" {
					t.Fatalf("bad plan reached preview: %+v %+v", r, caught)
				}
				say(t, s, "да", now.Add(time.Second))
				after := readAll(t, s)
				mutations := 0
				if !reflect.DeepEqual(before, after) {
					mutations = 1
					t.Fatal("adversarial mutation committed")
				}
				results = append(results, result{t.Name(), text, caught.EvidenceField, caught.EvidenceReason, true, mutations})
			})
		}
	}
	if len(results) != 48 {
		t.Fatal(len(results))
	}
	if path := os.Getenv("GENA_EVIDENCE_TEST_REPORT"); path != "" {
		b, _ := json.MarshalIndent(map[string]any{"cases": 48, "mutation_count": 0, "results": results}, "", "  ")
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEvidenceRequiredBeforePreview(t *testing.T) {
	for _, missing := range []string{"all", "changes.time", "batch.scope", "not_in_original", "embedded_word", "missing_exclusion"} {
		t.Run(missing, func(t *testing.T) {
			s, p, now := planService(t)
			seedPlanEvent(t, s, "Запись", "2026-10-18", "09:00", domain.EventMeeting, now)
			v := simplePlan("batch_update")
			v.Kind = pointer("meeting")
			v.Changes = &nlp.PlanChanges{Time: &nlp.ClockMeaning{Hour: 18}}
			text := "Все встречи послезавтра перенеси на 18:00"
			v.Evidence = []nlp.PlanEvidence{evidence("batch.scope", "Все встречи"), evidence("changes.time", "18:00")}
			switch missing {
			case "all":
				v.Evidence = nil
			case "changes.time":
				v.Evidence = v.Evidence[:1]
			case "batch.scope":
				v.Evidence = v.Evidence[1:]
			case "not_in_original":
				v.Evidence[1].Text = "17:00"
			case "embedded_word":
				v.Filters = &nlp.PlanFilters{Period: relativeDay(1)}
				v.Evidence = append(v.Evidence, evidence("filters.period", "завтра"))
			case "missing_exclusion":
				text += " кроме встречи с Иваном"
			}
			p.plans[text] = v
			before := readAll(t, s)
			r := say(t, s, text, now)
			say(t, s, "да", now.Add(time.Second))
			if r.Pending || !reflect.DeepEqual(before, readAll(t, s)) {
				t.Fatal(r)
			}
		})
	}
}

func TestEvidenceSourceFilterSeparateFromDestination(t *testing.T) {
	for _, tc := range []struct {
		text        string
		changes     *nlp.PlanChanges
		proof       nlp.PlanEvidence
		kind        string
		date, clock string
	}{
		{"Перенеси все встречи послезавтра на день позже", &nlp.PlanChanges{TimeShiftMinutes: integer(1440)}, evidence("changes.time_shift_minutes", "на день позже"), "meeting", "2026-10-19", "09:00"},
		{"все встречи послезавтра перенеси на 18:00", &nlp.PlanChanges{Time: &nlp.ClockMeaning{Hour: 18}}, evidence("changes.time", "18:00"), "meeting", "2026-10-18", "18:00"},
		{"послезавтра все встречи сдвинь на час", &nlp.PlanChanges{TimeShiftMinutes: integer(60)}, evidence("changes.time_shift_minutes", "на час"), "meeting", "2026-10-18", "10:00"},
		{"перенеси задачи с послезавтра на пятницу", &nlp.PlanChanges{Date: &nlp.TemporalMeaning{Type: "weekday", Weekday: pointer("friday")}}, evidence("changes.date", "пятницу"), "task", "2026-10-16", "09:00"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			s, p, now := planService(t)
			kind, _ := domain.ParseEventType(tc.kind)
			seedPlanEvent(t, s, "Нужная", "2026-10-18", "09:00", kind, now)
			seedPlanEvent(t, s, "Другая", "2026-10-17", "12:00", kind, now)
			v := simplePlan("batch_update")
			v.Kind = pointer(tc.kind)
			v.Filters = &nlp.PlanFilters{Period: relativeDay(2)}
			v.Changes = tc.changes
			scope := "все встречи"
			if tc.kind == "task" {
				scope = "задачи"
			}
			v.Evidence = []nlp.PlanEvidence{evidence("batch.scope", scope), evidence("filters.period", "послезавтра"), tc.proof}
			p.plans[tc.text] = v
			r := say(t, s, tc.text, now)
			if !r.Pending || r.Total == nil || *r.Total != 1 {
				t.Fatal(r)
			}
			r = say(t, s, "да", now.Add(time.Second))
			if r.Action != domain.ActionUpdate {
				t.Fatal(r)
			}
			items := readAll(t, s)
			for _, e := range items {
				if e.Title == "Нужная" && (e.Date.String() != tc.date || e.StartTime.String() != tc.clock) {
					t.Fatal(e)
				}
				if e.Title == "Другая" && e.Date.String() != "2026-10-17" {
					t.Fatal(e)
				}
			}
		})
	}
}
