package nlu

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const MaxActions = 5

// Keys are local to one plan. They are never database identifiers.
type ActionStep struct {
	AtomicGroup string    `json:"atomic_group,omitempty"`
	Key         string    `json:"key"`
	Source      string    `json:"source"`
	DependsOn   []string  `json:"depends_on,omitempty"`
	Operation   Operation `json:"operation"`
}

type MultiAction struct {
	Steps []ActionStep `json:"steps"`
}

type ResumeAction struct {
	Key       string    `json:"key"`
	Operation Operation `json:"operation"`
}

var actionKey = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

func ValidateSources(op Operation, message string) error {
	if op.Intent != "multi_action" {
		return nil
	}
	a, err := op.Args()
	if err != nil {
		return err
	}
	for i, step := range a.(*MultiAction).Steps {
		if !strings.Contains(message, step.Source) {
			return ValidationIssue{Code: "source_not_verbatim", Field: fmt.Sprintf("arguments.steps[%d].source", i)}
		}
	}
	return nil
}

func (m MultiAction) Validate() error {
	if len(m.Steps) < 2 || len(m.Steps) > MaxActions {
		return errors.New("multi_action requires two to five steps")
	}
	seen := map[string]bool{}
	closed := map[string]bool{}
	lastGroup := ""
	groups := map[string]int{}
	for _, step := range m.Steps {
		if step.AtomicGroup != "" && !actionKey.MatchString(step.AtomicGroup) {
			return errors.New("invalid atomic group")
		}
		if step.AtomicGroup != lastGroup {
			if lastGroup != "" {
				closed[lastGroup] = true
			}
			if closed[step.AtomicGroup] {
				return errors.New("atomic groups must be contiguous")
			}
			lastGroup = step.AtomicGroup
		}
		if step.AtomicGroup != "" {
			groups[step.AtomicGroup]++
		}
		if !actionKey.MatchString(step.Key) || seen[step.Key] || strings.TrimSpace(step.Source) == "" || len([]rune(step.Source)) > 2000 || len(step.DependsOn) > MaxActions-1 {
			return errors.New("invalid action key or source")
		}
		dependencies := map[string]bool{}
		for _, key := range step.DependsOn {
			if !seen[key] || dependencies[key] {
				return errors.New("dependencies must name distinct earlier steps")
			}
			dependencies[key] = true
		}
		seen[step.Key] = true
	}
	for _, count := range groups {
		if count < 2 {
			return errors.New("atomic group requires at least two steps")
		}
	}
	return nil
}

// Invalid individual operations are reported by the executor without dropping
// the other steps. Nested plans and consent fabricated inside a plan never run.
func StepAllowed(intent string) bool {
	switch intent {
	case "multi_action", "resume_action", "focus_action", "confirm", "cancel", "continue", "select", "choose_time", "chat":
		return false
	default:
		return true
	}
}
