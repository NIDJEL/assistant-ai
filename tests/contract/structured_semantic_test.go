package contract

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/llm"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/search"
)

func structuredRenameModel(t *testing.T) *llm.Semantic {
	t.Helper()
	content := structuredFixture(t, `{"status":"ok","action":"update","kind":"meeting","title":null,"query":null,"target":{"query":"Встреча с Никитой","all":false},"changes":{"title":"Встреча с Геной","date_expression":"завтра"}}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"done": true, "message": map[string]string{"content": content}})
	}))
	t.Cleanup(server.Close)
	provider, err := llm.NewOllamaSemantic(server.URL, "test", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return &llm.Semantic{Provider: provider, CacheDir: t.TempDir()}
}

func exerciseStructuredRename(t *testing.T, ctx context.Context, store repository.Store, cal *calendar.Service, now time.Time) {
	t.Helper()
	model := structuredRenameModel(t)
	service := assistant.New(store, model, cal)
	var one, two *domain.Event
	err := store.Write(ctx, func(tx repository.Tx) error {
		var err error
		one, err = tx.Create(ctx, domain.Event{Kind: domain.EventMeeting, Title: "Встреча с Никитой", Date: domain.Day(now), StartTime: &domain.TimeOfDay{Hour: 9}, Duration: time.Hour, Status: domain.StatusOpen, Priority: domain.PriorityNormal}, now)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := service.Respond(ctx, "Измени на завтра встречу с Никитой на встречу с Геной", "rename", "one", now)
	if err != nil || response.Action != domain.ActionUpdate || len(response.Events) != 1 || response.Events[0].Title != "Встреча с Геной" || response.Events[0].Date.String() != domain.Day(now).AddDays(1).String() || response.Events[0].StartTime.String() != "09:00" {
		t.Fatal(response, err)
	}
	again, err := service.Respond(ctx, "Измени на завтра встречу с Никитой на встречу с Геной", "rename", "one", now)
	if err != nil || len(again.Events) != 1 || again.Events[0].ID != one.ID {
		t.Fatal(again, err)
	}
	err = store.Write(ctx, func(tx repository.Tx) error {
		for range 2 {
			two, err = tx.Create(ctx, domain.Event{Kind: domain.EventMeeting, Title: "Встреча с Никитой", Date: domain.Day(now), StartTime: &domain.TimeOfDay{Hour: 11}, Duration: time.Hour, Status: domain.StatusOpen, Priority: domain.PriorityNormal}, now)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err = service.Respond(ctx, "Измени на завтра встречу с Никитой на встречу с Геной", "ambiguous", "two", now)
	if err != nil || response.Action != domain.ActionClarify || !response.Pending || len(response.Candidates) != 2 {
		t.Fatal(response, err)
	}
	err = store.Read(ctx, func(tx repository.Tx) error {
		e, err := tx.Get(ctx, two.ID)
		if e == nil || e.Title != "Встреча с Никитой" || e.Date.String() != domain.Day(now).String() {
			t.Fatal("ambiguous rename mutated a record", e)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStructuredSemanticAtomicRenameAndAmbiguity(t *testing.T) {
	store, cal, now := services(t)
	exerciseStructuredRename(t, t.Context(), store, cal, now)
}

func TestStructuredSemanticPostgresAtomicRename(t *testing.T) {
	if dsn := os.Getenv("TEST_DATABASE_URL"); dsn != "" {
		u, err := url.Parse(dsn)
		if err != nil || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback() || !strings.Contains(u.Path, "semantic_test") {
			t.Fatal("structured mutation integration requires a separately named loopback semantic_test DB")
		}
	}
	store, authService, _, now := pgFixture(t)
	user, err := authService.Register(t.Context(), "Semantic test", "semantic@example.test", "test-only-password-123", "test-only-password-123")
	if err != nil {
		t.Fatal(err)
	}
	engine, err := search.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx := repository.WithUser(t.Context(), user.ID)
	cal := &calendar.Service{Store: store, Search: engine, Location: now.Location()}
	exerciseStructuredRename(t, ctx, store, cal, now)
}

func TestStructuredSemanticDryRunCorpusDoesNotMutate(t *testing.T) {
	store, cal, now := services(t)
	err := store.Write(t.Context(), func(tx repository.Tx) error {
		for _, title := range []string{"Исходная запись A", "Исходная запись B"} {
			if _, err := tx.Create(t.Context(), domain.Event{Kind: domain.EventTask, Title: title, Duration: time.Hour, Status: domain.StatusOpen, Priority: domain.PriorityNormal}, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := cal.Count(t.Context())
	for _, text := range []string{"Напомни завтра в 10 проверить почту", "Перенеси встречу на завтра", "Удали все задачи", "Отметь отчет выполненным"} {
		p := func(s string) *string { return &s }
		semantic := nlp.SemanticCommand{Status: "ok", Action: p("create"), Kind: p("task"), Title: p("Проверить почту")}
		if _, err := nlp.MapSemantic(semantic, text, now); err != nil {
			t.Fatal(err)
		}
	}
	after, _ := cal.Count(t.Context())
	if before != 2 || after != before {
		t.Fatalf("dry run mutated DB: %d -> %d", before, after)
	}
}

func TestStructuredSemanticDialogueUsesExistingPendingFlow(t *testing.T) {
	store, cal, now := services(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode(map[string]any{"done": true, "message": map[string]string{
			"content": structuredFixture(t, `{"status":"ok","action":"create","kind":"reminder","title":"Проверить аквариум","date_expression":"завтра"}`),
		}})
	}))
	defer server.Close()
	provider, err := llm.NewOllamaSemantic(server.URL, "test", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	model := &llm.Semantic{Provider: provider, CacheDir: t.TempDir()}
	service := assistant.New(store, model, cal)
	first, err := service.Respond(t.Context(), "Напомни завтра проверить аквариум", "dialogue", "one", now)
	if err != nil || first.Action != domain.ActionClarify || !first.Pending {
		t.Fatal(first, err)
	}
	second, err := service.Respond(t.Context(), "в 10", "dialogue", "two", now)
	if err != nil || second.Action != domain.ActionCreate || len(second.Events) != 1 || second.Events[0].StartTime.String() != "10:00" || calls != 1 {
		t.Fatal(second, err, calls)
	}
}

func TestStructuredSemanticOrganisersCSVSearchAndList(t *testing.T) {
	_, cal, now := services(t)
	file, err := os.Open(filepath.Join(root(), "data", "voice_assistant_events_1000.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	result, err := cal.Import(t.Context(), file, "organisers-semantic-readonly.csv", now)
	if err != nil || result.TotalRows != 1000 || result.Imported != 1000 {
		t.Fatal(result, err)
	}
	p := func(s string) *string { return &s }
	for _, tc := range []struct {
		text     string
		semantic nlp.SemanticCommand
		search   bool
	}{
		{"Найди задачу про договор", nlp.SemanticCommand{Status: "ok", Action: p("search"), Kind: p("task"), Query: p("договор")}, true},
		{"Найди задачу про договр", nlp.SemanticCommand{Status: "ok", Action: p("search"), Kind: p("task"), Query: p("договр")}, true},
		{"Покажи записи за 2024 год", nlp.SemanticCommand{Status: "ok", Action: p("list"), PeriodExpression: p("за 2024 год")}, false},
	} {
		cmd, err := nlp.MapSemantic(tc.semantic, tc.text, now)
		if err != nil {
			t.Fatal(err)
		}
		list, err := cal.List(t.Context(), domain.Filter{Kind: cmd.Kind, Start: cmd.PeriodStart, End: cmd.PeriodEnd}, cmd.Query, 100, 0)
		if err != nil || list.Total == 0 {
			t.Fatal(list, err)
		}
		if !tc.search {
			for _, event := range list.Events {
				if event.Date == nil || event.Date.Year() != 2024 {
					t.Fatal(event)
				}
			}
		}
	}
	count, err := cal.Count(t.Context())
	if err != nil || count != 1000 {
		t.Fatal(count, err)
	}
}

func structuredFixture(t *testing.T, raw string) string {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatal(err)
	}
	var fill func(map[string]any, map[string]any)
	fill = func(v, schema map[string]any) {
		props := schema["properties"].(map[string]any)
		for _, key := range schema["required"].([]string) {
			if _, exists := v[key]; !exists {
				v[key] = nil
			}
			if (key == "all" || key == "notification_context") && v[key] == nil {
				v[key] = false
			}
			if child, ok := v[key].(map[string]any); ok {
				fill(child, props[key].(map[string]any))
			}
		}
	}
	fill(value, nlp.SemanticSchema())
	data, _ := json.Marshal(value)
	return string(data)
}
