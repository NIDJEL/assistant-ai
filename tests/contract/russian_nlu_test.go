package contract

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/storage/sqlite"
)

func TestRussianReminderRegressionExecutesWithoutClarification(t *testing.T) {
	for _, message := range []string{
		"Напомни мне что через минуту и нужно снять чайник с плиты",
		"Напомни мне что через минуту нужно снять чайник с плиты",
		"Мне через минуту надо снять чайник с плиты напомни",
		"Через минуту напомни снять чайник с плиты",
	} {
		t.Run(message, func(t *testing.T) {
			db, cal, now := services(t)
			now = now.Add(47 * time.Second)
			s := assistant.New(db, nlp.Hybrid{}, cal)
			s.AskReminderSettings = true // Production uses this setting.
			// Service must interpret relative expressions in the app timezone,
			// even when its caller happens to supply a UTC clock.
			r, err := s.Respond(t.Context(), message, "regression", "one", now.UTC())
			if err != nil || r.Pending || r.Action != domain.ActionCreate || len(r.Events) != 1 {
				t.Fatalf("unnecessary clarification: %+v %v", r, err)
			}
			e := r.Events[0]
			start, _, ok := e.Interval(cal.Location)
			if !ok || !start.Equal(now.Add(time.Minute)) || e.Kind != domain.EventReminder || e.Title != "Снять чайник с плиты" {
				t.Fatalf("wrong reminder: %+v, start=%v", e, start)
			}
			due, err := cal.Due(t.Context(), start.Add(-time.Second))
			if err != nil || len(due) != 0 {
				t.Fatal("early reminder", due, err)
			}
			due, err = cal.Due(t.Context(), start)
			if err != nil || len(due) != 1 {
				t.Fatal("missing reminder", due, err)
			}
			again, err := s.Respond(t.Context(), message, "regression", "one", now)
			if err != nil || len(again.Events) != 1 || again.Events[0].ID != e.ID {
				t.Fatal("idempotence lost", again, err)
			}
			count, err := cal.Count(t.Context())
			if err != nil || count != 1 {
				t.Fatal("duplicate reminder", count, err)
			}
		})
	}
}

func TestRussianReminderMissingFieldsAndFollowups(t *testing.T) {
	for _, tc := range []struct{ first, question, reply, date, clock string }{
		{"Напомни купить молоко", "Когда напомнить?", "через минуту", "2026-10-12", "12:31"},
		{"Напомни завтра купить молоко", "Во сколько напомнить?", "в 8", "2026-10-13", "08:00"},
		{"Напомни в пятницу купить молоко", "Во сколько напомнить?", "вечером", "2026-10-16", "18:00"},
		{"Напомни купить молоко на следующей неделе", "В какой день указанного периода напомнить?", "в понедельник в 8", "2026-10-19", "08:00"},
	} {
		t.Run(tc.first, func(t *testing.T) {
			db, cal, now := services(t)
			s := assistant.New(db, nlp.Hybrid{}, cal)
			s.AskReminderSettings = true
			r, err := s.Respond(t.Context(), tc.first, "dialogue", "first", now)
			if err != nil || !r.Pending || r.Reply != tc.question || len(r.Events) != 0 {
				t.Fatal(r, err)
			}
			// Reload pending state and merge only missing fields.
			s = assistant.New(db, nlp.Hybrid{}, cal)
			s.AskReminderSettings = true
			r, err = s.Respond(t.Context(), tc.reply, "dialogue", "reply", now)
			if err != nil || r.Pending || len(r.Events) != 1 || r.Events[0].Title != "Купить молоко" || r.Events[0].Kind != domain.EventReminder {
				t.Fatal("followup failed", r, err)
			}
			if r.Events[0].Date.String() != tc.date || r.Events[0].StartTime.String() != tc.clock {
				t.Fatal("followup changed the intended day/time", r)
			}
		})
	}
}

func TestRussianTrailingDirectiveDoesNotAskForLead(t *testing.T) {
	db, cal, now := services(t)
	s := assistant.New(db, nlp.Hybrid{}, cal)
	s.AskReminderSettings = true
	r, err := s.Respond(t.Context(), "Мне завтра в 8 надо купить молоко напомни", "trailing", "first", now)
	if err != nil || r.Pending || len(r.Events) != 1 || r.Events[0].Title != "Купить молоко" {
		t.Fatal(r, err)
	}
}

