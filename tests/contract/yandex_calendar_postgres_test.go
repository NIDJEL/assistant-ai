package contract

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/api"
	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/clock"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/integrations/oauth"
	"github.com/NIDJEL/assistant-ai/internal/integrations/yandexcalendar"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/search"
)

func TestPostgresYandexCalendarOAuthAndDurableSync(t *testing.T) {
	db, authService, _, now := pgFixture(t)
	loc := now.Location()
	engine, _ := search.New()
	cal := &calendar.Service{Store: db, Search: engine, Location: loc}
	ctx := context.Background()
	u, err := authService.Register(ctx, "Календарь", "calendar@example.test", "correct password 123", "correct password 123")
	if err != nil {
		t.Fatal(err)
	}
	ctx = repository.WithUser(ctx, u.ID)
	other, err := authService.Register(context.Background(), "Другой", "other-calendar@example.test", "correct password 123", "correct password 123")
	if err != nil {
		t.Fatal(err)
	}
	otherCtx := repository.WithUser(context.Background(), other.ID)
	date, _ := domain.ParseDate("2026-10-14")
	event, err := cal.Create(ctx, domain.Event{Kind: domain.EventTask, Title: "Подготовить отчёт", Date: date}, now, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cal.Create(ctx, domain.Event{Kind: domain.EventTask, Title: "Без даты"}, now, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cal.Create(otherCtx, domain.Event{Kind: domain.EventTask, Title: "Чужие дела", Date: date}, now, false)
	if err != nil {
		t.Fatal(err)
	}
	resources := map[string]string{}
	calendarCreated := false
	canonicalCalendar := "/calendars/events-42/"
	failNext := false
	rejectAccess := false
	rejectAll := false
	refreshCalls := 0
	var duringPut func()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_ = r.ParseForm()
			if r.Form.Get("grant_type") == "refresh_token" {
				refreshCalls++
				if r.Form.Get("refresh_token") != "refresh-secret" {
					t.Error("wrong refresh token")
				}
			} else if len(r.Form.Get("code_verifier")) != 43 {
				t.Error("PKCE missing")
			}
			access := "access-secret"
			if refreshCalls > 0 {
				access = "refreshed-secret"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": access, "refresh_token": "refresh-secret", "expires_in": 3600, "scope": "login:info login:email calendar:all"})
			return
		}
		if r.URL.Path == "/info" {
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "calendar-yandex-id", "login": "calendar-user", "default_email": "calendar-user@yandex.ru", "real_name": "Календарь"})
			return
		}
		login, token, ok := r.BasicAuth()
		if !ok || login != "calendar-user@yandex.ru" || token != "access-secret" && token != "refreshed-secret" {
			t.Error("invalid CalDAV credentials")
		}
		if rejectAll || rejectAccess && token == "access-secret" {
			w.WriteHeader(401)
			return
		}
		switch r.Method {
		case "PROPFIND":
			var prop string
			if r.URL.Path == "/" {
				prop = `<d:current-user-principal><d:href>/principal/</d:href></d:current-user-principal>`
			} else if r.URL.Path == "/principal/" {
				prop = `<c:calendar-home-set><d:href>/calendars/</d:href></c:calendar-home-set>`
			} else {
				prop = `<d:resourcetype><d:collection/></d:resourcetype>`
			}
			w.WriteHeader(207)
			body := `<d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:response><d:href>` + r.URL.Path + `</d:href><d:propstat><d:prop>` + prop + `</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`
			if r.URL.Path == "/calendars/" && calendarCreated {
				body += `<d:response><d:href>` + canonicalCalendar + `</d:href><d:propstat><d:prop><d:displayname>Ритм</d:displayname><d:resourcetype><c:calendar/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`
			}
			_, _ = io.WriteString(w, body+`</d:multistatus>`)
		case "MKCALENDAR":
			calendarCreated = true
			w.WriteHeader(201)
		case "PUT":
			if !strings.HasPrefix(r.URL.Path, canonicalCalendar) {
				w.WriteHeader(409)
				return
			}
			if failNext {
				failNext = false
				w.WriteHeader(503)
				return
			}
			body, _ := io.ReadAll(r.Body)
			uid := ""
			for _, line := range strings.Split(string(body), "\r\n") {
				if strings.HasPrefix(line, "UID:") {
					uid = strings.TrimPrefix(line, "UID:")
				}
			}
			// Yandex names the stored resource after UID even when PUT used a
			// different filename. A later DELETE must address the canonical name.
			resources[canonicalCalendar+uid+".ics"] = string(body)
			if duringPut != nil {
				callback := duringPut
				duringPut = nil
				callback()
			}
			w.WriteHeader(201)
		case "DELETE":
			delete(resources, r.URL.Path)
			w.WriteHeader(204)
		default:
			w.WriteHeader(405)
		}
	}))
	defer server.Close()
	provider := &oauth.Provider{Name: "yandex", ClientID: "client", ClientSecret: "secret", RedirectURI: "https://rhythm.test/api/auth/yandex/callback", AuthorizeEndpoint: server.URL + "/authorize", TokenEndpoint: server.URL + "/token", UserEndpoint: server.URL + "/info", Client: server.Client()}
	syncer, err := yandexcalendar.New(db, db, &yandexcalendar.DAV{BaseURL: server.URL + "/", Client: server.Client()}, provider, "test-only-session-secret-at-least-32-characters", loc, nil)
	if err != nil {
		t.Fatal(err)
	}
	authService.Providers["yandex"] = provider
	authService.Calendar = syncer
	app := &api.API{Config: config.Config{Root: root(), Timezone: "Europe/Moscow", PublicURL: "https://rhythm.test", Environment: "production"}, Clock: clock.Func(func() time.Time { return now }), Calendar: cal, CalendarSync: syncer, Auth: authService, Assistant: assistant.New(db, nlp.Hybrid{}, cal), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	router := app.Router()
	session, err := authService.NewSession(ctx, u.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: "rhythm_session", Value: session}
	call(t, router, nil, "POST", "/api/integrations/yandex/sync", map[string]any{}, 401)
	start := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/auth/yandex/start?calendar=1", nil)
	request.AddCookie(cookie)
	router.ServeHTTP(start, request)
	target, _ := url.Parse(start.Header().Get("Location"))
	q := target.Query()
	if !strings.Contains(q.Get("scope"), "calendar:all") || q.Get("force_confirm") != "yes" {
		t.Fatal("calendar permission missing")
	}
	q.Set("code", "valid")
	request = httptest.NewRequest("GET", "/api/auth/yandex/callback?"+q.Encode(), nil)
	request.AddCookie(cookie)
	request.AddCookie(start.Result().Cookies()[0])
	callback := httptest.NewRecorder()
	router.ServeHTTP(callback, request)
	if callback.Header().Get("Location") != "/integrations?service=yandex&connected=1" {
		t.Fatal("calendar callback failed", callback.Header().Get("Location"))
	}
	for _, c := range callback.Result().Cookies() {
		if c.Name == "rhythm_session" {
			t.Fatal("calendar callback replaced login session")
		}
	}
	credentials, err := db.CalendarCredentials(ctx)
	if err != nil || credentials == nil || strings.Contains(string(credentials.Ciphertext), "access-secret") {
		t.Fatal("unencrypted credentials")
	}
	connections, err := db.Connections(otherCtx)
	if err != nil || len(connections) != 0 {
		t.Fatal("cross-user connection")
	}
	data, _ := call(t, router, cookie, "GET", "/api/integrations", nil, 200)
	encoded, _ := json.Marshal(data)
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "calendar-user") {
		t.Fatal("private credentials exposed")
	}
	call(t, router, cookie, "POST", "/api/integrations/yandex/sync", map[string]any{}, 200)
	if len(resources) != 1 {
		t.Fatal("undated or foreign event exported", len(resources))
	}
	eventPath := fmt.Sprintf("%srhythm-%s-%d@rhythm.ics", canonicalCalendar, u.ID, event.ID)
	if !strings.Contains(resources[eventPath], "DTSTART;VALUE=DATE:20261014") {
		t.Fatal("all-day event missing")
	}
	// Repair the invalid path persisted by older versions without reconnecting
	// or creating a duplicate calendar/event.
	if err := db.SetCalendarTarget(ctx, server.URL+"/calendars/rhythm-"+u.ID+"/"); err != nil {
		t.Fatal(err)
	}
	if err := db.CalendarSyncStatus(ctx, "error", "previous address invalid", nil); err != nil {
		t.Fatal(err)
	}
	call(t, router, cookie, "PATCH", fmt.Sprintf("/api/events/%d", event.ID), map[string]any{"title": "Изменённый отчёт", "time": "10:30"}, 200)
	failNext = true
	call(t, router, cookie, "POST", "/api/integrations/yandex/sync", map[string]any{}, 502)
	changes, _ := db.CalendarChanges(ctx, 100)
	if len(changes) != 1 {
		t.Fatal("failed sync lost change")
	}
	call(t, router, cookie, "POST", "/api/integrations/yandex/sync", map[string]any{}, 200)
	if len(resources) != 1 || !strings.Contains(resources[eventPath], "DTSTART:20261014T073000Z") {
		t.Fatal("move duplicated event or wrong timezone")
	}
	title := "Изменение во время отправки"
	duringPut = func() {
		if _, err := cal.Update(ctx, event.ID, calendar.Patch{Title: &title}, now.Add(time.Minute)); err != nil {
			t.Error(err)
		}
	}
	call(t, router, cookie, "PATCH", fmt.Sprintf("/api/events/%d", event.ID), map[string]any{"description": "Проверить версию"}, 200)
	call(t, router, cookie, "POST", "/api/integrations/yandex/sync", map[string]any{}, 200)
	changes, _ = db.CalendarChanges(ctx, 100)
	if len(changes) != 1 {
		t.Fatal("concurrent edit was acknowledged prematurely")
	}
	call(t, router, cookie, "POST", "/api/integrations/yandex/sync", map[string]any{}, 200)
	if !strings.Contains(resources[eventPath], "SUMMARY:"+title) {
		t.Fatal("concurrent edit not exported")
	}
	call(t, router, cookie, "PATCH", fmt.Sprintf("/api/events/%d", event.ID), map[string]any{"reminder_enabled": true, "reminder_minutes": 60}, 200)
	call(t, router, cookie, "POST", "/api/integrations/yandex/sync", map[string]any{}, 200)
	if !strings.Contains(resources[eventPath], "TRIGGER:-PT60M") || !strings.Contains(resources[eventPath], "DTSTART:20261014T073000Z") {
		t.Fatal("early alarm changed event time or was not synced")
	}
	call(t, router, cookie, "PATCH", fmt.Sprintf("/api/events/%d", event.ID), map[string]any{"reminder_enabled": false}, 200)
	call(t, router, cookie, "POST", "/api/integrations/yandex/sync", map[string]any{}, 200)
	if strings.Contains(resources[eventPath], "BEGIN:VALARM") {
		t.Fatal("disabled alarm remains at provider")
	}
	call(t, router, cookie, "PATCH", fmt.Sprintf("/api/events/%d", event.ID), map[string]any{"status": "done"}, 200)
	rejectAccess = true
	call(t, router, cookie, "POST", "/api/integrations/yandex/sync", map[string]any{}, 200)
	if refreshCalls != 1 || !strings.Contains(resources[eventPath], "SUMMARY:✓ ") {
		t.Fatal("refresh or completion failed")
	}
	call(t, router, cookie, "DELETE", fmt.Sprintf("/api/events/%d", event.ID), nil, 204)
	call(t, router, cookie, "POST", "/api/integrations/yandex/sync", map[string]any{}, 200)
	if len(resources) != 0 {
		t.Fatal("deleted event remains")
	}
	call(t, router, cookie, "POST", fmt.Sprintf("/api/events/%d/restore", event.ID), map[string]any{}, 200)
	call(t, router, cookie, "POST", "/api/integrations/yandex/sync", map[string]any{}, 200)
	if len(resources) != 1 {
		t.Fatal("restore failed")
	}
	call(t, router, cookie, "PATCH", fmt.Sprintf("/api/events/%d", event.ID), map[string]any{"date": nil, "time": nil}, 200)
	call(t, router, cookie, "POST", "/api/integrations/yandex/sync", map[string]any{}, 200)
	if len(resources) != 0 {
		t.Fatal("undated event remains in calendar")
	}
	chat, _ := call(t, router, cookie, "POST", "/api/chat", map[string]any{"message": "Привет добавь Запись к врачу на завтра в 9:00 утра"}, 200)
	if chat["events"].([]any)[0].(map[string]any)["title"] != "Запись к врачу" {
		t.Fatal("greeting retained in saved event title")
	}
	call(t, router, cookie, "POST", "/api/integrations/yandex/sync", map[string]any{}, 200)
	if len(resources) != 1 {
		t.Fatal("assistant write not exported")
	}
	rejectAll = true
	call(t, router, cookie, "PATCH", fmt.Sprintf("/api/events/%d", event.ID), map[string]any{"date": "2026-10-15"}, 200)
	call(t, router, cookie, "POST", "/api/integrations/yandex/sync", map[string]any{}, 502)
	credentials, _ = db.CalendarCredentials(ctx)
	if credentials.Status != "reauthorization_required" {
		t.Fatal("revoked access was retried indefinitely")
	}
	users, _ := db.CalendarUsers(context.Background())
	for _, id := range users {
		if id == u.ID {
			t.Fatal("revoked connection still in worker list")
		}
	}
	changes, _ = db.CalendarChanges(ctx, 100)
	if len(changes) == 0 {
		t.Fatal("revoked access lost local changes")
	}
	call(t, router, cookie, "DELETE", "/api/integrations/yandex", nil, 204)
	credentials, _ = db.CalendarCredentials(ctx)
	if len(credentials.Ciphertext) != 0 || credentials.Status != "disconnected" {
		t.Fatal("disconnect retained credentials")
	}
	if len(resources) != 1 {
		t.Fatal("disconnect deleted user's calendar data")
	}
	_, err = cal.Create(ctx, domain.Event{Kind: domain.EventTask, Title: "После отключения", Date: date}, now, false)
	if err != nil {
		t.Fatal(err)
	}
	changes, _ = db.CalendarChanges(ctx, 100)
	if len(changes) != 0 {
		t.Fatal("disconnected event queued")
	}
}

