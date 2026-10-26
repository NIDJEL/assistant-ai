package assistant

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
)

func TestTimerOptionalEvidenceCannotRejectCreate(t *testing.T) {
	for _, tc := range []struct {
		text    string
		minutes int
	}{
		{"Запусти таймер на 10 минут", 10}, {"Поставь таймер минут на пять", 5},
		{"Засеки 13 минут", 13}, {"Таймер на полчаса", 30},
	} {
		s, p, now := planService(t)
		v := simplePlan("create")
		v.Kind = pointer("reminder")
		v.Title = pointer("Таймер")
		v.Schedule = &nlp.PlanSchedule{Delay: &nlp.TemporalMeaning{Type: "relative_delay", Unit: pointer("minute"), Value: integer(tc.minutes)}}
		v.Evidence = []nlp.PlanEvidence{evidence("title", "Таймер"), evidence("schedule.delay", "не из исходного текста")}
		p.plans[tc.text] = v
		r := say(t, s, tc.text, now)
		if r.Action != domain.ActionCreate || len(r.Events) != 1 {
			t.Fatal(tc, r)
		}
		at := now.Add(time.Duration(tc.minutes) * time.Minute)
		if r.Events[0].Date.String() != at.Format("2006-01-02") || r.Events[0].StartTime.String() != at.Format("15:04") {
			t.Fatal(r)
		}
	}
}

func TestSafeClarificationKeepsSearchResults(t *testing.T) {
	s, p, now := planService(t)
	seedPlanEvent(t, s, "Осмотр", "2026-10-18", "09:00", domain.EventTask, now)
	seedPlanEvent(t, s, "Осмотр", "2026-10-18", "12:00", domain.EventMeeting, now)
	v := simplePlan("search")
	v.Query = pointer("Осмотр")
	p.plans["Найди осмотры"] = v
	if r := say(t, s, "Найди осмотры", now); len(r.Events) != 2 {
		t.Fatal(r)
	}
	p.plans["Покажи ту предыдущую"] = nlp.SemanticPlan{Status: "ambiguous", DialogueAct: "command", Clarification: pointer("Какую?")}
	say(t, s, "Покажи ту предыдущую", now.Add(time.Second))
	v = simplePlan("search")
	v.Target = &nlp.PlanTarget{Reference: pointer("ordinal"), Ordinal: integer(2)}
	p.plans["А вторую"] = v
	r := say(t, s, "А вторую", now.Add(2*time.Second))
	if r.Action != domain.ActionSearch || len(r.Events) != 1 || r.Events[0].StartTime.String() != "12:00" {
		t.Fatal(r)
	}
	if p.contexts[2].LastResultCount != 2 || p.contexts[2].Query != "Осмотр" {
		t.Fatal(p.contexts)
	}
}

func TestNeutralAnyNeverNeedsSeparateEvidence(t *testing.T) {
	for _, op := range []string{"batch_delete", "batch_complete"} {
		s, p, now := planService(t)
		seedPlanEvent(t, s, "Заявка", "2026-10-18", "09:00", domain.EventTask, now)
		v := simplePlan(op)
		v.Kind = pointer("task")
		v.Filters = &nlp.PlanFilters{Status: pointer("any")}
		v.Evidence = []nlp.PlanEvidence{evidence("batch.scope", "Все задачи"), evidence("filters.status", "несуществующая необязательная цитата")}
		p.plans["Все задачи обработай"] = v
		if r := say(t, s, "Все задачи обработай", now); !r.Pending || len(r.Events) != 1 {
			t.Fatal(op, r)
		}
		r := say(t, s, "да", now.Add(time.Second))
		if op == "batch_delete" && r.Action != domain.ActionDelete || op == "batch_complete" && r.Action != domain.ActionComplete {
			t.Fatal(r)
		}
	}
}

// Twenty additional attacks include neutral-any proofs, case normalization,
// field substitution and required-proof duplicates. All go through strict JSON,
// the real Service and SQLite, including a later exact confirmation.
func TestPolicyAdversarialNoMutations(t *testing.T) {
	results := []map[string]any{}
	for variant := 0; variant < 4; variant++ {
		for _, cat := range []string{"any_does_not_hide_status", "optional_does_not_cover_change", "case_day_mismatch", "duplicate_required", "exclusion_kind"} {
			t.Run(fmt.Sprintf("%s_%d", cat, variant), func(t *testing.T) {
				s, p, now := planService(t)
				seedPlanEvent(t, s, fmt.Sprintf("Аудит %d", variant), "2026-10-18", "09:00", domain.EventTask, now)
				v := simplePlan("batch_update")
				v.Kind = pointer("task")
				v.Filters = &nlp.PlanFilters{Status: pointer("any")}
				clock := fmt.Sprintf("%02d:15", 18+variant)
				v.Changes = &nlp.PlanChanges{Time: &nlp.ClockMeaning{Hour: 18 + variant, Minute: 15}}
				text := "Все задачи перенеси на " + clock
				v.Evidence = []nlp.PlanEvidence{evidence("batch.scope", "Все задачи"), evidence("changes.time", clock), evidence("filters.status", "выдуманный optional proof")}
				switch cat {
				case "any_does_not_hide_status":
					lex, bad := "выполненные", "open"
					if variant%2 == 1 {
						lex, bad = "открытые", "done"
					}
					text = "Все " + lex + " задачи перенеси на " + clock
					v.Filters.Status = pointer(bad)
					v.Evidence[2] = evidence("filters.status", lex)
				case "optional_does_not_cover_change":
					v.Evidence = v.Evidence[:1]
					v.Evidence = append(v.Evidence, evidence("schedule.time", clock))
				case "case_day_mismatch":
					lex := []string{"послезавтра", "завтра", "вчера", "позавчера"}[variant]
					text = strings.ToUpper("Все задачи перенеси на " + lex)
					v.Changes = &nlp.PlanChanges{Date: relativeDay([]int{1, 2, 0, -1}[variant])}
					v.Evidence[1] = evidence("changes.date", lex)
				case "duplicate_required":
					v.Evidence = append(v.Evidence, evidence("changes.time", clock))
				case "exclusion_kind":
					text = "Все задачи перенеси на " + clock + ", кроме встреч"
					v.Filters.Exclude = []nlp.PlanExclusion{{Kind: pointer("task")}}
					v.Evidence = append(v.Evidence, evidence("filters.exclude.0.kind", "встреч"))
				}
				raw, _ := json.Marshal(v)
				decoded, err := nlp.DecodeSemanticPlan(raw)
				if err != nil {
					t.Fatal(err)
				}
				p.plans[text] = decoded
				before := readAll(t, s)
				var obs DialogueObservation
				s.ObserveTurn = func(o DialogueObservation) { obs = o }
				r := say(t, s, text, now)
				caught := obs
				if r.Action != domain.ActionClarify || r.Pending || caught.EvidenceReason == "" {
					t.Fatal(r, caught)
				}
				say(t, s, "да", now.Add(time.Second))
				if !reflect.DeepEqual(before, readAll(t, s)) {
					t.Fatal("mutation committed")
				}
				results = append(results, map[string]any{"id": t.Name(), "field": caught.EvidenceField, "reason": caught.EvidenceReason, "mutations": 0})
			})
		}
	}
	if len(results) != 20 {
		t.Fatal(len(results))
	}
	if path := os.Getenv("GENA_POLICY_ADVERSARIAL_REPORT"); path != "" {
		b, _ := json.MarshalIndent(map[string]any{"cases": 20, "mutation_count": 0, "results": results}, "", "  ")
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
