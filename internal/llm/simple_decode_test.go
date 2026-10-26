package llm

import (
	"github.com/NIDJEL/assistant-ai/internal/nlu"
	"testing"
)

func TestReportedUpdateFormattingCanBeRecoveredWithoutChangingMeaning(t *testing.T) {
	raw := []byte(`{"intent":"update","arguments":{"target":{"type":"query","value":"дизайнер","filter":{"kind":"meeting"}}},"changes":{"shift_minutes":60},"confidence":"high","clarification":null}`)
	if _, err := nlu.Decode(raw); err == nil {
		t.Fatal("public decoder must remain strict")
	}
	op, err := decodeSimpleOperation(raw)
	if err != nil {
		t.Fatal(err)
	}
	a, err := op.Args()
	if err != nil {
		t.Fatal(err)
	}
	update := a.(*nlu.Update)
	if update.Target.Type != "query" || update.Target.Value != "дизайнер" || update.Changes.ShiftMinutes == nil || *update.Changes.ShiftMinutes != 60 {
		t.Fatal(op)
	}
}

func TestUpdateFormattingRecoveryRejectsAmbiguousOrUnknownFields(t *testing.T) {
	for _, raw := range []string{
		`{"intent":"update","arguments":{"target":{"type":"last"},"changes":{"time":"12:00"}},"changes":{"shift_minutes":60},"confidence":"high","clarification":null}`,
		`{"intent":"delete","arguments":{"target":{"type":"last"}},"changes":{"status":"done"},"confidence":"high","clarification":null}`,
		`{"intent":"update","arguments":{"target":{"type":"last"}},"changes":{"target_id":42},"confidence":"high","clarification":null}`,
		`{"intent":"update","arguments":{"target":{"type":"last"}},"changes":{"shift_minutes":60},"id":42,"confidence":"high","clarification":null}`,
	} {
		if _, err := decodeSimpleOperation([]byte(raw)); err == nil {
			t.Fatal("unsafe normalization", raw)
		}
	}
}
