package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/api"
	"github.com/NIDJEL/assistant-ai/internal/clock"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/storage/sqlite"
)

func TestDesignMetadataAndPreferencesSurviveRestart(t *testing.T) {
	db, cal, now := services(t)
	app := &api.API{Config: config.Config{Root: root()}, Clock: clock.Func(func() time.Time { return now }), Calendar: cal, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	call := func(method, path, body string, want int) map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		app.Router().ServeHTTP(w, httptest.NewRequest(method, path, bytes.NewBufferString(body)))
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	task := call("POST", "/api/events", `{"kind":"task","title":"Проверка метаданных","tags":["Контроль"],"subtasks":[{"id":"a","title":"Проверить итог","done":false}]}`, 201)
	if call("GET", "/api/tasks?q=Контроль", "", 200)["total"] != float64(1) {
		t.Fatal("tag search missing")
	}
	id := fmt.Sprint(task["id"])
	call("PATCH", "/api/events/"+id, `{"subtasks":[{"id":"a","title":"Проверить итог","done":true}]}`, 200)
	meeting := call("POST", "/api/events", `{"kind":"meeting","title":"Обсуждение","date":"2026-10-13","time":"15:00","participants":[{"name":"Тестовый участник","email":"person@example.org"}]}`, 201)
	disabled := call("POST", "/api/events", fmt.Sprintf(`{"kind":"reminder","title":"Проверка отключения","date":"%s","time":"%s","reminder_enabled":false}`, now.Format("2006-01-02"), now.Format("15:04")), 201)
	reminderPath := "/api/events/" + fmt.Sprint(disabled["id"])
	call("PATCH", reminderPath, `{"reminder_enabled":true}`, 200)
	activeDue, err := cal.Due(t.Context(), now)
	if err != nil || len(activeDue) != 1 {
		t.Fatalf("enabled reminder not due: %v %v", activeDue, err)
	}
	call("PATCH", reminderPath, `{"reminder_enabled":false}`, 200)
	profile := "/api/preferences/11111111-1111-4111-8111-111111111111"
	call("PUT", profile, `{"widgets":false,"layouts":{"/tasks":{"order":["tasks","progress"],"hidden":["progress"],"settings":{"tasks":{"limit":5}}}}}`, 200)
	// Close and reopen the actual SQLite file; this also exercises migration persistence.
	path := db.Path
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	cal.Store = reopened
	loaded := call("GET", "/api/events/"+id, "", 200)
	if loaded["subtasks"].([]any)[0].(map[string]any)["done"] != true || loaded["tags"].([]any)[0] != "Контроль" {
		t.Fatal(loaded)
	}
	loaded = call("GET", "/api/events/"+fmt.Sprint(meeting["id"]), "", 200)
	if loaded["participants"].([]any)[0].(map[string]any)["name"] != "Тестовый участник" {
		t.Fatal(loaded)
	}
	loaded = call("GET", "/api/events/"+fmt.Sprint(disabled["id"]), "", 200)
	if loaded["reminder_enabled"] != false {
		t.Fatal(loaded)
	}
	due, err := cal.Due(t.Context(), now)
	if err != nil || len(due) != 0 {
		t.Fatalf("disabled reminder due: %v %v", due, err)
	}
	prefs := call("GET", profile, "", 200)
	if prefs["widgets"] != false {
		t.Fatal(prefs)
	}
	order := prefs["layouts"].(map[string]any)["/tasks"].(map[string]any)["order"].([]any)
	if order[0] != "tasks" {
		t.Fatal(prefs)
	}
	call("PATCH", "/api/events/"+id, `{"subtasks":[],"tags":[]}`, 200)
	loaded = call("GET", "/api/events/"+id, "", 200)
	if loaded["subtasks"] != nil || loaded["tags"] != nil {
		t.Fatal("collections not cleared", loaded)
	}
	call("PATCH", "/api/events/"+id, `{"subtasks":[{"id":"x","title":"a"},{"id":"x","title":"b"}]}`, 422)
	call("PUT", profile, `[]`, 422)
	call("GET", "/api/preferences/invalid", "", 422)
}
