package assistant

import (
	"bufio"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
)

func TestOmissionCorpusNoWrongWrites(t *testing.T) {
	f, err := os.Open("../../testdata/semantic/plan_omission_adversarial.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	results := []map[string]any{}
	for scanner.Scan() {
		var row struct {
			ID, Category, Source string
			Plan                 json.RawMessage
		}
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		t.Run(row.ID, func(t *testing.T) {
			plan, err := nlp.DecodeSemanticPlan(row.Plan)
			if err != nil {
				t.Fatalf("omission must pass strict shape and compatibility: %v", err)
			}
			s, p, now := planService(t)
			seedPlanEvent(t, s, "Проверка гироскопа 0", "2026-10-18", "08:10", domain.EventTask, now)
			seedPlanEvent(t, s, "Резерв", "2026-10-18", "09:10", domain.EventTask, now)
			p.plans[row.Source] = plan
			before := readAll(t, s)
			var obs DialogueObservation
			s.ObserveTurn = func(o DialogueObservation) { obs = o }
			r := say(t, s, row.Source, now)
			caught := obs
			if r.Action != domain.ActionClarify || r.Pending || caught.EvidenceReason != "source_constraint_omitted" {
				t.Fatal(r, caught)
			}
			say(t, s, "подтверждаю", now.Add(time.Second))
			if !reflect.DeepEqual(before, readAll(t, s)) {
				t.Fatal("wrong write after omission")
			}
			results = append(results, map[string]any{"id": row.ID, "field": caught.EvidenceField, "reason": caught.EvidenceReason, "json_valid": true, "wrong_writes": 0})
		})
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(results) != 40 {
		t.Fatal(len(results))
	}
	if path := os.Getenv("GENA_OMISSION_REPORT"); path != "" {
		b, _ := json.MarshalIndent(map[string]any{"cases": len(results), "wrong_writes": 0, "results": results}, "", "  ")
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCorrectionContextAndClarificationTTL(t *testing.T) {
	for _, variant := range []string{"fresh", "ambiguous", "expired", "read_only"} {
		t.Run(variant, func(t *testing.T) {
			s, p, now := planService(t)
			v := simplePlan("create")
			v.Kind, v.Title = pointer("task"), pointer("Стенд")
			v.Schedule = &nlp.PlanSchedule{Date: relativeDay(1), Time: &nlp.ClockMeaning{Hour: 14}}
			p.plans["Запиши задачу Стенд на завтра в 14:00"] = v
			say(t, s, "Запиши задачу Стенд на завтра в 14:00", now)
			if variant == "ambiguous" || variant == "read_only" {
				if variant == "ambiguous" {
					seedPlanEvent(t, s, "Второй стенд", "2026-10-18", "12:00", domain.EventTask, now)
				}
				p.plans["Список задач"] = simplePlan("list")
				say(t, s, "Список задач", now)
			}
			for i := 0; i < 6; i++ {
				p.plans["Не могу выбрать из тех записей"] = nlp.SemanticPlan{Status: "ambiguous", DialogueAct: "command"}
				say(t, s, "Не могу выбрать из тех записей", now.Add(time.Duration(i)*time.Second))
			}
			v = simplePlan("update")
			v.DialogueAct = "correction"
			v.Changes = &nlp.PlanChanges{Time: &nlp.ClockMeaning{Hour: 17, Minute: 46}}
			p.plans["Поправка: время 17:46"] = v
			at := now.Add(10 * time.Second)
			if variant == "expired" {
				at = now.Add(referenceTTL + time.Second)
			}
			before := readAll(t, s)
			r := say(t, s, "Поправка: время 17:46", at)
			if variant == "fresh" {
				if r.Action != domain.ActionUpdate || len(r.Events) != 1 || r.Events[0].StartTime.String() != "17:46" {
					t.Fatal(r)
				}
			} else if r.Action != domain.ActionClarify || !reflect.DeepEqual(before, readAll(t, s)) {
				t.Fatal(r)
			}
		})
	}
}

func TestSemanticCanonicalizerAndSourceAnchors(t *testing.T) {
	s, p, now := planService(t)
	v := simplePlan("create")
	v.Kind, v.Title = pointer("reminder"), pointer("Таймер")
	v.Schedule = &nlp.PlanSchedule{DurationMinutes: integer(26)}
	p.plans["Таймер длительностью 26 минут"] = v
	r := say(t, s, "Таймер длительностью 26 минут", now)
	if r.Action != domain.ActionCreate || r.Events[0].StartTime.String() != "12:56" {
		t.Fatal(r)
	}
	v = simplePlan("complete")
	v.Target = &nlp.PlanTarget{Reference: pointer("last")}
	v.Changes = &nlp.PlanChanges{Status: pointer("done")}
	v.Evidence = []nlp.PlanEvidence{{Field: "target.reference", Text: "его"}}
	p.plans["Отметь его выполненным"] = v
	if r = say(t, s, "Отметь его выполненным", now); r.Action != domain.ActionComplete {
		t.Fatal(r)
	}
}

func TestSearchAndAffectedTargetHaveIndependentState(t *testing.T) {
	s, p, now := planService(t)
	seedPlanEvent(t, s, "Поверка", "2026-10-18", "08:20", domain.EventTask, now)
	seedPlanEvent(t, s, "Поверка", "2026-10-18", "12:20", domain.EventMeeting, now)
	v := simplePlan("search")
	v.Query = pointer("Поверка")
	v.Filters = &nlp.PlanFilters{Period: period("month", 0)}
	p.plans["Найди поверки текущего месяца"] = v
	say(t, s, "Найди поверки текущего месяца", now)
	v = simplePlan("update")
	v.Target = &nlp.PlanTarget{Reference: pointer("ordinal"), Ordinal: integer(2)}
	v.Changes = &nlp.PlanChanges{Time: &nlp.ClockMeaning{Hour: 16, Minute: 24}}
	p.plans["Вторую поставь на 16:24"] = v
	if r := say(t, s, "Вторую поставь на 16:24", now); r.Action != domain.ActionUpdate {
		t.Fatal(r)
	}
	_, state, err := s.History(t.Context(), "dialogue")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.LastIDs) != 2 || len(state.Dialogue.AffectedIDs) != 1 || state.Dialogue.Query != "Поверка" {
		t.Fatal(state)
	}
	v = simplePlan("update")
	v.DialogueAct = "correction"
	v.Changes = &nlp.PlanChanges{Time: &nlp.ClockMeaning{Hour: 17, Minute: 24}}
	p.plans["Поправка: 17:24"] = v
	if r := say(t, s, "Поправка: 17:24", now); r.Action != domain.ActionUpdate || r.Events[0].Kind != domain.EventMeeting {
		t.Fatal(r)
	}
	v = simplePlan("search")
	v.Target = &nlp.PlanTarget{Reference: pointer("last")}
	p.plans["Покажи её"] = v
	if r := say(t, s, "Покажи её", now); r.Action != domain.ActionSearch || len(r.Events) != 1 || r.Events[0].Kind != domain.EventMeeting {
		t.Fatal(r)
	}
	v = simplePlan("search")
	v.DialogueAct = "refinement"
	v.Kind = pointer("meeting")
	p.plans["В выборке только встречи"] = v
	if r := say(t, s, "В выборке только встречи", now); r.Action != domain.ActionSearch || len(r.Events) != 1 {
		t.Fatal(r)
	}
}

func TestDeferredPreviewSurvivesReloadAndExpires(t *testing.T) {
	for _, expire := range []bool{false, true} {
		s, p, now := planService(t)
		seedPlanEvent(t, s, "Архив", "2026-10-18", "08:20", domain.EventTask, now)
		v := simplePlan("batch_delete")
		v.Kind = pointer("task")
		v.Evidence = []nlp.PlanEvidence{{Field: "batch.scope", Text: "Все задачи"}}
		p.plans["Все задачи убери"] = v
		if r := say(t, s, "Все задачи убери", now); !r.Pending {
			t.Fatal(r)
		}
		_, before, _ := s.History(t.Context(), "dialogue")
		if r := say(t, s, "Подтверждение отложи, ещё проверяю", now.Add(time.Second)); !r.Pending {
			t.Fatal(r)
		}
		_, after, _ := s.History(t.Context(), "dialogue")
		if after.Pending.ConfirmationState != "deferred_confirmation" || !reflect.DeepEqual(before.Pending.Candidates, after.Pending.Candidates) || !after.Pending.At.Equal(before.Pending.At) {
			t.Fatal(after)
		}
		reloaded := New(s.Store, p, s.Calendar)
		at := now.Add(2 * time.Second)
		if expire {
			at = now.Add(pendingTTL + time.Second)
		}
		r := say(t, reloaded, "удаляй", at)
		if expire {
			if r.Reply != "Сейчас нечего подтверждать." || len(readAll(t, s)) != 1 {
				t.Fatal(r)
			}
		} else if r.Action != domain.ActionDelete || len(readAll(t, s)) != 0 {
			t.Fatal(r)
		}
	}
}

func TestRejectedPlansPreserveResultContext(t *testing.T) {
	for _, failure := range []string{"invalid", "evidence", "coverage"} {
		t.Run(failure, func(t *testing.T) {
			s, p, now := planService(t)
			seedPlanEvent(t, s, "Геодезия", "2026-10-18", "08:20", domain.EventTask, now)
			seedPlanEvent(t, s, "Геодезия", "2026-10-18", "12:20", domain.EventTask, now)
			v := simplePlan("search")
			v.Query = pointer("Геодезия")
			p.plans["Отыщи геодезию"] = v
			say(t, s, "Отыщи геодезию", now)
			_, before, _ := s.History(t.Context(), "dialogue")
			text := "Вторую измени на 18:26"
			v = simplePlan("update")
			v.Target = &nlp.PlanTarget{Reference: pointer("ordinal"), Ordinal: integer(2)}
			v.Changes = &nlp.PlanChanges{Time: &nlp.ClockMeaning{Hour: 18, Minute: 26}}
			switch failure {
			case "invalid":
				v.Target = nil
			case "evidence":
				v.Evidence = []nlp.PlanEvidence{{Field: "changes.time", Text: "не из этого сообщения"}}
			case "coverage":
				text += " и на послезавтра"
			}
			p.plans[text] = v
			for i := 0; i < 6; i++ {
				if r := say(t, s, text, now.Add(time.Duration(i+1)*time.Second)); r.Action != domain.ActionClarify {
					t.Fatal(r)
				}
			}
			_, after, _ := s.History(t.Context(), "dialogue")
			if !reflect.DeepEqual(before.LastIDs, after.LastIDs) || !reflect.DeepEqual(before.Dialogue.Search, after.Dialogue.Search) || !before.Dialogue.ResultAt.Equal(after.Dialogue.ResultAt) {
				t.Fatal("safe failure lost context", after)
			}
		})
	}
}