type calendarGrantFake struct{ *oauthFake }

func (p *calendarGrantFake) CalendarAuthorizationURL(state, challenge string) string {
	return p.AuthorizationURL(state, challenge)
}
func (p *calendarGrantFake) Grant(ctx context.Context, code, verifier, state, device string) (repository.OAuthGrant, error) {
	i, err := p.Identity(ctx, code, verifier, state, device)
	return repository.OAuthGrant{Identity: i, Login: "alice@yandex.ru", Token: repository.OAuthToken{AccessToken: "access", Scope: "calendar:all"}}, err
}

type calendarConnectorFake struct {
	calls  int
	userID string
}

func (c *calendarConnectorFake) Connect(ctx context.Context, grant repository.OAuthGrant, now time.Time) error {
	c.calls++
	c.userID = repository.UserID(ctx)
	return nil
}

func TestPostgresCalendarOAuthBoundToCurrentUserAndOneUseState(t *testing.T) {
	_, svc, _, now := pgFixture(t)
	first, err := svc.Register(context.Background(), "Первый", "calendar-first@example.test", "correct password 123", "correct password 123")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Register(context.Background(), "Второй", "calendar-second@example.test", "correct password 123", "correct password 123")
	if err != nil {
		t.Fatal(err)
	}
	connector := &calendarConnectorFake{}
	svc.Calendar = connector
	svc.Providers["yandex"] = &calendarGrantFake{oauthFake: &oauthFake{subject: "calendar-subject", email: "alice@yandex.ru"}}
	for _, current := range []string{"", second.ID} {
		target, browser, err := svc.StartCalendarOAuth(context.Background(), first.ID, now)
		if err != nil {
			t.Fatal(err)
		}
		parsed, _ := url.Parse(target)
		q := parsed.Query()
		q.Set("code", "valid")
		if _, err = svc.FinishOAuthFlow(context.Background(), "yandex", browser, current, q, now); err == nil {
			t.Fatal("calendar connected without matching session")
		}
	}
	if connector.calls != 0 {
		t.Fatal("calendar tokens saved for wrong user")
	}
	target, browser, err := svc.StartCalendarOAuth(context.Background(), first.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(target)
	q := parsed.Query()
	q.Set("code", "valid")
	result, err := svc.FinishOAuthFlow(context.Background(), "yandex", browser, first.ID, q, now)
	if err != nil || !result.Calendar || connector.calls != 1 || connector.userID != first.ID {
		t.Fatal("valid calendar flow failed", err)
	}
	if _, err = svc.FinishOAuthFlow(context.Background(), "yandex", browser, first.ID, q, now); err == nil {
		t.Fatal("calendar state replay accepted")
	}
}

func TestPostgresCalendarBackfillIncludesConcurrentCreate(t *testing.T) {
	db, svc, _, now := pgFixture(t)
	u, err := svc.Register(context.Background(), "Параллельный", "calendar-backfill@example.test", "correct password 123", "correct password 123")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(repository.WithUser(context.Background(), u.ID), 5*time.Second)
	defer cancel()
	created := make(chan struct{})
	commit := make(chan struct{})
	writeDone := make(chan error, 1)
	go func() {
		writeDone <- db.Write(ctx, func(tx repository.Tx) error {
			day, _ := domain.ParseDate("2026-10-14")
			_, err := tx.Create(ctx, domain.Event{Kind: domain.EventTask, Title: "Во время подключения", Date: day, Duration: time.Hour, Status: domain.StatusOpen, Priority: domain.PriorityNormal}, now)
			close(created)
			if err != nil {
				return err
			}
			select {
			case <-commit:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	<-created
	saved := make(chan error, 1)
	go func() {
		saved <- db.SaveCalendarCredentials(ctx, repository.CalendarCredentials{Subject: "subject", Login: "login", Ciphertext: []byte("encrypted")})
	}()
	var saveErr error
	early := false
	select {
	case saveErr = <-saved:
		early = true
		t.Error("connection backfill did not wait for in-flight event write")
	case <-time.After(80 * time.Millisecond):
	}
	close(commit)
	if err = <-writeDone; err != nil {
		t.Fatal(err)
	}
	if !early {
		saveErr = <-saved
	}
	if saveErr != nil {
		t.Fatal(saveErr)
	}
	changes, err := db.CalendarChanges(ctx, 100)
	if err != nil || len(changes) != 1 {
		t.Fatal("concurrent event omitted from initial sync", len(changes), err)
	}
}
