package search

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

type semanticFixture struct {
	calls int
	fail  bool
}

func (*semanticFixture) ModelID() string { return "fixture-no-network" }
func (f *semanticFixture) Embed(_ context.Context, texts []string) ([][]float64, error) {
	f.calls++
	if f.fail {
		return nil, errors.New("provider unavailable")
	}
	result := make([][]float64, len(texts))
	for i, text := range texts {
		if strings.Contains(text, "авто") || strings.Contains(text, "СТО") {
			result[i] = []float64{1, 0}
		} else {
			result[i] = []float64{0, 1}
		}
	}
	return result, nil
}

func TestHybridAddsSemanticResultsKeepsMutationSearchLexicalAndIsolatesScope(t *testing.T) {
	e, _ := New()
	f := &semanticFixture{}
	e.EnableHybrid(f, time.Second)
	events := []domain.Event{{ID: 1, Title: "Записаться на СТО"}, {ID: 2, Title: "Позвонить маме"}}
	a := repository.WithUser(t.Context(), "alice")
	prepared := e.Prepare(a, events, "авто")
	got, status := e.SearchContext(prepared, events, "авто", 10)
	if len(got) != 1 || got[0].ID != 1 || status.Mode != "hybrid" {
		t.Fatal(got, status)
	}
	if len(e.Search(events, "авто", 10)) != 0 {
		t.Fatal("mutation selector became semantic")
	}
	e.Prepare(a, events, "авто")
	if f.calls != 1 {
		t.Fatal("cache missed", f.calls)
	}
	b := repository.WithUser(prepared, "bob")
	got, status = e.SearchContext(b, events, "авто", 10)
	if len(got) != 0 || status.Mode != "lexical" {
		t.Fatal("scope leak", got, status)
	}
	e.Prepare(b, events, "авто")
	if f.calls != 2 {
		t.Fatal("cross-user cache shared")
	}
	events[0].Title = "Другой документ"
	got, _ = e.SearchContext(prepared, events, "авто", 10)
	if len(got) != 0 {
		t.Fatal("stale vector selected changed content", got)
	}
}

func TestHybridFailureFallsBackAndDocumentBudgetIsVisible(t *testing.T) {
	e, _ := New()
	f := &semanticFixture{fail: true}
	e.EnableHybrid(f, time.Second)
	events := []domain.Event{{ID: 1, Title: "Купить хлеб"}, {ID: 2, Title: "Записаться на СТО"}}
	ctx := e.Prepare(t.Context(), events, "хлеб")
	got, status := e.SearchContext(ctx, events, "хлеб", 10)
	if len(got) != 1 || got[0].ID != 1 || status.Mode != "lexical" || status.Warning == "" {
		t.Fatal(got, status)
	}
	f.fail = false
	e.hybrid.unavailableUntil = time.Time{}
	e.hybrid.MaxDocuments = 1
	ctx = e.Prepare(t.Context(), events, "авто")
	_, status = e.SearchContext(ctx, events, "авто", 10)
	if status.Warning == "" {
		t.Fatal("silent truncation")
	}
}
