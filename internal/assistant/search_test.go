package assistant

import (
	"context"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"testing"
	"time"
)

type observedStore struct {
	repository.Store
	writing bool
}

func (s *observedStore) Write(ctx context.Context, fn func(repository.Tx) error) error {
	return s.Store.Write(ctx, func(tx repository.Tx) error { s.writing = true; defer func() { s.writing = false }(); return fn(tx) })
}

type filteredQueryModel struct {
	t     *testing.T
	store *observedStore
	calls int
	id    int64
}

func (*filteredQueryModel) ModelID() string { return "filtered-test-cloud" }
func (m *filteredQueryModel) Rewrite(context.Context, string) ([]string, error) {
	m.calls++
	if m.store.writing {
		m.t.Error("external search called inside write transaction")
	}
	return []string{"СТО"}, nil
}
func (m *filteredQueryModel) Rerank(_ context.Context, _ string, candidates []domain.Event) ([]int, error) {
	m.calls++
	if m.store.writing {
		m.t.Error("external search called inside write transaction")
	}
	if len(candidates) != 1 || candidates[0].ID != m.id {
		m.t.Error("date/status/kind filters applied after external candidate selection", candidates)
	}
	return []int{1}, nil
}
func TestCloudReadUsesExactFiltersOutsideWritesAndLeavesMutationsDeterministic(t *testing.T) {
	s, p, now := simpleService(t)
	store := &observedStore{Store: s.Store}
	s.Store = store
	s.Calendar.Store = store
	day := domain.Day(now).AddDays(1)
	for i := 0; i < 4; i++ {
		e := domain.Event{Kind: domain.EventTask, Title: "Записаться на СТО", Date: day, Duration: time.Hour, Status: domain.StatusOpen, Priority: domain.PriorityNormal}
		if i == 1 {
			e.Kind = domain.EventMeeting
			e.StartTime = &domain.TimeOfDay{Hour: 10}
		}
		if i == 2 {
			e.Status = domain.StatusDone
		}
		if i == 3 {
			e.Date = domain.Day(now)
		}
		created, err := s.Calendar.Create(t.Context(), e, now, true)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			s.Calendar.Search.EnableCloud(&filteredQueryModel{t: t, store: store, id: created.ID}, time.Second)
		}
	}
	model := &filteredQueryModel{t: t, store: store, id: 1}
	s.Calendar.Search.EnableCloud(model, time.Second)
	operationFixture(p, "search", `{"query":"техобслуживание машины","kind":"task","date":{"type":"relative_day","offset":1},"status":"open"}`)
	r := say(t, s, "Найди завтрашние открытые задачи по техобслуживанию машины", now)
	if r.Search == nil || r.Search.Mode != "cloud" || len(r.Events) != 1 || model.calls != 2 {
		t.Fatal(r, model.calls)
	}
	operationFixture(p, "update", `{"target":{"type":"title","value":"техобслуживание машины"},"changes":{"title":"Другая задача"}}`)
	_ = say(t, s, "Переименуй техобслуживание машины", now.Add(time.Second))
	if model.calls != 2 {
		t.Fatal("mutation invoked semantic search")
	}
	operationFixture(p, "create_task", `{"title":"Купить хлеб"}`)
	_ = say(t, s, "Купить хлеб", now.Add(2*time.Second))
	if model.calls != 2 {
		t.Fatal("ordinary create invoked semantic search")
	}
}
