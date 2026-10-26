package domain

import (
	"encoding/json"
	"time"
)

// ActionPlan is persisted with dialogue state in the same transaction as its
// effects and the request cache. A completed step is never scheduled again.
type ActionPlan struct {
	Token        string                 `json:"token"`
	Groups       map[string]AtomicGroup `json:"groups,omitempty"`
	BaseLastIDs  []int64                `json:"base_last_ids,omitempty"`
	BaseDialogue *DialogueState         `json:"base_dialogue,omitempty"`
	At           time.Time              `json:"at"`
	Active       string                 `json:"active"`
	Steps        []ActionStep           `json:"steps"`
}

type ActionStep struct {
	AtomicGroup string          `json:"atomic_group,omitempty"`
	Key         string          `json:"key"`
	Source      string          `json:"source"`
	DependsOn   []string        `json:"depends_on,omitempty"`
	Operation   json.RawMessage `json:"operation"`
	Status      string          `json:"status"`
	Action      Action          `json:"action,omitempty"`
	Pending     *Pending        `json:"pending,omitempty"`
	IDs         []int64         `json:"ids,omitempty"`
	Reply       string          `json:"reply,omitempty"`
	Preview     *Response       `json:"preview,omitempty"`
}

type ActionResult struct {
	AtomicGroup  string  `json:"atomic_group,omitempty"`
	ControlToken string  `json:"control_token,omitempty"`
	CanConfirm   bool    `json:"can_confirm,omitempty"`
	Active       bool    `json:"active,omitempty"`
	Applied      bool    `json:"applied,omitempty"`
	Action       Action  `json:"action,omitempty"`
	EventIDs     []int64 `json:"event_ids,omitempty"`
	Key          string  `json:"key"`
	Status       string  `json:"status"`
	Reply        string  `json:"reply"`
}

type AtomicGroup struct {
	EvaluatedAt time.Time `json:"evaluated_at"`
	Snapshot    string    `json:"snapshot"`
	PreviewAt   time.Time `json:"preview_at"`
	Ready       bool      `json:"ready"`
}
