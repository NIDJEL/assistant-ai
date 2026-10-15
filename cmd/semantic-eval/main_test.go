package main

import (
	"testing"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
)

func TestUnsafeMutationUsesTargetScopeAndFailClosedErrors(t *testing.T) {
	task := "task"
	want := expected{Action: "complete", Kind: &task, Query: "отчет", Selector: &domain.EventSelector{}}
	got := actual{Action: "complete", Kind: &task, Query: "договор", Selector: &domain.EventSelector{}}
	if !unsafeMutation(got, want) {
		t.Fatal("wrong target was not unsafe")
	}
	got.Error = "Ничего не изменено"
	if unsafeMutation(got, want) {
		t.Fatal("rejected command counted as a performed mutation")
	}
}

func TestFieldScoresDoNotHideSemanticTargetErrorsBehindMapping(t *testing.T) {
	p := func(s string) *string { return &s }
	gold := nlp.SemanticCommand{Status: "ok", Action: p("update"), Kind: p("meeting"), Target: &nlp.SemanticTarget{Query: p("Встреча с Никитой")}, Changes: &nlp.SemanticChanges{Title: p("Встреча с Геной"), DateExpression: p("завтра")}}
	broken := gold
	broken.Target = &nlp.SemanticTarget{Query: p("Встреча с Геной")}
	kind := "meeting"
	date := "2026-10-16"
	want := expected{Action: "update", Kind: &kind, Title: "Встреча с Геной", Query: "Встреча с Никитой", Date: &date, DurationMinutes: 60, Selector: &domain.EventSelector{}}
	got := actual{Action: "update", Kind: &kind, Title: want.Title, Query: want.Query, Date: &date, DurationMinutes: 60, Selector: &domain.EventSelector{}}
	scores := fieldScores(got, broken, corpusCase{Expected: want, SemanticExpected: &gold}, nil)
	if scores["target_accuracy"] || scores["full_semantic_accuracy"] {
		t.Fatal("Go mapping hid model target error", scores)
	}
	if !scores["accepted_target_accuracy"] || !scores["temporal_extraction_accuracy"] {
		t.Fatal(scores)
	}
}

func TestScoreApplicabilityMatchesCommandFields(t *testing.T) {
	for _, action := range []string{"create", "update", "delete", "complete", "list", "help"} {
		if scoreApplies("query_accuracy", expected{Action: action, Query: "отчет"}, true) {
			t.Fatal("non-search command scored as search query", action)
		}
	}
	if !scoreApplies("query_accuracy", expected{Action: "search", Query: "отчет"}, true) || scoreApplies("temporal_extraction_accuracy", expected{}, false) {
		t.Fatal("incorrect metric denominator")
	}
	if scoreApplies("changes_accuracy", expected{Action: "update"}, false) || !scoreApplies("changes_accuracy", expected{Action: "update"}, true) {
		t.Fatal("changes extraction requires semantic gold, not calculated dates")
	}
}

func TestTemporalExtractionPreservesRolesAndIgnoresNonTemporalFields(t *testing.T) {
	p := func(s string) *string { return &s }
	gold := nlp.SemanticCommand{Target: &nlp.SemanticTarget{Query: p("Отчет")}, Changes: &nlp.SemanticChanges{Title: p("Договор")}}
	if string(semanticTemporalProjection(gold)) != string(semanticTemporalProjection(nlp.SemanticCommand{})) {
		t.Fatal("target presence or rename affected temporal extraction")
	}
	gold.Target.DateExpression = p("завтра")
	wrongRole := nlp.SemanticCommand{Changes: &nlp.SemanticChanges{DateExpression: p("завтра")}}
	if string(semanticTemporalProjection(gold)) == string(semanticTemporalProjection(wrongRole)) {
		t.Fatal("target and changes temporal roles were conflated")
	}
}

func TestSemanticProjectionTreatsTodayWithRelativeIntervalAsRedundant(t *testing.T) {
	p := func(s string) *string { return &s }
	gold := nlp.SemanticCommand{RelativeTimeExpression: p("через 30 секунд")}
	got := gold
	got.DateExpression = p("сегодня")
	if string(semanticProjection(got)) != string(semanticProjection(gold)) {
		t.Fatal("redundant today changed semantic score")
	}
}
