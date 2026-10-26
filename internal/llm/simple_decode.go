package llm

import (
	"encoding/json"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
)

// Recover one lossless provider formatting mistake: update.changes beside
// arguments instead of inside it. Never infer a value, discard unknown fields
// or override another changes object. The public NLU decoder remains strict.
func decodeSimpleOperation(raw []byte) (nlu.Operation, error) {
	op, original := nlu.Decode(raw)
	if original == nil {
		return op, nil
	}
	var envelope map[string]json.RawMessage
	if len(raw) > 65536 || json.Unmarshal(raw, &envelope) != nil {
		return op, original
	}
	for key := range envelope {
		switch key {
		case "intent", "arguments", "confidence", "clarification", "changes":
		default:
			return op, original
		}
	}
	var intent string
	if json.Unmarshal(envelope["intent"], &intent) != nil || intent != "update" || len(envelope["changes"]) == 0 {
		return op, original
	}
	var args map[string]json.RawMessage
	if json.Unmarshal(envelope["arguments"], &args) != nil || args == nil || len(args["changes"]) != 0 {
		return op, original
	}
	args["changes"] = envelope["changes"]
	envelope["arguments"], _ = json.Marshal(args)
	delete(envelope, "changes")
	canonical, err := json.Marshal(envelope)
	if err != nil {
		return op, original
	}
	return nlu.Decode(canonical)
}
