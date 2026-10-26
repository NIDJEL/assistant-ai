package main

import (
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

func TestWrongValueMeasuresWritesIncludingInventedCreateSchedule(t *testing.T) {
	e := domain.Event{ID: 1, Kind: domain.EventTask, Title: "Заявка", Status: domain.StatusOpen, Priority: domain.PriorityNormal, Duration: time.Hour}
	w := want{Action: "create", Event: &eventSpec{Kind: "task", Title: "Заявка"}}
	if wrongValueChanges(nil, []domain.Event{e}, w, nil) != 0 {
		t.Fatal("canonical defaults penalized")
	}
	e.Date, _ = domain.ParseDate("2027-04-01")
	if wrongValueChanges(nil, []domain.Event{e}, w, nil) != 1 {
		t.Fatal("invented create date ignored")
	}
	if wrongValueChanges(nil, nil, w, nil) != 0 {
		t.Fatal("safe refusal counted as a wrong write")
	}
}

func TestWrongValueChecksPreservedFieldsOnAllowedRecord(t *testing.T) {
	e := domain.Event{ID: 1, Kind: domain.EventTask, Title: "Заявка", Status: domain.StatusOpen, Priority: domain.PriorityNormal, Duration: time.Hour}
	w := want{Action: "update", Changes: []eventSpec{{Key: "a", Title: "Новая заявка"}}}
	after := e
	after.Title = "Новая заявка"
	if wrongValueChanges([]domain.Event{e}, []domain.Event{after}, w, map[string]int64{"a": 1}) != 0 {
		t.Fatal("valid update penalized")
	}
	after.Status = domain.StatusDone
	if wrongValueChanges([]domain.Event{e}, []domain.Event{after}, w, map[string]int64{"a": 1}) != 1 {
		t.Fatal("unrequested status ignored")
	}
}
