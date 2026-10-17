package nlp

import (
	"encoding/json"
	"testing"
)

func TestSourceCoverageAndNegatedAlternatives(t *testing.T) {
	base := SemanticPlan{Status: "ok", DialogueAct: "correction", Operation: planStr("update"), Changes: &PlanChanges{Date: &TemporalMeaning{Type: "relative_day", Offset: planInt(2)}}}
	for _, source := range []string{"Исправление: послезавтра, не завтра", "Меняю дату на послезавтра, не на завтра"} {
		if err := base.Validate(); err != nil {
			t.Fatal(err)
		}
		if err := ValidatePlanEvidence(base, source); err != nil {
			t.Fatal(source, err)
		}
	}
	base.Evidence = []PlanEvidence{{Field: "changes.date", Text: "послезавтра"}, {Field: "changes.date", Text: "не на завтра"}}
	if err := ValidatePlanEvidence(base, "Меняю дату на послезавтра, не на завтра"); err != nil {
		t.Fatal("negative alternative counted as duplicate support", err)
	}
	base.Evidence = nil
	base.Changes.Date.Offset = planInt(1)
	if ValidatePlanEvidence(base, "Меняю дату на послезавтра, не на завтра") == nil {
		t.Fatal("negated alternative authorized")
	}
	base.Changes.Date = nil
	base.Changes.Title = planStr("Отчёт")
	if ValidatePlanEvidence(base, "Название Отчёт, дату на сегодня") == nil {
		t.Fatal("omitted date allowed")
	}
}

func TestGoSourceAnchorsDoNotRepairBadEvidence(t *testing.T) {
	p := SemanticPlan{Status: "ok", DialogueAct: "correction", Operation: planStr("update"), Changes: &PlanChanges{Time: &ClockMeaning{Hour: 19, Minute: 12}}}
	text := "Исправляю начало: 19:12"
	if err := ValidatePlanEvidence(p, text); err != nil {
		t.Fatal(err)
	}
	p.Evidence = []PlanEvidence{{Field: "changes.time", Text: "18:12"}}
	if ValidatePlanEvidence(p, text) == nil {
		t.Fatal("fabricated proof ignored")
	}
	p.Evidence = []PlanEvidence{{Field: "changes.time", Text: "19:12"}, {Field: "changes.time", Text: "19:12"}}
	if ValidatePlanEvidence(p, text) == nil {
		t.Fatal("duplicate required proof ignored")
	}
}

func TestCanonicalizerIsIdempotentAndPreservesConflicts(t *testing.T) {
	correction := SemanticPlan{Status: "ok", DialogueAct: "correction", Operation: planStr("update"), Target: &PlanTarget{Reference: planStr("last")}, Changes: &PlanChanges{Time: &ClockMeaning{Hour: 12, Minute: 18}}}
	if c, n := CanonicalizePlanSource(correction, "Исправление времени"); n != 1 || c.Target != nil || c.Validate() != nil {
		t.Fatal(c, n)
	}
	correction.Target = &PlanTarget{Reference: planStr("ordinal"), Ordinal: planInt(2)}
	if CanonicalizePlan(correction).Target == nil {
		t.Fatal("explicit target discarded")
	}
	p := SemanticPlan{Status: "ok", DialogueAct: "command", Operation: planStr("complete"), Target: &PlanTarget{Reference: planStr("last")}, Changes: &PlanChanges{Status: planStr("done")}}
	c, n := CanonicalizePlanSource(p, "Заверши её")
	if n != 1 || c.Changes != nil || p.Changes == nil {
		t.Fatal(c, n)
	}
	if _, n = CanonicalizePlanSource(c, "Заверши её"); n != 0 {
		t.Fatal("not idempotent")
	}
	p.Changes.Status = planStr("open")
	if CanonicalizePlan(p).Validate() == nil {
		t.Fatal("conflicting status repaired")
	}
	p.Changes.Status = planStr("done")
	p.Changes.Title = planStr("Новое")
	if CanonicalizePlan(p).Validate() == nil {
		t.Fatal("additional change discarded")
	}
	p = SemanticPlan{Status: "ok", DialogueAct: "command", Operation: planStr("create"), Kind: planStr("meeting"), Title: planStr("Лекция"), Schedule: &PlanSchedule{DurationMinutes: planInt(30)}}
	c, n = CanonicalizePlanSource(p, "Встреча длительностью полчаса")
	if n != 0 || c.Schedule.DurationMinutes == nil || c.Schedule.Delay != nil {
		t.Fatal("meeting duration became countdown")
	}
}

func TestQuotedContentIsNotOmittedSchedule(t *testing.T) {
	p := SemanticPlan{Status: "ok", DialogueAct: "command", Operation: planStr("create"), Kind: planStr("task"), Title: planStr("Обсудить фильм Сегодня в 12:00")}
	if err := ValidatePlanEvidence(p, `Создай задачу «Обсудить фильм Сегодня в 12:00»`); err != nil {
		t.Fatal(err)
	}
}

func TestTimerCanonicalizationPrecedesCompatibility(t *testing.T) {
	p := SemanticPlan{Status: "ok", DialogueAct: "command", Operation: planStr("create"), Kind: planStr("reminder"), Title: planStr("Таймер"), Schedule: &PlanSchedule{DurationMinutes: planInt(2)}}
	raw, _ := json.Marshal(p)
	c, err := DecodeSemanticPlanSource(raw, "Таймер длительностью 2 минуты")
	if err != nil || c.Schedule.Delay == nil || *c.Schedule.Delay.Value != 2 || c.Schedule.DurationMinutes != nil {
		t.Fatal(c, err)
	}
	if _, err = DecodeSemanticPlanSource(raw, "Встреча длительностью 2 минуты"); err == nil {
		t.Fatal("missing timer marker accepted")
	}
}

func TestSourceAnchorRetainsOrdinalGrammarContext(t *testing.T) {
	p := SemanticPlan{Status: "ok", DialogueAct: "command", Operation: planStr("update"), Target: &PlanTarget{Reference: planStr("ordinal"), Ordinal: planInt(2)}, Changes: &PlanChanges{Time: &ClockMeaning{Hour: 17, Minute: 22}}, Evidence: []PlanEvidence{{Field: "target.reference", Text: "второй"}}}
	if err := ValidatePlanEvidence(p, "У второй записи время 17:22"); err != nil {
		t.Fatal(err)
	}
	if ValidatePlanEvidence(p, "Второй час: время 17:22") == nil {
		t.Fatal("temporal ordinal selected record")
	}
	p.Target.Ordinal = planInt(1)
	if ValidatePlanEvidence(p, "У второй записи время 17:22") == nil {
		t.Fatal("wrong ordinal selected record")
	}
}
