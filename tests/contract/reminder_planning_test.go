package contract

import (
	"context"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
)

func TestReminderQuestionPersistsUntilAnswerAndKeepsEventTime(t *testing.T) {
	db, cal, now := services(t)
	s := assistant.New(db, nlp.Hybrid{}, cal)
	s.AskReminderSettings = true
	r, err := s.Respond(context.Background(), "Добавь встречу с Олей завтра в 15:00", "lead-dialogue", "question", now)
	if err != nil || !r.Pending || r.Action != domain.ActionClarify || len(r.Events) != 0 || len(r.Suggestions) != 4 {
		t.Fatalf("missing lead question: %+v %v", r, err)
	}
	count, _ := cal.Count(context.Background())
	if count != 0 {
		t.Fatal("saved before the user chose a reminder")
	}
	// A new service instance reloads the pending command from persistent state.
	s = assistant.New(db, nlp.Hybrid{}, cal)
	s.AskReminderSettings = true
	r, err = s.Respond(context.Background(), "за час", "lead-dialogue", "answer", now)
	if err != nil || r.Pending || r.Action != domain.ActionCreate || len(r.Events) != 1 {
		t.Fatalf("answer not saved: %+v %v", r, err)
	}
	e := r.Events[0]
	if e.StartTime.String() != "15:00" || e.ReminderMinutes == nil || *e.ReminderMinutes != 60 || !e.HasReminder() {
		t.Fatalf("mixed up event and alert: %+v", e)
	}
	trigger := e.Date.At(*e.StartTime, now.Location()).Add(-time.Hour)
	due, err := cal.Due(context.Background(), trigger.Add(-time.Second))
	if err != nil || len(due) != 0 {
		t.Fatal("early notification", due, err)
	}
	due, err = cal.Due(context.Background(), trigger)
	if err != nil || len(due) != 1 {
		t.Fatal("missing early notification", due, err)
	}
	newTime, _ := domain.ParseTime("16:00")
	_, err = cal.Update(context.Background(), e.ID, calendar.Patch{Time: &newTime}, now)
	if err != nil {
		t.Fatal(err)
	}
	due, _ = cal.Due(context.Background(), trigger)
	if len(due) != 0 {
		t.Fatal("old reminder time survived moving the event")
	}
	due, _ = cal.Due(context.Background(), trigger.Add(time.Hour))
	if len(due) != 1 {
		t.Fatal("reminder did not move with the event")
	}
	disabled := false
	_, err = cal.Update(context.Background(), e.ID, calendar.Patch{ReminderEnabled: &disabled}, now)
	if err != nil {
		t.Fatal(err)
	}
	due, _ = cal.Due(context.Background(), trigger.Add(time.Hour))
	if len(due) != 0 {
		t.Fatal("disabled reminder delivered")
	}
}

func TestExplicitNotificationClockAndCrossDayOffsets(t *testing.T) {
	db, cal, now := services(t)
	s := assistant.New(db, nlp.Hybrid{}, cal)
	s.AskReminderSettings = true
	r, err := s.Respond(context.Background(), "у меня завтра пара в 12:50 Поставь пожалуйста уведомление на 12:00 чтобы я не опоздал на пару", "sample", "clock", now)
	if err != nil || r.Action != domain.ActionCreate || len(r.Events) != 1 {
		t.Fatalf("unexpected reply: %+v %v", r, err)
	}
	e := r.Events[0]
	if e.Title != "Пара" || e.StartTime.String() != "12:50" || e.ReminderMinutes == nil || *e.ReminderMinutes != 50 {
		t.Fatalf("wrong event/notification: %+v", e)
	}
	trigger := e.Date.At(*e.StartTime, now.Location()).Add(-50 * time.Minute)
	if trigger.Format("15:04") != "12:00" {
		t.Fatal(trigger)
	}
	day, _ := domain.ParseDate("2026-10-15")
	clock, _ := domain.ParseTime("00:30")
	minutes := 60
	enabled := true
	_, err = cal.Create(context.Background(), domain.Event{EventDetails: domain.EventDetails{ReminderMinutes: &minutes, ReminderEnabled: &enabled}, Kind: domain.EventTask, Title: "Ночное дело", Date: day, StartTime: clock}, now, false)
	if err != nil {
		t.Fatal(err)
	}
	nightDue, err := cal.Due(context.Background(), time.Date(2026, 10, 14, 23, 30, 0, 0, now.Location()))
	if err != nil || len(nightDue) != 1 || nightDue[0].Title != "Ночное дело" {
		t.Fatalf("cross-day task notification lost: %+v %v", nightDue, err)
	}
}

func TestNoReminderAnswerSavesEventAndCancelDoesNot(t *testing.T) {
	for _, choice := range []string{"нет", "без уведомлений", "отмена"} {
		t.Run(choice, func(t *testing.T) {
			db, cal, now := services(t)
			s := assistant.New(db, nlp.Hybrid{}, cal)
			s.AskReminderSettings = true
			_, _ = s.Respond(context.Background(), "Добавь встречу с Олей завтра в 15:00", "none", "start", now)
			r, err := s.Respond(context.Background(), choice, "none", "answer", now)
			if err != nil || r.Pending {
				t.Fatal(r, err)
			}
			if choice == "отмена" {
				if r.Action != domain.ActionCancel || len(r.Events) != 0 {
					t.Fatal(r)
				}
				return
			}
			if len(r.Events) != 1 || r.Events[0].HasReminder() {
				t.Fatal("event was cancelled instead of disabling its reminder", r)
			}
		})
	}
}

func TestReminderAnswersCannotBecomeTheTitleOrReplaceTheEventClock(t *testing.T) {
	db, cal, now := services(t)
	s := assistant.New(db, nlp.Hybrid{}, cal)
	s.AskReminderSettings = true
	_, _ = s.Respond(context.Background(), "Поставь уведомление завтра в 12:00", "no-title", "first", now)
	r, err := s.Respond(context.Background(), "за час", "no-title", "lead", now)
	if err != nil || !r.Pending || len(r.Events) != 0 {
		t.Fatal("interval became a title", r, err)
	}
	r, err = s.Respond(context.Background(), "Пара", "no-title", "title", now)
	if err != nil || len(r.Events) != 1 || r.Events[0].Title != "Пара" || *r.Events[0].ReminderMinutes != 60 {
		t.Fatal(r, err)
	}
	_, _ = s.Respond(context.Background(), "Напомни завтра позвонить маме", "no-clock", "first", now)
	r, err = s.Respond(context.Background(), "15", "no-clock", "clock", now)
	if err != nil || r.Pending || len(r.Events) != 1 || r.Events[0].StartTime.String() != "15:00" || r.Events[0].ReminderLead() != 0 {
		t.Fatal("clock was mistaken for lead time", r, err)
	}
}
