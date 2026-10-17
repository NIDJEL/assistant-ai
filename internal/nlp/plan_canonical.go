package nlp

import "strings"

func explicitTimerMarker(source string) bool {
	for _, w := range []string{"таймер", "таймера", "таймером"} {
		if ExactSourceSpan(folded(source), w) {
			return true
		}
	}
	return false
}

// CanonicalizePlan removes semantically inert payloads before compatibility
// validation. It never repairs a kind, date, query, target or exclusion.
func CanonicalizePlan(p SemanticPlan) SemanticPlan {
	if p.Status != "ok" {
		// Non-ok is an unconditional refusal. Its action data cannot execute;
		// removing it preserves that meaning and avoids burdening the model
		// with nulling every unused field in an already safe clarification.
		if oneOf(p.Status, "ambiguous", "unknown") {
			p.Operation, p.Kind, p.Title, p.Query = nil, nil, nil, nil
			p.Filters, p.Target, p.Schedule, p.Changes = nil, nil, nil, nil
			p.ChatResponse = nil
		}
		return p
	}
	if oneOf(semanticString(p.Operation), "complete", "batch_complete") && p.Changes != nil {
		c := *p.Changes
		if semanticString(c.Status) == "done" {
			c.Status = nil
			if c == (PlanChanges{}) {
				p.Changes = nil
			}
		}
	}
	// Correction already denotes the sole fresh affected target. An empty
	// "last" reference adds no selection information; Go still checks context.
	if p.DialogueAct == "correction" && p.Target != nil && semanticString(p.Target.Reference) == "last" && p.Target.Query == nil && p.Target.Ordinal == nil {
		p.Target = nil
	}
	if semanticString(p.Operation) != "search" {
		return p
	}
	if p.Title != nil && strings.TrimSpace(*p.Title) == "" {
		p.Title = nil
	}
	if c := p.Changes; c != nil && *c == (PlanChanges{}) {
		p.Changes = nil
	}
	if s := p.Schedule; s != nil && *s == (PlanSchedule{}) {
		p.Schedule = nil
	}
	return p
}

// Source-dependent canonicalization is restricted to explicit countdowns.
// A meeting's duration is never converted to a delay.
func CanonicalizePlanSource(p SemanticPlan, source string) (SemanticPlan, int) {
	before := p
	p = CanonicalizePlan(p)
	n := 0
	if before.Operation != p.Operation || before.Kind != p.Kind || before.Query != p.Query || before.Target != p.Target || before.Filters != p.Filters || before.Title != p.Title || before.Changes != p.Changes || before.Schedule != p.Schedule || before.ChatResponse != p.ChatResponse {
		n++
	}
	if p.Status == "ok" && semanticString(p.Operation) == "create" && semanticString(p.Kind) == "reminder" && explicitTimerMarker(source) && p.Schedule != nil {
		s := *p.Schedule
		if s.DurationMinutes != nil && s.Delay == nil && s.Date == nil && s.Time == nil && s.Recurrence == nil {
			unit, value := "minute", *s.DurationMinutes
			s.Delay = &TemporalMeaning{Type: "relative_delay", Unit: &unit, Value: &value}
			s.DurationMinutes = nil
			p.Schedule = &s
			n++
		}
	}
	return p, n
}
