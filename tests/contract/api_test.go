package contract

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/api"
	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/clock"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
)

func TestHTTPCRUDAndTypedErrors(t *testing.T) {
	db, cal, now := services(t)
	app := &api.API{Config: config.Config{Root: root(), Timezone: "Europe/Moscow", NLPProvider: "rules"}, Clock: clock.Func(func() time.Time { return now }), Calendar: cal, Assistant: assistant.New(db, nlp.Hybrid{}, cal), Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	router := app.Router()
	request := func(method, path, body string, status int) map[string]any {
		t.Helper()
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		if status == 204 {
			return nil
		}
		var value map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		if status >= 400 {
			if value["error"] == nil || value["request_id"] == nil {
				t.Fatal("unstructured error")
			}
		}
		return value
	}
	created := request("POST", "/api/events", `{"kind":"task","title":"Документы","description":"Проверить договор с Иваном","priority":"high"}`, 201)
	if created["id"] != float64(1) || created["priority"] != "high" {
		t.Fatal(created)
	}
	listed := request("GET", "/api/tasks?q=контракт&status=open", "", 200)
	if listed["total"] != float64(1) {
		t.Fatal(listed)
	}
	request("PATCH", "/api/events/1", `{"date":"2026-10-13","time":"10:00","status":"done"}`, 200)
	request("GET", "/api/events/1", "", 200)
	meeting := request("POST", "/api/events", `{"kind":"meeting","title":"Встреча с Иваном","date":"2026-10-13","time":"15:00","location":"Переговорная","meeting_url":"https://example.org/meeting"}`, 201)
	if meeting["location"] != "Переговорная" || meeting["meeting_url"] != "https://example.org/meeting" {
		t.Fatal("meeting details not stored", meeting)
	}
	request("PATCH", "/api/events/2", `{"location":"Офис","meeting_url":""}`, 200)
	meeting = request("GET", "/api/events/2", "", 200)
	if meeting["location"] != "Офис" || meeting["meeting_url"] != "" {
		t.Fatal("meeting details not updated", meeting)
	}
	request("PATCH", "/api/events/2", `{"meeting_url":"javascript:alert(1)"}`, 422)
	request("POST", "/api/events", `{"kind":"meeting","title":"Встреча с Ольгой","date":"2026-10-13","time":"15:30"}`, 409)
	request("POST", "/api/events", `{"kind":"meeting","title":"Встреча с Ольгой","date":"2026-10-13","time":"15:30","force":true}`, 201)
	request("DELETE", "/api/events/1", "", 204)
	request("GET", "/api/events/1", "", 404)
	for _, body := range []string{`{"kind":"sql","title":"x"}`, `{"kind":"task","title":"x","priority":"critical"}`, `{"kind":"task","title":"x","duration_minutes":9223372036854775807}`, `{"kind":"meeting","title":"x"}`, `{"kind":"task","title":"x","date":"0000-01-01"}`} {
		request("POST", "/api/events", body, 422)
	}
	request("PATCH", "/api/events/2", `{"time":null}`, 422)
	request("PATCH", "/api/events/2", `{"duration_minutes":9223372036854775807}`, 422)
	request("POST", "/health", `{}`, 405)
	request("GET", "/api/unknown", "", 404)
	spec := request("GET", "/openapi.json", "", 200)
	paths := spec["paths"].(map[string]any)
	get := paths["/api/events/{id}"].(map[string]any)["get"].(map[string]any)
	if len(get["parameters"].([]any)) == 0 {
		t.Fatal("missing OpenAPI path parameter")
	}
}
