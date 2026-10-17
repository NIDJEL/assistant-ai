package llm

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/nlp"
)

func TestPlanProviderOriginalMessageStrictSchemaAndNoTools(t *testing.T) {
	op, kind, query := "search", "meeting", "Алисой"
	p := nlp.SemanticPlan{Status: "ok", DialogueAct: "command", Operation: &op, Kind: &kind, Query: &query}
	raw, _ := json.Marshal(p)
	text := "  Ну, найди встречи с Алисой только за предыдущий месяц?  "
	calls := 0
	g := testGiga(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth" {
			tokenResponse(w, "token")
			return
		}
		calls++
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["tools"] != nil || body["functions"] != nil {
			t.Fatal("LLM received execution tools")
		}
		var format struct {
			Strict bool           `json:"strict"`
			Schema map[string]any `json:"schema"`
		}
		json.Unmarshal(body["response_format"], &format)
		if !format.Strict || format.Schema["additionalProperties"] != false {
			t.Fatal("non-strict schema")
		}
		var messages []map[string]string
		json.Unmarshal(body["messages"], &messages)
		var user struct {
			Message string              `json:"current_message"`
			Context nlp.DialogueContext `json:"context"`
		}
		json.Unmarshal([]byte(messages[1]["content"]), &user)
		if user.Message != text || user.Context.Query != "Алисой" || strings.Contains(messages[1]["content"], "target_id") {
			t.Fatal("changed original or unsafe context")
		}
		chatResponse(w, string(raw))
	})
	model := &Planner{Provider: g}
	got, err := model.ParsePlan(t.Context(), text, nlp.DialogueContext{LastAction: "search", LastKind: "meeting", Query: "Алисой"}, time.Now())
	if err != nil || *got.Operation != "search" || calls != 1 {
		t.Fatal(got, err, calls)
	}
}

func TestPlanLanguageGroundingRejectsInventedTargets(t *testing.T) {
	op, kind, title := "update", "meeting", "Встреча с вымышленным участником"
	ref := "query"
	p := nlp.SemanticPlan{Status: "ok", DialogueAct: "command", Operation: &op, Kind: &kind, Target: &nlp.PlanTarget{Reference: &ref, Query: &title}}
	if groundPlanLanguage(p, "Перенеси встречу с Ириной", nlp.DialogueContext{}) == nil {
		t.Fatal("invented target accepted")
	}
}

func TestOptionalTimerTitleRetainsLanguageGrounding(t *testing.T) {
	op, kind, title, unit := "create", "reminder", "Таймер", "minute"
	minutes := 13
	p := nlp.SemanticPlan{Status: "ok", DialogueAct: "command", Operation: &op, Kind: &kind, Title: &title,
		Schedule: &nlp.PlanSchedule{Delay: &nlp.TemporalMeaning{Type: "relative_delay", Unit: &unit, Value: &minutes}},
		Evidence: []nlp.PlanEvidence{{Field: "title", Text: "Таймер"}}}
	if err := nlp.ValidatePlanEvidence(p, "Засеки 13 минут"); err != nil {
		t.Fatal(err)
	}
	if err := groundPlanLanguage(p, "Засеки 13 минут", nlp.DialogueContext{}); err != nil {
		t.Fatal(err)
	}
	title = "Выдуманное содержание"
	if groundPlanLanguage(p, "Засеки 13 минут", nlp.DialogueContext{}) == nil {
		t.Fatal("optional evidence allowed fabricated create title")
	}
	title = "Таймер"
	kind = "task"
	if groundPlanLanguage(p, "Засеки 13 минут", nlp.DialogueContext{}) == nil {
		t.Fatal("canonical title exception escaped timer create")
	}
}