func TestPostgresRussianNLURetainsSecondPrecision(t *testing.T) {
	db, _, router, now := pgFixture(t)
	userID, _ := registerHTTP(t, router, "nlu@example.test")
	ctx := repository.WithUser(t.Context(), userID)
	cal := &calendar.Service{Store: db, Location: now.Location()}
	s := assistant.New(db, nlp.Hybrid{}, cal)
	s.AskReminderSettings = true
	now = now.Add(47 * time.Second)
	r, err := s.Respond(ctx, "Напомни мне что через минуту нужно снять чайник с плиты", "pg-nlu", "first", now)
	if err != nil || r.Pending || len(r.Events) != 1 {
		t.Fatal(r, err)
	}
	e, err := cal.Get(ctx, r.Events[0].ID)
	if err != nil || e == nil {
		t.Fatal(e, err)
	}
	start, _, _ := e.Interval(cal.Location)
	if !start.Equal(now.Add(time.Minute)) {
		t.Fatal("PostgreSQL lost seconds", start)
	}
	due, err := cal.Due(ctx, start.Add(-time.Second))
	if err != nil || len(due) != 0 {
		t.Fatal("early PostgreSQL reminder", due, err)
	}
	due, err = cal.Due(ctx, start)
	if err != nil || len(due) != 1 {
		t.Fatal("missing PostgreSQL reminder", due, err)
	}
}

func TestRussianSecondPrecisionSurvivesRestartAndReschedule(t *testing.T) {
	db, cal, now := services(t)
	now = now.Add(45 * time.Second)
	s := assistant.New(db, nlp.Hybrid{}, cal)
	r, err := s.Respond(t.Context(), "Напомни через 30 секунд проверить почту", "seconds", "first", now)
	if err != nil || len(r.Events) != 1 {
		t.Fatal(r, err)
	}
	id := r.Events[0].ID
	file := db.Path
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(t.Context(), file)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cal.Store = db
	s = assistant.New(db, nlp.Hybrid{}, cal)
	e, err := cal.Get(t.Context(), id)
	if err != nil || e == nil {
		t.Fatal(e, err)
	}
	start, _, _ := e.Interval(cal.Location)
	if !start.Equal(now.Add(30 * time.Second)) {
		t.Fatal("precision lost after restart", start)
	}
	r, err = s.Respond(t.Context(), fmt.Sprintf("Перенеси напоминание #%d на завтра в 10", id), "seconds", "move", now)
	if err != nil || len(r.Events) != 1 || r.Events[0].StartSecond != 0 || r.Events[0].StartTime.String() != "10:00" {
		t.Fatal("seconds survived explicit reschedule", r, err)
	}
	newClock, _ := domain.ParseTime("11:00")
	e, err = cal.Update(t.Context(), id, calendar.Patch{Time: &newClock}, now)
	if err != nil || e.StartSecond != 0 {
		t.Fatal(e, err)
	}
}

func TestRussianRestoreUsesDeletedRecordsAndProtectsConflicts(t *testing.T) {
	db, cal, now := services(t)
	s := assistant.New(db, nlp.Hybrid{}, cal)
	one := selectionEvent(t, t.Context(), cal, now, "Проверить почту", domain.EventTask, 1, "")
	if err := cal.Delete(t.Context(), one.ID); err != nil {
		t.Fatal(err)
	}
	r, err := s.Respond(t.Context(), "Восстанови задачу проверить почту из корзины", "restore", "first", now)
	if err != nil || r.Action != domain.ActionRestore || r.Pending || len(r.Events) != 1 || r.Events[0].ID != one.ID || r.Events[0].DeletedAt != nil {
		t.Fatal(r, err)
	}
	meeting := selectionEvent(t, t.Context(), cal, now, "С Олей", domain.EventMeeting, 1, "10:00")
	if err := cal.Delete(t.Context(), meeting.ID); err != nil {
		t.Fatal(err)
	}
	selectionEvent(t, t.Context(), cal, now, "С командой", domain.EventMeeting, 1, "10:00")
	r, err = s.Respond(t.Context(), fmt.Sprintf("Восстанови встречу #%d", meeting.ID), "restore", "conflict", now)
	if err != nil || !r.Pending || !strings.Contains(r.Reply, "пересекается") {
		t.Fatal("missing conflict protection", r, err)
	}
	if err := db.Read(t.Context(), func(tx repository.Tx) error {
		deleted, err := tx.GetDeleted(t.Context(), meeting.ID)
		if err != nil {
			return err
		}
		if deleted == nil {
			t.Fatal("restored before conflict confirmation")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	r, err = s.Respond(context.Background(), "да", "restore", "yes", now)
	if err != nil || r.Pending || r.Action != domain.ActionRestore || len(r.Events) != 1 || r.Events[0].ID != meeting.ID {
		t.Fatal(r, err)
	}
}
