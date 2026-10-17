package nlp

import (
	"strings"
	"time"
)

// ExtractFilterRefinement handles only a complete, explicit "only <filter>"
// reply. The caller must have a fresh successful search/list filter to retain.
func ExtractFilterRefinement(message string, now time.Time) *SlotUpdate {
	raw := strings.TrimSpace(message)
	m := first(`(?i)^(?:(?:ну|а|тогда|давай|оставь|пожалуйста)[\s,]+)*только\s+(.+?)\s*[?.!]*$`, raw)
	if m == nil {
		return nil
	}
	expression := strings.Trim(m.Parts[1], " ,.!?")
	for kind, pattern := range map[string]string{"meeting": `встреч[аиу]|созвоны`, "task": `задач[аиу]`, "reminder": `напоминани[ея]`} {
		if full(`(?i)`+pattern, expression) {
			value := kind
			return &SlotUpdate{Kind: &value}
		}
	}
	normalized := temporalAlias(expression)
	t := ParseDateTime(normalized, now)
	if t.Error != "" || t.Start == nil || t.End == nil || t.Time != nil || t.Relative || t.Recurrence != nil || strings.Trim(t.Clean(normalized), " ,.!?:;-") != "" {
		return nil
	}
	return &SlotUpdate{PeriodExpression: &expression}
}
