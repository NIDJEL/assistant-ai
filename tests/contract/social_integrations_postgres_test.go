package contract

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/integrations/cloud"
	"github.com/NIDJEL/assistant-ai/internal/integrations/oauth"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/social"
	"github.com/google/uuid"
)

func TestPostgresSharedCalendarExportsAndIncompleteAvailability(t *testing.T) {
	db, auth, _, now := pgFixture(t)
	u, _ := auth.Register(t.Context(), "Host", "shared-host@example.test", "password 12345", "password 12345")
	v, _ := auth.Register(t.Context(), "Guest", "shared-guest@example.test", "password 12345", "password 12345")
	a, b := repository.WithUser(t.Context(), u.ID), repository.WithUser(t.Context(), v.ID)
	execute := func(ctx context.Context, op social.Operation) social.Result {
		t.Helper()
		op.RequestID = uuid.NewString()
		var result social.Result
		err := db.Write(ctx, func(tx repository.Tx) error {
			s, _ := social.In(ctx, tx, now.Location())
			var e error
			result, e = s.Execute(social.WithTx(ctx, tx), op, now)
			return e
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	var code string
	db.DB.QueryRow(`SELECT friend_code FROM users WHERE id=$1`, v.ID).Scan(&code)
	r := execute(a, social.Operation{Action: "request", Code: code})
	execute(b, social.Operation{Action: "accept_request", ID: r.ID})
	day := domain.Day(now.AddDate(0, 0, 1))
	e := domain.Event{Kind: domain.EventMeeting, Title: "Shared export", Date: day, StartTime: &domain.TimeOfDay{Hour: 15}, Duration: 30 * time.Minute, Status: domain.StatusOpen, Priority: domain.PriorityNormal}
	r = execute(a, social.Operation{Action: "create_meeting", Event: &e, FriendIDs: []string{v.ID}})
	// Configure existing provider connections without publishing credentials.
	db.DB.Exec(`INSERT INTO calendar_connections(id,user_id,provider,status) VALUES($1,$2,'yandex','connected')`, uuid.NewString(), v.ID)
	db.DB.Exec(`INSERT INTO cloud_connections(user_id,provider,subject,status,calendar_id) VALUES($1,'google','guest','connected','primary')`, v.ID)
	execute(b, social.Operation{Action: "respond", EventID: r.EventID, Version: 1, Status: "accepted"})
	for _, table := range []string{"calendar_outbox", "cloud_outbox"} {
		var count int
		if err := db.DB.QueryRow("SELECT count(*) FROM "+table+" WHERE user_id=$1 AND event_id=$2", v.ID, r.EventID).Scan(&count); err != nil || count != 1 {
			t.Fatal("shared event not enqueued", table, count, err)
		}
	}
	var av social.Availability
	err := db.Read(a, func(tx repository.Tx) error {
		s, _ := social.In(a, tx, now.Location())
		var err error
		av, err = s.Availability(a, []string{v.ID}, social.Slot{Start: now.AddDate(0, 0, 2), End: now.AddDate(0, 0, 2).Add(30 * time.Minute)}, 0, now)
		return err
	})
	if err != nil || av.Complete || av.Users[1].Status != "unknown" {
		t.Fatal("external outage falsely reports free", av, err)
	}
	// Guest edits imported from Google cannot alter the organizer's canonical event.
	if err = db.SaveCloudLink(b, "google", "primary", repository.CloudLink{EventID: r.EventID, RemoteID: "shared-remote", ETag: "one"}); err != nil {
		t.Fatal(err)
	}
	incoming := e
	incoming.Title = "ATTENDEE CANNOT RENAME"
	incoming.StartTime = &domain.TimeOfDay{Hour: 18}
	if err = db.ImportGoogleEvent(b, "primary", repository.CloudLink{RemoteID: "shared-remote", ETag: "two"}, &incoming, false); err != nil {
		t.Fatal(err)
	}
	cal := calendar.Service{Store: db, Location: now.Location()}
	canonical, err := cal.Get(a, r.EventID)
	if err != nil || canonical.Title != "Shared export" || canonical.StartTime.Hour != 15 {
		t.Fatal("attendee import mutated organizer", canonical, err)
	}
	privateTags := []string{"PRIVATE ORGANIZER TAG"}
	if _, err = cal.Update(a, r.EventID, calendar.Patch{Tags: &privateTags}, now); err != nil {
		t.Fatal(err)
	}
	execute(a, social.Operation{Action: "cancel_invitation", EventID: r.EventID, Version: 1, UserID: v.ID})
	title := "Private new agenda"
	if _, err = cal.Update(a, r.EventID, calendar.Patch{Title: &title}, now); err != nil {
		t.Fatal(err)
	}
	err = db.Read(b, func(tx repository.Tx) error {
		s, _ := social.In(b, tx, now.Location())
		m, e := s.Meeting(b, tx, r.EventID)
		if e != nil {
			return e
		}
		if m.Event.Title != "Shared export" || !m.Cancelled {
			t.Fatal("cancelled invitation leaked later changes", m)
		}
		if len(m.Event.Tags) != 0 || len(m.Event.Participants) != 0 || m.Event.ReminderMinutes != nil {
			t.Fatal("organizer metadata leaked in cancelled snapshot", m.Event)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPostgresGooglePlanningReadsAllCalendarsAndMapsSharedIDs(t *testing.T) {
	db, auth, _, now := pgFixture(t)
	u, _ := auth.Register(t.Context(), "Calendar", "planning-google@example.test", "password 12345", "password 12345")
	ctx := repository.WithUser(t.Context(), u.ID)
	cal := calendar.Service{Store: db, Location: now.Location()}
	event, err := cal.Create(ctx, domain.Event{Kind: domain.EventMeeting, Title: "Known", Date: domain.Day(now.AddDate(0, 0, 1)), StartTime: &domain.TimeOfDay{Hour: 15}}, now, false)
	if err != nil {
		t.Fatal(err)
	}
	readCalendars := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer private-access" {
			t.Error("bad authorization")
		}
		switch r.URL.Path {
		case "/users/me/calendarList":
			fmt.Fprint(w, `{"items":[{"id":"primary","accessRole":"owner"},{"id":"other","accessRole":"reader"}]}`)
		case "/calendars/primary/events", "/calendars/other/events":
			readCalendars++
			fields := r.URL.Query().Get("fields")
			if strings.Contains(fields, "summary") || strings.Contains(fields, "description") || strings.Contains(fields, "attendees") {
				t.Error("private fields requested", fields)
			}
			fmt.Fprint(w, `{"items":[{"id":"remote-known","start":{"dateTime":"2026-10-14T15:00:00+03:00"},"end":{"dateTime":"2026-10-14T16:00:00+03:00"}},{"id":"transparent","start":{"dateTime":"2026-10-14T16:00:00+03:00"},"end":{"dateTime":"2026-10-14T17:00:00+03:00"},"transparency":"transparent"}]}`)
		default:
			t.Error(r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	svc, err := cloud.New(db, map[string]*oauth.Provider{"google": {Name: "google"}}, strings.Repeat("test-secret", 4), now.Location())
	if err != nil {
		t.Fatal(err)
	}
	svc.GoogleBase = server.URL
	grant := repository.OAuthGrant{Identity: repository.Identity{Provider: "google", Subject: "subject"}, Token: repository.OAuthToken{AccessToken: "private-access", RefreshToken: "private-refresh", ExpiresIn: 3600, Scope: "https://www.googleapis.com/auth/calendar.events https://www.googleapis.com/auth/calendar.calendarlist.readonly"}}
	if err = svc.Connect(ctx, grant, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = db.SaveCloudLink(ctx, "google", "primary", repository.CloudLink{EventID: event.ID, RemoteID: "remote-known"}); err != nil {
		t.Fatal(err)
	}
	busy, err := svc.PlanningBusy(ctx, now, now.AddDate(0, 0, 14))
	if err != nil || len(busy) != 2 || readCalendars != 2 || busy[0].EventID != event.ID {
		t.Fatal(busy, readCalendars, err)
	}
	raw, _ := json.Marshal(busy)
	if strings.Contains(string(raw), "private-") {
		t.Fatal("credentials or remote text leaked")
	}
}
