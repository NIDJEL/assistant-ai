package nlp

import (
	"reflect"
	"strconv"
	"strings"
)

// EvidenceRequirement concerns provenance only. Schema validation and language
// grounding still apply to optional fields (including generated create titles).
type EvidenceRequirement string

const (
	Required            EvidenceRequirement = "Required"
	OptionalNonBlocking EvidenceRequirement = "OptionalNonBlocking"
	NotApplicable       EvidenceRequirement = "NotApplicable"
)

// EvidencePolicyFields is the explicit inventory of semantic evidence units.
// Temporal/clock objects are one unit; exclusion indices use a wildcard.
var evidencePolicyFields = map[string]bool{
	"target.reference": true, "target.ordinal": true, "target.query": true,
	"changes.date": true, "changes.time": true, "changes.delay": true,
	"changes.time_shift_minutes": true, "changes.title": true,
	"changes.status": true, "changes.priority": true,
	"changes.reminder_minutes": true, "changes.reminder_enabled": true,
	"filters.period": true, "filters.time_from": true, "filters.time_before": true,
	"filters.status": true, "filters.priority": true,
	"filters.exclude.*.query": true, "filters.exclude.*.kind": true,
	"filters.exclude.*.status": true, "filters.exclude.*.priority": true,
	"filters.exclude.*.period": true, "query": true, "batch.scope": true,
	"title": false, "kind": false,
	"schedule.date": false, "schedule.time": false, "schedule.delay": false,
	"schedule.duration_minutes": false, "schedule.reminder_minutes": false,
	"schedule.reminder_enabled": false, "schedule.recurrence": false,
}

func evidencePolicyPath(field string) string {
	parts := strings.Split(field, ".")
	if len(parts) == 4 && parts[0] == "filters" && parts[1] == "exclude" {
		if _, err := strconv.Atoi(parts[2]); err == nil {
			parts[2] = "*"
		}
	}
	return strings.Join(parts, ".")
}

func IsPlanMutation(operation string) bool {
	return oneOf(operation, "update", "delete", "complete", "batch_update", "batch_delete", "batch_complete")
}

// EvidencePolicy maps user constraints, neutral values and presentation defaults
// centrally. An optional proof can never promote a field to Required.
func EvidencePolicy(field, operation string, value any) EvidenceRequirement {
	if value == nil {
		return NotApplicable
	}
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return NotApplicable
		}
		return EvidencePolicy(field, operation, v.Elem().Interface())
	}
	path := evidencePolicyPath(field)
	if text, ok := value.(string); ok && (strings.HasSuffix(path, ".status") || strings.HasSuffix(path, ".kind") || path == "kind") && text == "any" {
		return NotApplicable
	}
	mutationSensitive, known := evidencePolicyFields[path]
	if !known {
		return NotApplicable
	}
	// Applicability follows the operation contract, independently of whether
	// the provider attached a proof. Invalid combinations still fail schema.
	group := strings.Split(path, ".")[0]
	allowed := map[string][]string{
		"target":  {"search", "update", "delete", "complete"},
		"changes": {"update", "batch_update"},
		"batch":   {"batch_update", "batch_delete", "batch_complete"},
		"query":   {"search", "batch_update", "batch_delete", "batch_complete"},
		"title":   {"create"}, "schedule": {"create"},
		"filters": {"list", "search", "update", "delete", "complete", "batch_update", "batch_delete", "batch_complete"},
		"kind":    {"create", "list", "search", "update", "delete", "complete", "batch_update", "batch_delete", "batch_complete"},
	}
	if !oneOf(operation, allowed[group]...) {
		return NotApplicable
	}
	if mutationSensitive && IsPlanMutation(operation) {
		return Required
	}
	return OptionalNonBlocking
}

// EvidencePolicyInventory returns a copy for documentation and policy audits.
func EvidencePolicyInventory(operation string) map[string]EvidenceRequirement {
	out := make(map[string]EvidenceRequirement, len(evidencePolicyFields))
	for field := range evidencePolicyFields {
		out[field] = EvidencePolicy(field, operation, "populated")
	}
	return out
}
