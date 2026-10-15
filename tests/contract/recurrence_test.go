package contract

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/storage/postgres"
	"github.com/NIDJEL/assistant-ai/internal/storage/sqlite"
	"github.com/google/uuid"
)

var extendedScenarios = []struct {
	message, reply, title string
	recurring             bool
}{
	{"через час и 20 минут напомни проверить духовку", "", "Проверить духовку", false},
	{"через два дня и три часа напомни проверить сервер", "", "Проверить сервер", false},
	{"через месяц напомни оплатить подписку", "", "Оплатить подписку", false},
	{"каждый день в 9 напомни проверить почту", "", "Проверить почту", true},
	{"по будням в 8:30 напомни выйти из дома", "", "Выйти из дома", true},
	{"каждые два часа напомни размяться", "", "Размяться", true},
	{"15 числа каждого месяца напомни оплатить интернет", "в 9", "Оплатить интернет", true},
}

func assertStoredOccurrence(t *testing.T, cal *calendar.Service, ctxID string, e domain.Event, now time.Time) {
	t.Helper()
	ctx := t.Context()
	if ctxID != "" {
		ctx = repository.WithUser(ctx, ctxID)
	}
	stored, err := cal.Get(ctx, e.ID)
	if err != nil || stored == nil {
		t.Fatal(stored, err)
	}
	if stored.Title != e.Title || (stored.Recurrence == nil) != (e.Recurrence == nil) {
		t.Fatal("read-back changed event", stored)
	}
	start, _, ok := stored.Interval(cal.Location)
	if !ok || !start.After(now) {
		t.Fatal("bad occurrence", start)
	}
	due, err := cal.Due(ctx, start.Add(-time.Second))
	if err != nil || len(due) != 0 {
		t.Fatal("early signal", due, err)
	}
	if db, ok := cal.Store.(*postgres.Database); ok {
		if _, err := db.ProcessDue(ctx, start, cal.Location); err != nil {
			t.Fatal(err)
		}
	}
	due, err = cal.Due(ctx, start)
	if err != nil || len(due) != 1 || due[0].ID != e.ID {
		t.Fatal("missing signal", due, err)
	}
	if e.Recurrence != nil {
		next, err := cal.Get(ctx, e.ID)
		if err != nil {
			t.Fatal(err)
		}
		at, _, _ := next.Interval(cal.Location)
		if !at.After(start) || !next.Recurrence.Anchor.Equal(e.Recurrence.Anchor) {
			t.Fatal("series lost or drifted", next)
		}
		replay, err := cal.Due(ctx, start.Add(time.Minute))
		if err != nil || len(replay) != 1 {
			t.Fatal("polling replay lost", replay, err)
		}
		got, _, _ := replay[0].Interval(cal.Location)
		if !got.Equal(start) {
			t.Fatal("replayed another occurrence", got, start)
		}
	}
}

func TestSQLiteExtendedNLUCreationAndPersistence(t *testing.T) {
	for _, tc := range extendedScenarios {
		t.Run(tc.message, func(t *testing.T) {
			db, cal, now := services(t)
			s := assistant.New(db, nlp.Hybrid{}, cal)
			s.AskReminderSettings = true
			r, err := s.Respond(t.Context(), tc.message, "extended", "create", now)
			if err != nil {
				t.Fatal(err)
			}
			if tc.reply != "" {
				if !r.Pending || r.Reply != "Во сколько напомнить?" {
					t.Fatal(r)
				}
				s = assistant.New(db, nlp.Hybrid{}, cal)
				s.AskReminderSettings = true
				r, err = s.Respond(t.Context(), tc.reply, "extended", "reply", now)
			}
			if err != nil || r.Pending || len(r.Events) != 1 || r.Events[0].Title != tc.title || (r.Events[0].Recurrence != nil) != tc.recurring {
				t.Fatal(r, err)
			}
			file := db.Path
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := sqlite.Open(t.Context(), file)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			cal.Store = reopened
			assertStoredOccurrence(t, cal, "", r.Events[0], now)
		})
	}
}

