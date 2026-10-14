package contract

import (
	"context"
	"testing"

	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
)

func TestSpokenCalendarReminder(t *testing.T) {
	db, cal, now := services(t)
	s := assistant.New(db, nlp.Hybrid{}, cal)
	response, err := s.Respond(context.Background(), "мне сегодня надо в универ к 12:50 на пару Оставь пожалуйста уведомление в календарь что у меня пара", "spoken", "reminder", now)
	if err != nil {
		t.Fatal(err)
	}
	if response.Action != domain.ActionCreate || len(response.Events) != 1 {
		t.Fatalf("expected creation, got %+v", response)
	}
	e := response.Events[0]
	if e.Kind != domain.EventReminder || e.Title != "Пара" || e.Date == nil || e.Date.String() != domain.Day(now).String() || e.StartTime == nil || e.StartTime.String() != "12:50" {
		t.Fatalf("wrong reminder: %+v", e)
	}
}

func TestNegatedSpeechDoesNotCompletePendingCreate(t *testing.T) {
	db, cal, now := services(t)
	s := assistant.New(db, nlp.Hybrid{}, cal)
	first, err := s.Respond(context.Background(), "Напомни позвонить маме", "negative", "start", now)
	if err != nil || !first.Pending {
		t.Fatalf("expected clarification: %+v %v", first, err)
	}
	response, err := s.Respond(context.Background(), "Не добавляй напоминание завтра в 10:00", "negative", "decline", now)
	if err != nil || response.Action == domain.ActionCreate || len(response.Events) != 0 {
		t.Fatalf("negation created an event: %+v %v", response, err)
	}
}

func TestConversationalDirectiveCanReplacePendingEmptyTitle(t *testing.T) {
	db, cal, now := services(t)
	s := assistant.New(db, nlp.Hybrid{}, cal)
	first, err := s.Respond(context.Background(), "Добавь напоминание", "new-directive", "start", now)
	if err != nil || !first.Pending {
		t.Fatalf("expected clarification: %+v %v", first, err)
	}
	response, err := s.Respond(context.Background(), "Назначь встречу с Олей завтра в 12:00", "new-directive", "replace", now)
	if err != nil || response.Action != domain.ActionCreate || len(response.Events) != 1 || response.Events[0].Kind != domain.EventMeeting {
		t.Fatalf("new directive was swallowed: %+v %v", response, err)
	}
}
