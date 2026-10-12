package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/llm"
)

func TestStructuredCommandValidation(t *testing.T) {
	for _, content := range []string{`not json`, `{"action":"execute_sql"}`, `{"action":"create","kind":"task","title":"Договор","date":"2026-02-31"}`, `{"action":"create","kind":"task","duration_minutes":99999}`, `{"action":"create","kind":"task","sql":"DELETE FROM events"}`} {
		t.Run(content, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/chat" {
					t.Error(r.URL.Path)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["format"] == nil || body["stream"] != false {
					t.Error("missing JSON schema or stream=false")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]string{"content": content}})
			}))
			defer server.Close()
			adapter := llm.Ollama{URL: server.URL, Model: "test", Client: server.Client()}
			if _, err := adapter.Interpret(context.Background(), "Запрос", time.Now()); err == nil {
				t.Fatal("invalid command accepted")
			}
		})
	}
}
func TestHTTPContextCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-release }))
	defer server.Close()
	defer close(release)
	adapter := llm.Ollama{URL: server.URL, Client: server.Client()}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := adapter.Interpret(ctx, "Запрос", time.Now()); done <- err }()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP ignored request cancellation")
	}
}
