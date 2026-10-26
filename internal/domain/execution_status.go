package domain

import "time"

type ExecutionStatus struct {
	EventID         int64      `json:"event_id"`
	Provider        string     `json:"provider"`
	State           string     `json:"state"`
	Message         string     `json:"message"`
	StepKeys        []string   `json:"step_keys,omitempty"`
	Attempts        int        `json:"attempts,omitempty"`
	AcceptedDevices int        `json:"accepted_devices,omitempty"`
	PendingDevices  int        `json:"pending_devices,omitempty"`
	NextAttemptAt   *time.Time `json:"next_attempt_at,omitempty"`
}
