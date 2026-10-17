package llm

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/NIDJEL/assistant-ai/internal/nlu"
)

func TestSimpleGigaChatUsesCompactContextAndStrictOperationDecoder(t *testing.T) {
	calls := 0
	g := testGiga(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth" {
			tokenResponse(w, "unit-token")
			return
		}
		calls++
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if _, ok := payload["response_format"]; ok {
			t.Fatal("simple runtime must not send giant semantic schema")
		}
		messages := payload["messages"].([]any)
		system := messages[0].(map[string]any)["content"].(string)
		if strings.Contains(system, "SemanticPlan") || strings.Contains(system, "evidence") {
			t.Fatal("legacy prompt used")
		}
		user := messages[1].(map[string]any)["content"].(string)
		if !strings.Contains(user, `"index":2`) || strings.Contains(user, `"id"`) {
			t.Fatal(user)
		}
		if calls == 1 {
			chatResponse(w, `{"intent":"update","arguments":{"target":{"type":"ordinal","index":2},"changes":{"time":"16:00"}},"confidence":"high","clarification":null}`)
		} else {
			chatResponse(w, `{"intent":"delete","arguments":{"target":{"type":"last","id":481}},"confidence":"high","clarification":null}`)
		}
	})
	s := &Simple{Provider: g}
	d := nlu.Context{Timezone: "Europe/Moscow", LastResults: []nlu.Item{{Index: 2, Title: "Встреча с Алисой"}}}
	op, err := s.InterpretOperation(t.Context(), "вторую на 16", d)
	if err != nil || op.Intent != "update" {
		t.Fatal(op, err)
	}
	if _, err = s.InterpretOperation(t.Context(), "её удали", d); err == nil {
		t.Fatal("model ID accepted")
	}
	if calls != 2 {
		t.Fatal("unexpected extra inference", calls)
	}
}
