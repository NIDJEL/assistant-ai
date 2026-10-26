package search

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

type queryFixture struct {
	rewrites, ranks int
	fail, slow      bool
	indices         []int
}

func (*queryFixture) ModelID() string { return "test-cloud" }
func (f *queryFixture) Rewrite(ctx context.Context, _ string) ([]string, error) {
	f.rewrites++
	if f.slow {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.fail {
		return nil, errors.New("offline")
	}
	return []string{"СТО"}, nil
}
func (f *queryFixture) Rerank(_ context.Context, _ string, _ []domain.Event) ([]int, error) {
	f.ranks++
	if f.indices != nil {
		return f.indices, nil
	}
	return []int{1}, nil
}

func TestCloudPoolVersionsFiltersAndScope(t *testing.T) {
	e, _ := New()
	f := &queryFixture{}
	e.EnableCloud(f, time.Second)
	a := repository.WithUser(t.Context(), "alice")
	x := []domain.Event{{ID: 1, Title: "Записаться на СТО"}}
	y := []domain.Event{{ID: 2, Title: "СТО в пятницу"}}
	ctx := e.Prepare(a, x, "техобслуживание машины")
	ctx = e.Prepare(ctx, y, "техобслуживание машины")
	for _, pool := range [][]domain.Event{x, y} {
		got, status := e.SearchContext(ctx, pool, "техобслуживание машины", 10)
		if len(got) != 1 || got[0].ID != pool[0].ID || status.Mode != "cloud" {
			t.Fatal(got, status)
		}
	}
	if f.rewrites != 1 || f.ranks != 2 {
		t.Fatal("rewrite cache or filtered ranks", f)
	}
	ctx = e.Prepare(ctx, x, "техобслуживание машины")
	if f.ranks != 2 {
		t.Fatal("same snapshot prepared twice")
	}
	got, status := e.SearchContext(repository.WithUser(ctx, "bob"), x, "техобслуживание машины", 10)
	if len(got) != 0 || status.Mode != "lexical" {
		t.Fatal("cross-user ranking", got, status)
	}
	x[0].Title = "Другой документ"
	got, status = e.SearchContext(ctx, x, "техобслуживание машины", 10)
	if len(got) != 0 || status.Mode != "lexical" || status.Warning == "" {
		t.Fatal("stale snapshot", got, status)
	}
	if len(e.Search(y, "техобслуживание машины", 10)) != 0 {
		t.Fatal("mutation selector became semantic")
	}
}

func TestCloudBoundedFailureAndInvalidRanking(t *testing.T) {
	for _, tc := range []struct {
		name    string
		fixture queryFixture
	}{
		{"unavailable", queryFixture{fail: true}}, {"timeout", queryFixture{slow: true}},
		{"invented index", queryFixture{indices: []int{999}}}, {"duplicate index", queryFixture{indices: []int{1, 1}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, _ := New()
			f := tc.fixture
			e.EnableCloud(&f, time.Millisecond)
			events := []domain.Event{{ID: 1, Title: "СТО"}}
			start := time.Now()
			ctx := e.Prepare(t.Context(), events, "СТО")
			got, status := e.SearchContext(ctx, events, "СТО", 10)
			if len(got) != 1 || status.Warning == "" || time.Since(start) > time.Second {
				t.Fatal(got, status)
			}
		})
	}
}

func TestCloudEmptyPoolDoesNotCallProvider(t *testing.T) {
	e, _ := New()
	f := &queryFixture{}
	e.EnableCloud(f, time.Second)
	ctx := e.Prepare(t.Context(), nil, "СТО")
	got, status := e.SearchContext(ctx, nil, "СТО", 10)
	if len(got) != 0 || status.Mode != "cloud" || f.rewrites+f.ranks != 0 {
		t.Fatal(got, status, f)
	}
}

func TestCloudCatalogueRetainsRareTopicsAmongRepeatedCommonTitles(t *testing.T) {
	e, _ := New()
	events := []domain.Event{}
	for i := 0; i < 1000; i++ {
		events = append(events, domain.Event{ID: int64(i + 1), Title: fmt.Sprintf("Заказать бумагу: этап %d", i+1)})
	}
	rare := "Записаться на вакцинацию питомца"
	events = append(events, domain.Event{ID: 1001, Title: rare})
	if !slices.Contains(e.searchVocabulary(events), rare) {
		t.Fatal("repeated common topics crowded a distinct rare topic out of the catalogue")
	}
}
