package contract

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/search"
)

type scopedOperations struct{}

func (scopedOperations) Interpret(context.Context, string, time.Time) (domain.Command, error) {
	return domain.Command{}, errors.New("legacy called")
}
func (scopedOperations) InterpretOperation(_ context.Context, text string, _ nlu.Context) (nlu.Operation, error) {
	raw := `{"intent":"create_task","arguments":{"title":"Общая тема"},"confidence":"high","clarification":null}`
	if text == "удали все задачи" {
		raw = `{"intent":"batch_delete","arguments":{"filter":{"kind":"task"}},"confidence":"high","clarification":null}`
	}
	return nlu.Decode([]byte(raw))
}
func TestSimpleLLMPostgresUserIsolationAndDurableConfirmation(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required; disposable loopback *_test database only")
	}
	u, err := url.Parse(dsn)
	if err != nil || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback() || !strings.Contains(u.Path, "_test") {
		t.Fatal("requires disposable loopback *_test database")
	}
	db, authService, _, now := pgFixture(t)
	one, err := authService.Register(t.Context(), "Первый", "simple-one@example.test", "test password 12345", "test password 12345")
	if err != nil {
		t.Fatal(err)
	}
	two, err := authService.Register(t.Context(), "Второй", "simple-two@example.test", "test password 12345", "test password 12345")
	if err != nil {
		t.Fatal(err)
	}
	a, b := repository.WithUser(t.Context(), one.ID), repository.WithUser(t.Context(), two.ID)
	engine, _ := search.New()
	cal := &calendar.Service{Store: db, Search: engine, Location: now.Location()}
	s := assistant.New(db, scopedOperations{}, cal)
	for _, ctx := range []context.Context{a, b} {
		r, err := s.Respond(ctx, "создай задачу", "same-session", "same-request", now)
		if err != nil || r.Action != domain.ActionCreate {
			t.Fatal(r, err)
		}
	}
	r, err := s.Respond(a, "удали все задачи", "same-session", "preview", now)
	if err != nil || !r.Pending || len(r.Events) != 1 {
		t.Fatal(r, err)
	}
	s = assistant.New(db, scopedOperations{}, cal)
	r, err = s.Respond(a, "да", "same-session", "confirm", now.Add(time.Second))
	if err != nil || r.Action != domain.ActionDelete {
		t.Fatal(r, err)
	}
	for _, check := range []struct {
		ctx   context.Context
		count int
	}{{a, 0}, {b, 1}} {
		if err := db.Read(check.ctx, func(tx repository.Tx) error {
			events, err := tx.Events(check.ctx, domain.Filter{})
			if len(events) != check.count {
				t.Fatal("cross-user mutation", events)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
}