func TestPostgresExtendedNLUAPIAndPersistence(t *testing.T) {
	for _, tc := range extendedScenarios {
		t.Run(tc.message, func(t *testing.T) {
			db, _, router, now := pgFixture(t)
			uid, cookie := registerHTTP(t, router, "extended@example.test")
			sid := uuid.NewString()
			body, _ := call(t, router, cookie, "POST", "/api/chat", map[string]string{"message": tc.message, "session_id": sid, "request_id": uuid.NewString()}, 200)
			if tc.reply != "" {
				if body["pending"] != true || body["reply"] != "Во сколько напомнить?" {
					t.Fatal(body)
				}
				body, _ = call(t, router, cookie, "POST", "/api/chat", map[string]string{"message": tc.reply, "session_id": sid, "request_id": uuid.NewString()}, 200)
			}
			raw, _ := json.Marshal(body)
			var response domain.Response
			if err := json.Unmarshal(raw, &response); err != nil {
				t.Fatal(err)
			}
			if response.Pending || len(response.Events) != 1 || response.Events[0].Title != tc.title || (response.Events[0].Recurrence != nil) != tc.recurring {
				t.Fatal(response)
			}
			e := response.Events[0]
			call(t, router, cookie, "GET", fmt.Sprintf("/api/events/%d", e.ID), nil, 200)
			var schema string
			if err := db.DB.QueryRow("SELECT current_schema()").Scan(&schema); err != nil {
				t.Fatal(err)
			}
			dsn, _ := url.Parse(os.Getenv("TEST_DATABASE_URL"))
			q := dsn.Query()
			q.Set("search_path", schema)
			dsn.RawQuery = q.Encode()
			reopened, err := postgres.Open(t.Context(), dsn.String())
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			cal := &calendar.Service{Store: reopened, Location: now.Location()}
			assertStoredOccurrence(t, cal, uid, e, now)
		})
	}
}

