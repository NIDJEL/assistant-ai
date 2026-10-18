package contract

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/integrations/cloud"
	"github.com/NIDJEL/assistant-ai/internal/integrations/oauth"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

func TestPostgresCloudSyncZoomAndIsolation(t *testing.T) {
	db, auth, _, now := pgFixture(t)
	u, err := auth.Register(context.Background(), "Cloud", "cloud@example.test", "correct password 123", "correct password 123")
	if err != nil {
		t.Fatal(err)
	}
	ctx := repository.WithUser(context.Background(), u.ID)
	remote := map[string]map[string]any{}
	zoomCreates, zoomUpdates := 0, 0
	var zoomStart string
	incoming := []map[string]any{}
	tokenCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/token" {
			tokenCalls++
			fmt.Fprint(w, `{"access_token":"test-new-private-access","refresh_token":"test-rotated-private-refresh","expires_in":3600}`)
			return
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("missing authorization")
		}
		switch {
		case r.URL.Path == "/users/me/calendarList":
			fmt.Fprint(w, `{"items":[{"id":"primary","summary":"Personal","accessRole":"owner"}]}`)
		case r.URL.Path == "/calendars/primary/events" && r.Method == "GET":
			json.NewEncoder(w).Encode(map[string]any{"items": incoming, "nextSyncToken": "cursor-1"})
			incoming = nil
		case r.URL.Path == "/calendars/primary/events" && r.Method == "POST":
			var v map[string]any
			json.NewDecoder(r.Body).Decode(&v)
			id, _ := v["id"].(string)
			if remote[id] != nil {
				w.WriteHeader(409)
				return
			}
			v["etag"] = "etag-1"
			remote[id] = v
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(v)
		case strings.HasPrefix(r.URL.Path, "/calendars/primary/events/"):
			id := strings.TrimPrefix(r.URL.Path, "/calendars/primary/events/")
			v := remote[id]
			if v == nil {
				w.WriteHeader(404)
				return
			}
			if r.Method == "DELETE" {
				delete(remote, id)
				w.WriteHeader(204)
				return
			}
			if r.Method == "PATCH" {
				var patch map[string]any
				json.NewDecoder(r.Body).Decode(&patch)
				for k, x := range patch {
					v[k] = x
				}
				v["etag"] = "etag-2"
			}
			json.NewEncoder(w).Encode(v)
		case r.URL.Path == "/users/me/meetings" && r.Method == "POST":
			zoomCreates++
			var v map[string]any
			json.NewDecoder(r.Body).Decode(&v)
			zoomStart, _ = v["start_time"].(string)
			fmt.Fprint(w, `{"id":12345,"join_url":"https://us02web.zoom.us/j/12345"}`)
		case r.URL.Path == "/meetings/12345" && r.Method == "PATCH":
			zoomUpdates++
			var v map[string]any
			json.NewDecoder(r.Body).Decode(&v)
			zoomStart, _ = v["start_time"].(string)
			w.WriteHeader(204)
		case r.URL.Path == "/meetings/12345" && r.Method == "GET":
			fmt.Fprint(w, `{"id":12345,"join_url":"https://us02web.zoom.us/j/12345"}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	providers := map[string]*oauth.Provider{"google": {Name: "google", TokenEndpoint: server.URL + "/token"}, "zoom": {Name: "zoom", TokenEndpoint: server.URL + "/token"}}
	svc, err := cloud.New(db, providers, strings.Repeat("private-test-key", 3), now.Location())
	if err != nil {
		t.Fatal(err)
	}
	svc.GoogleBase, svc.ZoomBase = server.URL, server.URL
	for _, p := range []string{"google", "zoom"} {
		g := repository.OAuthGrant{Identity: repository.Identity{Provider: p, Subject: "subject-" + p}, Login: "cloud@example.test", Token: repository.OAuthToken{AccessToken: "private-access", RefreshToken: "private-refresh", ExpiresIn: 3600, Scope: "https://www.googleapis.com/auth/calendar.events https://www.googleapis.com/auth/calendar.calendarlist.readonly"}}
		if err = svc.Connect(ctx, g, now); err != nil {
			t.Fatal(err)
		}
	}
	c, err := db.CloudConnection(ctx, "google")
	if err != nil || strings.Contains(string(c.Ciphertext), "private-access") {
		t.Fatal("unencrypted token", err)
	}
	if err = svc.SelectCalendar(ctx, "primary"); err != nil {
		t.Fatal(err)
	}
	cal := &calendar.Service{Store: db, Location: now.Location()}
	day := domain.Day(now.AddDate(0, 0, 1))
	clock := &domain.TimeOfDay{Hour: 17}
	e, err := cal.Create(ctx, domain.Event{EventDetails: domain.EventDetails{CalendarProvider: "google", ConferenceProvider: "zoom"}, Kind: domain.EventMeeting, Title: "Cloud test", Date: day, StartTime: clock}, now, false)
	if err != nil {
		t.Fatal(err)
	}
	current, err := svc.SyncEvent(ctx, e.ID, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if current.MeetingURL == "" || zoomCreates != 1 || len(remote) != 1 {
		t.Fatalf("not integrated: creates %d remote %d url %s", zoomCreates, len(remote), current.MeetingURL)
	}
	for _, v := range remote {
		if !strings.Contains(fmt.Sprint(v["description"]), current.MeetingURL) {
			t.Fatal("Zoom URL missing in Google")
		}
	}
	if _, err = svc.SyncEvent(ctx, e.ID, true, true); err != nil || zoomCreates != 1 {
		t.Fatal("retry duplicated Zoom", err)
	}
	err = db.Write(ctx, func(tx repository.Tx) error {
		v, err := tx.Get(ctx, e.ID)
		if err != nil {
			return err
		}
		v.StartTime = &domain.TimeOfDay{Hour: 18}
		_, err = tx.Update(ctx, *v, now)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.SyncEvent(ctx, e.ID, true, true); err != nil {
		t.Fatal(err)
	}
	if zoomUpdates == 0 || !strings.Contains(zoomStart, "15:00:00Z") {
		t.Fatal("Zoom reschedule not applied", zoomStart)
	}
	incoming = []map[string]any{{"id": "remote-import", "etag": "one", "summary": "From Google", "start": map[string]string{"dateTime": day.String() + "T10:00:00+03:00"}, "end": map[string]string{"dateTime": day.String() + "T11:00:00+03:00"}}}
	if err = svc.Sync(ctx, "google"); err != nil {
		t.Fatal(err)
	}
	var imported *domain.Event
	err = db.Read(ctx, func(tx repository.Tx) error {
		list, e := tx.Events(ctx, domain.Filter{})
		for i := range list {
			if list[i].Title == "From Google" {
				imported = &list[i]
			}
		}
		return e
	})
	if err != nil || imported == nil {
		t.Fatal("import failed", err)
	}
	incoming = []map[string]any{{"id": "remote-import", "etag": "two", "status": "cancelled"}}
	if err = svc.Sync(ctx, "google"); err != nil {
		t.Fatal(err)
	}
	db.Read(ctx, func(tx repository.Tx) error {
		v, err := tx.Get(ctx, imported.ID)
		if v != nil {
			t.Error("remote deletion not imported")
		}
		return err
	})
	other, err := auth.Register(context.Background(), "Other", "other-cloud@example.test", "correct password 123", "correct password 123")
	if err != nil {
		t.Fatal(err)
	}
	otherCtx := repository.WithUser(context.Background(), other.ID)
	if c, err := db.CloudConnection(otherCtx, "google"); err != nil || c != nil {
		t.Fatal("cross user connection leak")
	}
	if err = svc.Disconnect(ctx, "google"); err != nil {
		t.Fatal(err)
	}
	c, err = db.CloudConnection(ctx, "google")
	if err != nil || len(c.Ciphertext) != 0 || c.Status != "disconnected" {
		t.Fatal("disconnect retained token")
	}
	if tokenCalls == 0 {
		t.Fatal("expired credentials not refreshed")
	}
}
