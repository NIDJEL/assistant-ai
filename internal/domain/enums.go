package domain

import (
	"encoding/json"
	"fmt"
)

type Action uint8

const (
	ActionUnknown Action = iota
	ActionCreate
	ActionList
	ActionSearch
	ActionUpdate
	ActionComplete
	ActionDelete
	ActionHelp
	ActionClarify
	ActionCancel
	ActionRestore
)

var actions = [...]string{"", "create", "list", "search", "update", "complete", "delete", "help", "clarify", "cancel", "restore"}

func (v Action) String() string {
	if int(v) < len(actions) {
		return actions[v]
	}
	return ""
}
func (v Action) Interpretable() bool {
	return v >= ActionCreate && v <= ActionHelp || v == ActionRestore
}
func (v Action) MarshalJSON() ([]byte, error) { return json.Marshal(v.String()) }
func (v *Action) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	for i, name := range actions {
		if name == s && i > 0 {
			*v = Action(i)
			return nil
		}
	}
	return fmt.Errorf("invalid action %q", s)
}

type EventType uint8

const (
	EventUnknown EventType = iota
	EventTask
	EventMeeting
	EventReminder
)

var kinds = [...]string{"", "task", "meeting", "reminder"}

func (v EventType) String() string {
	if int(v) < len(kinds) {
		return kinds[v]
	}
	return ""
}
func ParseEventType(s string) (EventType, error) {
	for i, n := range kinds {
		if n == s {
			return EventType(i), nil
		}
	}
	return EventUnknown, fmt.Errorf("invalid event type %q", s)
}
func (v EventType) MarshalJSON() ([]byte, error) {
	if v == EventUnknown {
		return []byte("null"), nil
	}
	return json.Marshal(v.String())
}
func (v *EventType) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*v = EventUnknown
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	n, err := ParseEventType(s)
	if err == nil {
		*v = n
	}
	return err
}

type Status uint8

const (
	StatusUnknown Status = iota
	StatusOpen
	StatusDone
)

func (v Status) String() string {
	switch v {
	case StatusOpen:
		return "open"
	case StatusDone:
		return "done"
	}
	return ""
}
func ParseStatus(s string) (Status, error) {
	switch s {
	case "":
		return StatusUnknown, nil
	case "open":
		return StatusOpen, nil
	case "done":
		return StatusDone, nil
	}
	return StatusUnknown, fmt.Errorf("invalid status %q", s)
}
func (v Status) MarshalJSON() ([]byte, error) { return json.Marshal(v.String()) }
func (v *Status) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	n, err := ParseStatus(s)
	if err == nil {
		*v = n
	}
	return err
}

type Priority uint8

const (
	PriorityUnknown Priority = iota
	PriorityNormal
	PriorityLow
	PriorityHigh
)

func (v Priority) String() string {
	switch v {
	case PriorityNormal:
		return "normal"
	case PriorityLow:
		return "low"
	case PriorityHigh:
		return "high"
	}
	return ""
}
func ParsePriority(s string) (Priority, error) {
	switch s {
	case "", "normal":
		return PriorityNormal, nil
	case "low":
		return PriorityLow, nil
	case "high":
		return PriorityHigh, nil
	}
	return PriorityUnknown, fmt.Errorf("invalid priority %q", s)
}
func (v Priority) MarshalJSON() ([]byte, error) { return json.Marshal(v.String()) }
func (v *Priority) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	n, err := ParsePriority(s)
	if err == nil {
		*v = n
	}
	return err
}
