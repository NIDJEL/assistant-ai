package contract

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/llm"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/search"
)

type multiScopedOperations struct{ raw string }

func (*multiScopedOperations) Interpret(context.Context, string, time.Time) (domain.Command, error) {
	return domain.Command{}, errors.New("legacy called")
}
func (p *multiScopedOperations) InterpretOperation(context.Context, string, nlu.Context) (nlu.Operation, error) {
	return nlu.Decode([]byte(p.raw))
}

func TestMultiActionPostgresDurabilityAndUserIsolation(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("disposable TEST_DATABASE_URL required")
	}
	db, authService, _, now := pgFixture(t)
	first, err := authService.Register(t.Context(), "Первый", "multi-one@example.test", "test password 12345", "test password 12345")
	if err != nil {
		t.Fatal(err)
	}
	second, err := authService.Register(t.Context(), "Второй", "multi-two@example.test", "test password 12345", "test password 12345")
	if err != nil {
		t.Fatal(err)
	}
	p := &multiScopedOperations{raw: `{"intent":"multi_action","confidence":"high","arguments":{"steps":[{"key":"meeting","source":"Встреча с Колей","operation":{"intent":"create_meeting","confidence":"high","arguments":{"title":"Встреча с Колей","friends":["Коля"],"date":{"type":"relative_day","offset":1}}}},{"key":"task","source":"купить продукты","operation":{"intent":"create_task","confidence":"high","arguments":{"title":"Купить продукты"}}}]}}`}
	engine, _ := search.New()
	cal := &calendar.Service{Store: db, Search: engine, Location: now.Location()}
	s := assistant.New(db, p, cal)
	a, b := repository.WithUser(t.Context(), first.ID), repository.WithUser(t.Context(), second.ID)
	for _, ctx := range []context.Context{a, b} {
		r, err := s.Respond(ctx, "Встреча с Колей; купить продукты", "multi-session", "initial", now)
		if err != nil || !r.Pending || len(r.Events) != 1 {
			t.Fatal(r, err)
		}
	}
	s = assistant.New(db, p, cal)
	p.raw = `{"intent":"continue","confidence":"high","arguments":{"changes":{"time":"20:00"}}}`
	r, err := s.Respond(a, "20:00", "multi-session", "time", now.Add(time.Second))
	if err != nil || r.Pending || len(r.Events) != 1 || len(r.Operations) != 2 {
		t.Fatal(r, err)
	}
	for _, check := range []struct {
		ctx   context.Context
		count int
	}{{a, 2}, {b, 1}} {
		if err := db.Read(check.ctx, func(tx repository.Tx) error {
			events, err := tx.Events(check.ctx, domain.Filter{})
			if len(events) != check.count {
				t.Fatalf("cross-user effects: %+v", events)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// Explicit opt-in: real provider, disposable PostgreSQL, no production accounts.
func TestMultiActionLivePostgres(t *testing.T) {
	if os.Getenv("GENA_TEST_MULTI_LIVE") != "1" || os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("GENA_TEST_MULTI_LIVE=1 and disposable TEST_DATABASE_URL required")
	}
	db, authService, _, now := pgFixture(t)
	user, err := authService.Register(t.Context(), "Проверка", "multi-live@example.test", "test password 12345", "test password 12345")
	if err != nil {
		t.Fatal(err)
	}
	ctx := repository.WithUser(t.Context(), user.ID)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	g, err := llm.NewGigaChat(cfg.GigaChat)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	engine, _ := search.New()
	s := assistant.New(db, &llm.Simple{Provider: g}, &calendar.Service{Store: db, Search: engine, Location: now.Location()})
	message := "Создай встречу с Колей завтра в 20:00, добавь задачу купить продукты и напомни завтра в 22:00 позвонить маме"
	r, err := s.Respond(ctx, message, "live-multi", "initial", now)
	data, _ := json.Marshal(r)
	t.Log(string(data))
	if err != nil || r.Pending || len(r.Events) != 3 || len(r.Operations) != 3 {
		t.Fatal(r, err)
	}
}

func TestMultiActionAmbiguousLivePostgres(t *testing.T) {
	if os.Getenv("GENA_TEST_MULTI_LIVE") != "1" || os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("GENA_TEST_MULTI_LIVE=1 and disposable TEST_DATABASE_URL required")
	}
	db, authService, _, clock := pgFixture(t)
	now := time.Date(2026, 10, 26, 4, 53, 0, 0, clock.Location())
	user, err := authService.Register(t.Context(), "Проверка", "ambiguous-live@example.test", "test password 12345", "test password 12345")
	if err != nil {
		t.Fatal(err)
	}
	ctx := repository.WithUser(t.Context(), user.ID)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	g, err := llm.NewGigaChat(cfg.GigaChat)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	engine, _ := search.New()
	s := assistant.New(db, &llm.Simple{Provider: g}, &calendar.Service{Store: db, Search: engine, Location: now.Location()})
	turn := 0
	send := func(text string) (domain.Response, []domain.Event) {
		t.Helper()
		turn++
		r, err := s.Respond(ctx, text, "ambiguous-live", strconv.Itoa(turn), now.Add(time.Duration(turn)*time.Second))
		data, _ := json.Marshal(r)
		t.Log(string(data))
		if err != nil || len(r.Operations) != 3 {
			t.Fatal(r, err)
		}
		var events []domain.Event
		if err := db.Read(ctx, func(tx repository.Tx) error {
			var err error
			events, err = tx.Events(ctx, domain.Filter{})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return r, events
	}
	r, events := send("Создай встречу с Колей в 8, добавь задачу купить продукты и напомни позвонить маме в 10")
	if !r.Pending || len(events) != 1 || events[0].Kind != domain.EventTask {
		t.Fatal(events)
	}
	r, events = send("20:00")
	// Providers can use today's civil day or ask for it. Either path must
	// preserve the other steps and cannot guess the morning/evening clock.
	if r.Operations[0].Status == "waiting" {
		r, events = send("сегодня")
	}
	if len(events) != 2 {
		t.Fatal(events)
	}
	r, events = send("10:00")
	if r.Pending && r.Operations[2].Status == "waiting" {
		r, events = send("сегодня")
	}
	if r.Pending || len(events) != 3 {
		t.Fatal(events)
	}
	for _, e := range events {
		if e.Kind == domain.EventMeeting && e.StartTime.String() != "20:00" || e.Kind == domain.EventReminder && e.StartTime.String() != "10:00" {
			t.Fatal(e)
		}
	}
}

func TestMultiActionDependencyLivePostgres(t *testing.T) {
	if os.Getenv("GENA_TEST_MULTI_LIVE") != "1" || os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("GENA_TEST_MULTI_LIVE=1 and disposable TEST_DATABASE_URL required")
	}
	db, authService, _, now := pgFixture(t)
	user, err := authService.Register(t.Context(), "Проверка", "dependency-live@example.test", "test password 12345", "test password 12345")
	if err != nil {
		t.Fatal(err)
	}
	ctx := repository.WithUser(t.Context(), user.ID)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	g, err := llm.NewGigaChat(cfg.GigaChat)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	engine, _ := search.New()
	s := assistant.New(db, &llm.Simple{Provider: g}, &calendar.Service{Store: db, Search: engine, Location: now.Location()})
	r, err := s.Respond(ctx, "Создай задачу Отчёт, затем отметь именно её выполненной; независимо добавь задачу Купить хлеб", "dependency-live", "initial", now)
	data, _ := json.Marshal(r)
	t.Log(string(data))
	if err != nil || r.Pending || len(r.Operations) != 3 {
		t.Fatal(r, err)
	}
	if err := db.Read(ctx, func(tx repository.Tx) error {
		events, err := tx.Events(ctx, domain.Filter{})
		if len(events) != 2 {
			t.Fatal(events)
		}
		for _, e := range events {
			if (e.Title == "Отчёт") != (e.Status == domain.StatusDone) {
				t.Fatal(e)
			}
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	r, err = s.Respond(ctx, "Добавь задачу Альфа; добавь задачу Бета; добавь задачу Гамма; добавь задачу Дельта; добавь задачу Эпсилон", "dependency-live", "five-actions", now.Add(time.Second))
	data, _ = json.Marshal(r)
	t.Log(string(data))
	if err != nil || r.Pending || len(r.Operations) != 5 || len(r.Events) != 5 {
		t.Fatal(r, err)
	}
}