func TestRecurringLifecycleAndNoDrift(t *testing.T) {
	db, cal, now := services(t)
	s := assistant.New(db, nlp.Hybrid{}, cal)
	r, err := s.Respond(t.Context(), "каждые два часа напомни размяться", "lifecycle", "create", now)
	if err != nil || len(r.Events) != 1 {
		t.Fatal(r, err)
	}
	e := r.Events[0]
	start, _, _ := e.Interval(cal.Location)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			if _, err := cal.Due(t.Context(), start.Add(45*time.Second)); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	stored, err := cal.Get(t.Context(), e.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The editor sends unchanged date/time on a title save. Preserve the phase
	// and sub-minute precision of elapsed rules in that case.
	day, clock := stored.Date, stored.StartTime
	stored, err = cal.Update(t.Context(), e.ID, calendar.Patch{Date: &day, Time: &clock}, now)
	if err != nil || stored.StartSecond != e.StartSecond || !stored.Recurrence.Anchor.Equal(e.Recurrence.Anchor) {
		t.Fatal("editor changed series precision", stored, err)
	}
	next, _, _ := stored.Interval(cal.Location)
	if !next.Equal(start.Add(2 * time.Hour)) {
		t.Fatal("poll drift or duplicate advancement", next)
	}
	// Reopening storage must replay the same occurrence without advancing again.
	path := db.Path
	db.Close()
	db, err = sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cal.Store = db
	replay, err := cal.Due(t.Context(), start.Add(time.Minute))
	if err != nil || len(replay) != 1 {
		t.Fatal(replay, err)
	}
	at, _, _ := replay[0].Interval(cal.Location)
	if !at.Equal(start) {
		t.Fatal(at)
	}
	done := domain.StatusDone
	stored, err = cal.Update(t.Context(), e.ID, calendar.Patch{Status: &done}, start)
	if err != nil || stored.Recurrence == nil {
		t.Fatal(stored, err)
	}
	due, err := cal.Due(t.Context(), next)
	if err != nil || len(due) != 0 {
		t.Fatal("completed series fired", due, err)
	}
	open := domain.StatusOpen
	stored, err = cal.Update(t.Context(), e.ID, calendar.Patch{Status: &open}, next.Add(time.Minute))
	if err != nil || stored.Recurrence == nil {
		t.Fatal(stored, err)
	}
	if err := cal.Delete(t.Context(), e.ID); err != nil {
		t.Fatal(err)
	}
	due, err = cal.Due(t.Context(), next.Add(time.Hour))
	if err != nil || len(due) != 0 {
		t.Fatal("deleted series fired", due, err)
	}
	stored, err = cal.Restore(t.Context(), e.ID, next.Add(3*time.Hour), false)
	if err != nil || stored.Recurrence == nil || !stored.Recurrence.Anchor.Equal(e.Recurrence.Anchor) {
		t.Fatal(stored, err)
	}
	newTime, _ := domain.ParseTime("18:00")
	stored, err = cal.Update(t.Context(), e.ID, calendar.Patch{Time: &newTime}, now)
	if err != nil || stored.StartTime.String() != "18:00" {
		t.Fatal(stored, err)
	}
	rule := stored.Recurrence
	stored, err = cal.Update(t.Context(), e.ID, calendar.Patch{Recurrence: &rule}, now)
	if err != nil || !stored.Recurrence.Anchor.Equal(rule.Anchor) {
		t.Fatal("unchanged edit drifted anchor", stored, err)
	}
	var none *domain.Recurrence
	stored, err = cal.Update(t.Context(), e.ID, calendar.Patch{Recurrence: &none}, now)
	if err != nil || stored.Recurrence != nil || stored.OccurrenceAt != nil {
		t.Fatal(stored, err)
	}
}

func TestRecurringPendingPersistsWeekdayAndTimezone(t *testing.T) {
	for _, tc := range []struct {
		message, reply, frequency string
		day                       int
	}{
		{"каждый понедельник напомни отправить отчет", "в 9", "week", 0},
		{"15 числа каждого месяца напомни оплатить интернет", "09:30", "month", 15},
		{"каждый год 15 октября напомни поздравить друга", "в 10", "year", 15},
		{"напомни каждую неделю отправить отчет", "по пятницам в 9", "week", 0},
	} {
		t.Run(tc.message, func(t *testing.T) {
			db, cal, now := services(t)
			s := assistant.New(db, nlp.Hybrid{}, cal)
			r, err := s.Respond(t.Context(), tc.message, "pending", "first", now)
			if err != nil || !r.Pending {
				t.Fatal(r, err)
			}
			path := db.Path
			db.Close()
			db, err = sqlite.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			cal.Store = db
			s = assistant.New(db, nlp.Hybrid{}, cal)
			r, err = s.Respond(t.Context(), tc.reply, "pending", "reply", now)
			if err != nil || r.Pending || len(r.Events) != 1 {
				t.Fatal(r, err)
			}
			rule := r.Events[0].Recurrence
			if rule == nil || rule.Timezone != now.Location().String() || rule.Frequency != tc.frequency || rule.Day != tc.day {
				t.Fatal(rule)
			}
		})
	}
}

func TestStructuredRecurrenceAPIUpdatesAndValidation(t *testing.T) {
	_, _, router, _ := pgFixture(t)
	_, cookie := registerHTTP(t, router, "recurrence-api@example.test")
	input := map[string]any{"kind": "reminder", "title": "Вода", "recurrence": map[string]any{"frequency": "day", "interval": 1, "local_time": "09:00", "timezone": "Europe/Moscow"}}
	created, _ := call(t, router, cookie, "POST", "/api/events", input, 201)
	path := fmt.Sprintf("/api/events/%.0f", created["id"])
	changed, _ := call(t, router, cookie, "PATCH", path, map[string]any{"recurrence": map[string]any{"frequency": "week", "interval": 1, "weekdays": []int{2, 4}, "local_time": "18:00", "timezone": "Europe/Moscow"}}, 200)
	if changed["time"] != "18:00" || changed["recurrence"].(map[string]any)["frequency"] != "week" {
		t.Fatal(changed)
	}
	for _, rule := range []map[string]any{
		{"frequency": "hour", "interval": 0, "timezone": "Europe/Moscow"},
		{"frequency": "year", "interval": 1, "day_of_month": 31, "month": 2, "local_time": "09:00", "timezone": "Europe/Moscow"},
		{"frequency": "day", "interval": 1, "local_time": "09:00", "timezone": "Local"},
	} {
		call(t, router, cookie, "PATCH", path, map[string]any{"recurrence": rule}, 422)
	}
	call(t, router, cookie, "PATCH", path, map[string]any{"recurrence": nil}, 200)
}

func TestRecurringTaskExplicitIntentAndDateMoves(t *testing.T) {
	db, cal, now := services(t)
	s := assistant.New(db, nlp.Hybrid{}, cal)
	s.AskReminderSettings = true
	r, err := s.Respond(t.Context(), "создай задачу каждый день в 9 проверить встречи", "task-series", "first", now)
	if err != nil || r.Pending || len(r.Events) != 1 || r.Events[0].Kind != domain.EventTask || r.Events[0].Title != "Проверить встречи" {
		t.Fatal(r, err)
	}
	for _, tc := range []struct {
		frequency  string
		day, month int
		weekdays   []time.Weekday
		target     string
	}{
		{"week", 0, 0, []time.Weekday{time.Monday}, "2026-10-20"},
		{"month", 15, 0, nil, "2026-10-21"},
		{"year", 15, 10, nil, "2026-11-21"},
	} {
		t.Run(tc.frequency, func(t *testing.T) {
			clock, _ := domain.ParseTime("09:00")
			e, err := cal.Create(t.Context(), domain.Event{Kind: domain.EventReminder, Title: "Перенос серии", EventDetails: domain.EventDetails{Recurrence: &domain.Recurrence{Frequency: tc.frequency, Interval: 1, Day: tc.day, Month: time.Month(tc.month), Weekdays: tc.weekdays, LocalTime: clock, Timezone: now.Location().String()}}}, now, false)
			if err != nil {
				t.Fatal(err)
			}
			day, _ := domain.ParseDate(tc.target)
			e, err = cal.Update(t.Context(), e.ID, calendar.Patch{Date: &day}, now)
			if err != nil || e.Date.String() != tc.target {
				t.Fatal(e, err)
			}
		})
	}
}

func TestRelativeCanonicalInstantSurvivesDSTAndRestart(t *testing.T) {
	for _, tc := range []struct{ zone, now, text, want string }{
		{"Europe/Berlin", "2026-10-25T01:50:00+02:00", "через 20 минут напомни проверить", "2026-10-25T02:10:00+02:00"},
		{"Europe/Berlin", "2026-10-25T02:50:00+02:00", "через 20 минут напомни проверить", "2026-10-25T02:10:00+01:00"},
		{"Europe/Berlin", "2026-03-29T01:50:00+01:00", "через 30 минут напомни проверить", "2026-03-29T03:20:00+02:00"},
		{"Europe/Berlin", "2026-03-28T09:00:00+01:00", "через день и три часа напомни проверить", "2026-03-29T12:00:00+02:00"},
		{"Europe/Moscow", "2026-01-31T09:00:00+03:00", "через месяц напомни проверить", "2026-02-28T09:00:00+03:00"},
		{"Europe/Moscow", "2024-02-29T09:00:00+03:00", "через год напомни проверить", "2025-02-28T09:00:00+03:00"},
		{"Europe/Berlin", "2026-03-28T12:30:17+01:00", "через день утром напомни проверить", "2026-03-29T09:00:00+02:00"},
	} {
		t.Run(tc.text+tc.now, func(t *testing.T) {
			db, cal, _ := services(t)
			loc, _ := time.LoadLocation(tc.zone)
			cal.Location = loc
			now, _ := time.Parse(time.RFC3339, tc.now)
			now = now.In(loc)
			s := assistant.New(db, nlp.Hybrid{}, cal)
			r, err := s.Respond(t.Context(), tc.text, "dst", "first", now)
			if err != nil || len(r.Events) != 1 {
				t.Fatal(r, err)
			}
			path := db.Path
			db.Close()
			db, err = sqlite.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			cal.Store = db
			e, err := cal.Get(t.Context(), r.Events[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			got, _, _ := e.Interval(loc)
			if got.In(loc).Format(time.RFC3339) != tc.want {
				t.Fatal(got, tc.want)
			}
		})
	}
}
