package nlp

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

func TestDialogueClock(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 30, 0, 0, time.UTC)
	for _, raw := range []string{"часов в десять", "На 15", "в 15", "через полчаса"} {
		c, err := ApplyDialogueSlots(domain.Command{Action: domain.ActionUpdate, Duration: time.Hour}, SlotUpdate{TemporalExpression: &raw}, raw, now)
		if err != nil {
			normalized := replace(`^на\s+(\d{1,2}(?::\d{2})?)$`, temporalAlias(raw), "в $1")
			t.Errorf("%q => %q: %v (%+v clean=%q)", raw, normalized, err, ParseDateTimeReply(normalized, now, c), ParseDateTimeReply(normalized, now, c).Clean(normalized))
		}
	}
}

func TestStrictContextualShape(t *testing.T) {
	typeFixture := `{"status":"ok","turn_type":"slot_reply","command":null,"slot_update":{"title":null,"temporal_expression":"через 10 минут","reminder_expression":null,"period_expression":null,"kind":null},"reference":null,"confirmation":null,"clarification":null,"chat_response":null}`
	if _, err := DecodeContextualTurn([]byte(typeFixture)); err != nil {
		t.Fatal(err)
	}
	var base map[string]any
	json.Unmarshal([]byte(typeFixture), &base)
	for _, raw := range []string{
		typeFixture + `{}`,
		`{"status":"unknown","turn_type":"chat","command":null,"slot_update":null,"reference":null,"confirmation":null,"clarification":null,"chat_response":null,"id":1}`,
		`{"status":"ok","status":"unknown","turn_type":"chat","command":null,"slot_update":null,"reference":null,"confirmation":null,"clarification":null,"chat_response":null}`,
	} {
		if _, err := DecodeContextualTurn([]byte(raw)); err == nil {
			t.Fatal("accepted malformed turn", raw)
		}
	}
	delete(base, "confirmation")
	raw, _ := json.Marshal(base)
	if _, err := DecodeContextualTurn(raw); err == nil {
		t.Fatal("missing required field accepted")
	}
}
