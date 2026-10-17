package nlp

import (
	"encoding/json"
	"os"
	"testing"
)

func TestEvidencePolicyAndOptionalProofs(t *testing.T) {
	for _, tc := range []struct {
		field, op string
		value     any
		want      EvidenceRequirement
	}{
		{"changes.date", "update", "populated", Required},
		{"filters.status", "batch_delete", "done", Required},
		{"filters.status", "batch_delete", "open", Required},
		{"filters.status", "batch_delete", "any", NotApplicable},
		{"filters.exclude.4.kind", "batch_update", "any", NotApplicable},
		{"filters.exclude.4.query", "batch_update", "Иванов", Required},
		{"target.ordinal", "update", 2, Required},
		{"title", "create", "Таймер", OptionalNonBlocking},
		{"schedule.duration_minutes", "create", 60, OptionalNonBlocking},
		{"filters.status", "search", "done", OptionalNonBlocking},
		{"changes.title", "update", nil, NotApplicable},
		{"changes.time", "search", "populated", NotApplicable},
	} {
		if got := EvidencePolicy(tc.field, tc.op, tc.value); got != tc.want {
			t.Fatalf("%+v: %s", tc, got)
		}
	}
	for _, op := range []string{"create", "list", "search"} {
		p := SemanticPlan{Status: "ok", Operation: planStr(op), Title: planStr("Таймер"), Evidence: []PlanEvidence{
			{Field: "title", Text: "несуществующая цитата"}, {Field: "title", Text: "дубликат"},
			{Field: "changes.date", Text: "послезавтра"}, {Field: "filters.status", Text: "ВЫПОЛНЕННЫЕ"},
		}}
		if err := ValidatePlanEvidence(p, "Засеки 13 минут"); err != nil {
			t.Fatal(op, err)
		}
	}
	if path := os.Getenv("GENA_POLICY_TEST_REPORT"); path != "" {
		out := map[string]any{}
		for _, op := range []string{"create", "search", "update", "batch_update", "batch_delete"} {
			counts := map[EvidenceRequirement]int{Required: 0, OptionalNonBlocking: 0, NotApplicable: 0}
			for _, r := range EvidencePolicyInventory(op) {
				counts[r]++
			}
			out[op] = map[string]any{"counts": counts, "fields": EvidencePolicyInventory(op)}
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSearchCanonicalizationPreservesMeaning(t *testing.T) {
	p := SemanticPlan{Status: "ok", DialogueAct: "command", Operation: planStr("search"), Query: planStr("осмотр"), Title: planStr("  "), Changes: &PlanChanges{}, Schedule: &PlanSchedule{}}
	raw, _ := json.Marshal(p)
	got, err := DecodeSemanticPlan(raw)
	if err != nil || got.Title != nil || got.Changes != nil || got.Schedule != nil {
		t.Fatal(got, err)
	}
	p.Title = planStr("осмотр")
	raw, _ = json.Marshal(p)
	if _, err = DecodeSemanticPlan(raw); err == nil {
		t.Fatal("nonempty incompatible title repaired")
	}
	p.Title = nil
	p.Changes.Time = &ClockMeaning{Hour: 17}
	raw, _ = json.Marshal(p)
	if _, err = DecodeSemanticPlan(raw); err == nil {
		t.Fatal("meaningful update discarded")
	}
	p.Changes = nil
	p.Schedule = nil
	p.Kind = planStr("task")
	raw, _ = json.Marshal(p)
	got, err = DecodeSemanticPlan(raw)
	if err != nil || semanticString(got.Kind) != "task" {
		t.Fatal("kind guessed", got, err)
	}
}

func TestConstraintProofMayIncludeOtherFieldWords(t *testing.T) {
	for _, tc := range []struct{ field, value, proof string }{
		{"status", "done", "все выполненные задачи"},
		{"status", "open", "все открытые встречи"},
		{"priority", "high", "все важные задачи"},
	} {
		p := SemanticPlan{Status: "ok", DialogueAct: "command", Operation: planStr("batch_delete"), Kind: planStr("any"), Filters: &PlanFilters{},
			Evidence: []PlanEvidence{{Field: "batch.scope", Text: tc.proof}, {Field: "filters." + tc.field, Text: tc.proof}}}
		if tc.field == "status" {
			p.Filters.Status = planStr(tc.value)
		} else {
			p.Filters.Priority = planStr(tc.value)
		}
		if err := ValidatePlanEvidence(p, "Удали "+tc.proof); err != nil {
			t.Fatal(tc, err)
		}
		if tc.field == "status" {
			bad := "done"
			if tc.value == bad {
				bad = "open"
			}
			p.Filters.Status = &bad
		} else {
			p.Filters.Priority = planStr("low")
		}
		if ValidatePlanEvidence(p, "Удали "+tc.proof) == nil {
			t.Fatal("wrong enum passed", tc)
		}
	}
}
