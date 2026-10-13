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

func restoreAPI(t *testing.T) (*api.API, *sqlite.Database, func(string, string, string, int) map[string]any) {
	t.Helper()
	db, cal, now := services(t)
	app := &api.API{Config: config.Config{Root: root()}, Clock: clock.Func(func() time.Time { return now }), Calendar: cal, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	call := func(method, path, body string, want int) map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		app.Router().ServeHTTP(w, httptest.NewRequest(method, path, bytes.NewBufferString(body)))
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		if w.Code == 204 {
			return nil
		}
		var result map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	return app, db, call
}

func TestDeletedEventsAreRecoverableAfterRestart(t *testing.T) {
	app, db, call := restoreAPI(t)
	original := call("POST", "/api/events", `{"kind":"task","title":"Вернуть исходную запись","status":"done","priority":"high","tags":["Контроль"],"subtasks":[{"id":"a","title":"Проверить результат","done":true}]}`, 201)
	path := "/api/events/" + fmt.Sprint(original["id"])
	call("DELETE", path, "", 204)
	call("GET", path, "", 404)
	call("PATCH", path, `{"title":"Нельзя изменить удалённую"}`, 404)
	if call("GET", "/api/events", "", 200)["total"] != float64(0) {
		t.Fatal("deleted event leaked into active list")
	}
	deleted := call("GET", "/api/events?deleted=true&q=исходную", "", 200)
	if deleted["total"] != float64(1) || deleted["events"].([]any)[0].(map[string]any)["deleted_at"] == nil {
		t.Fatal(deleted)
	}
	file := db.Path
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(t.Context(), file)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	app.Calendar.Store = reopened
	restored := call("POST", path+"/restore", `{}`, 200)
	for _, field := range []string{"id", "title", "created_at", "status", "priority", "tags", "subtasks", "date", "time", "duration_minutes", "source_key"} {
		before, _ := json.Marshal(original[field])
		after, _ := json.Marshal(restored[field])
		if !bytes.Equal(before, after) {
			t.Fatalf("%s changed: %s / %s", field, before, after)
		}
	}
	if _, exists := restored["deleted_at"]; exists {
		t.Fatal("restored event is still marked deleted")
	}
	call("POST", path+"/restore", `{}`, 404)
	if call("GET", "/api/events?deleted=true", "", 200)["total"] != float64(0) {
		t.Fatal("trash not cleared by restore")
	}
	call("GET", "/api/events?deleted=invalid", "", 422)
}

func TestRestoreRetainsConflictProtectionAndDeletedRemindersDoNotFire(t *testing.T) {
	app, _, call := restoreAPI(t)
	body := `{"kind":"meeting","title":"Сохранить время встречи","date":"2026-10-13","time":"08:00"}`
	one := call("POST", "/api/events", body, 201)
	path := "/api/events/" + fmt.Sprint(one["id"])
	call("DELETE", path, "", 204)
	call("POST", "/api/events", body, 201)
	conflict := call("POST", path+"/restore", `{}`, 409)
	if len(conflict["conflicts"].([]any)) != 1 {
		t.Fatal(conflict)
	}
	if call("GET", "/api/events?deleted=true", "", 200)["total"] != float64(1) {
		t.Fatal("failed restore lost the deleted record")
	}
	call("POST", path+"/restore", `{"force":true}`, 200)
	now := app.Clock.Now()
	reminder := call("POST", "/api/events", fmt.Sprintf(`{"kind":"reminder","title":"Не уведомлять после удаления","date":"%s","time":"%s"}`, now.Format("2006-01-02"), now.Format("15:04")), 201)
	reminderPath := "/api/events/" + fmt.Sprint(reminder["id"])
	call("DELETE", reminderPath, "", 204)
	due, err := app.Calendar.Due(t.Context(), now)
	if err != nil || len(due) != 0 {
		t.Fatalf("deleted reminder fired: %v %v", due, err)
	}
	call("POST", reminderPath+"/restore", `{}`, 200)
	due, err = app.Calendar.Due(t.Context(), now)
	if err != nil || len(due) != 1 {
		t.Fatalf("restored reminder missing: %v %v", due, err)
	}
}
