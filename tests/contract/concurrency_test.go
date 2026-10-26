package contract

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/search"
	"github.com/NIDJEL/assistant-ai/internal/storage/sqlite"
	"github.com/google/uuid"
)

func services(t *testing.T) (*sqlite.Database, *calendar.Service, time.Time) {
	t.Helper()
	db, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	engine, err := search.New()
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Europe/Moscow")
	now := time.Date(2026, 10, 12, 12, 30, 0, 0, loc)
	return db, &calendar.Service{Store: db, Search: engine, Location: loc, DemoPath: filepath.Join(root(), "data/voice_assistant_events_1000.csv")}, now
}
func TestConcurrentConflictAndDuplicateRequests(t *testing.T) {
	db, cal, now := services(t)
	service := assistant.New(db, nlp.Hybrid{}, cal)
	var wg sync.WaitGroup
	results := make(chan domain.Action, 24)
	errs := make(chan error, 24)
	for i := range 24 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, err := service.Respond(context.Background(), fmt.Sprintf("Добавь встречу с Иваном завтра в 15:00"), uuid.NewString(), uuid.NewString(), now)
			if err != nil {
				errs <- err
			} else {
				results <- r.Action
			}
		}(i)
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	created, clarified := 0, 0
	for action := range results {
		switch action {
		case domain.ActionCreate:
			created++
		case domain.ActionClarify:
			clarified++
		}
	}
	if created != 1 || clarified != 23 {
		t.Fatalf("created=%d clarified=%d", created, clarified)
	}
	sid, rid := uuid.NewString(), uuid.NewString()
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := service.Respond(context.Background(), "Поставь задачу купить билеты", sid, rid, now); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	count, err := cal.Count(context.Background())
	if err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}
func TestLLMHTTPDoesNotHoldSQLiteWriteTransaction(t *testing.T) {
	db, cal, now := services(t)
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_ = json.NewEncoder(w).Encode(domain.Command{Action: domain.ActionCreate, Kind: domain.EventTask, Title: "Проверить почту", Duration: time.Hour})
	}))
	defer server.Close()
	adapter := httpInterpreter(func(ctx context.Context, _ string, _ time.Time) (domain.Command, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		if err != nil {
			return domain.Command{}, err
		}
		response, err := server.Client().Do(req)
		if err != nil {
			return domain.Command{}, err
		}
		defer response.Body.Close()
		var command domain.Command
		err = json.NewDecoder(response.Body).Decode(&command)
		return command, err
	})
	service := assistant.New(db, nlp.Hybrid{Fallback: adapter}, cal)
	done := make(chan error, 1)
	go func() {
		_, err := service.Respond(context.Background(), "Помоги организовать почту", uuid.NewString(), uuid.NewString(), now)
		done <- err
	}()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	err := db.Write(ctx, func(tx repository.Tx) error {
		_, err := tx.Create(ctx, domain.Event{Kind: domain.EventTask, Title: "Параллельная задача", Duration: time.Hour, Status: domain.StatusOpen, Priority: domain.PriorityNormal}, now)
		return err
	})
	cancel()
	close(release)
	if err != nil {
		t.Fatalf("LLM HTTP held the SQLite writer: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
func TestCSVValidationIsAtomic(t *testing.T) {
	_, cal, now := services(t)
	for _, body := range []string{"id,date,time,text\n1,2026-10-13,10:00,Задача\n2,2026-02-31,10:00,Ошибка\n", "id,date,time,text\n1,2026-10-13,10:00,\n", "id,date,time,text\n1,2026-10-13,10:00,Задача\n1,2026-10-13,10:00,Дубль\n"} {
		if _, err := cal.Import(context.Background(), strings.NewReader(body), "invalid.csv", now); err == nil {
			t.Fatal("invalid CSV accepted")
		}
		n, err := cal.Count(context.Background())
		if err != nil || n != 0 {
			t.Fatalf("partial CSV write: %d %v", n, err)
		}
	}
}

type httpInterpreter func(context.Context, string, time.Time) (domain.Command, error)

func (f httpInterpreter) Interpret(ctx context.Context, text string, now time.Time) (domain.Command, error) {
	return f(ctx, text, now)
}
