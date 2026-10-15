package contract

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/api"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/clock"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/integrations/yandexcalendar"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

type calendarOAuthFake struct{ oauthFake }

func (p *calendarOAuthFake) CalendarAuthorizationURL(state, challenge string) string {
	return p.AuthorizationURL(state, challenge)
}
func (p *calendarOAuthFake) Grant(ctx context.Context, code, verifier, state string) (repository.OAuthGrant, error) {
	i, err := p.Identity(ctx, code, verifier, state)
	if err != nil {
		return repository.OAuthGrant{}, err
	}
	return repository.OAuthGrant{Identity: i, Login: "calendar-user@yandex.ru", Token: repository.OAuthToken{AccessToken: "private-access", RefreshToken: "private-refresh", Scope: "calendar:all"}}, nil
}

type onboardingRemote struct{ calls int }

func (r *onboardingRemote) EnsureCalendar(context.Context, string, string, string) (string, error) {
	return "https://calendar.test/gena/", nil
}
func (r *onboardingRemote) Put(context.Context, string, string, string, string) error {
	r.calls++
	return nil
}
func (r *onboardingRemote) Delete(context.Context, string, string, string) error { return nil }
func TestPostgresOnboardingOAuthReturnState(t *testing.T) {
	for _, tc := range []struct {
		source, code, providerError, want string
		complete                          bool
	}{
		{"onboarding", "valid", "", "/?calendar=connected", true},
		{"onboarding", "", "access_denied", "/onboarding?error=oauth_cancelled", false},
		{"onboarding", "invalid", "", "/onboarding?error=oauth_invalid", false},
		{"integrations", "valid", "", "/integrations?service=yandex&connected=1", false},
		{"integrations", "", "access_denied", "/integrations?service=yandex&error=oauth_cancelled", false},
		{"https://attacker.test/", "valid", "", "/integrations?service=yandex&connected=1", false},
	} {
		t.Run(tc.source+tc.providerError+tc.code, func(t *testing.T) {
			db, svc, _, now := pgFixture(t)
			svc.Providers["yandex"] = &calendarOAuthFake{oauthFake{subject: "onboarding-calendar", email: "oauth@example.test"}}
			remote := &onboardingRemote{}
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			syncService, err := yandexcalendar.New(db, db, remote, nil, "test-encryption-secret", now.Location(), logger)
			if err != nil {
				t.Fatal(err)
			}
			svc.Calendar = syncService
			cal := &calendar.Service{Store: db, Location: now.Location()}
			app := &api.API{Auth: svc, Calendar: cal, CalendarSync: syncService, Clock: clock.Func(func() time.Time { return now }), Config: config.Config{Root: root(), PublicURL: "https://gena.test"}, Logger: logger}
			router := app.Router()
			uid, cookie := registerHTTP(t, router, "onboarding-oauth@example.test")
			ctx := repository.WithUser(t.Context(), uid)
			createPushReminder(t, db, uid, now, nil)
			start := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/api/auth/yandex/start?calendar=1&source="+url.QueryEscape(tc.source), nil)
			req.AddCookie(cookie)
			router.ServeHTTP(start, req)
			if start.Code != 303 || strings.HasPrefix(start.Header().Get("Location"), "/integrations") {
				t.Fatal("did not start OAuth", start.Code)
			}
			target, _ := url.Parse(start.Header().Get("Location"))
			query := target.Query()
			query.Set("code", tc.code)
			query.Set("error", tc.providerError)
			callback := httptest.NewRecorder()
			req = httptest.NewRequest("GET", "/api/auth/yandex/callback?"+query.Encode(), nil)
			req.AddCookie(cookie)
			req.AddCookie(start.Result().Cookies()[0])
			router.ServeHTTP(callback, req)
			if callback.Code != 303 || callback.Header().Get("Location") != tc.want {
				t.Fatal("bad return", callback.Code, callback.Header().Get("Location"))
			}
			me, _ := call(t, router, cookie, "GET", "/api/auth/me", nil, 200)
			if me["user"].(map[string]any)["onboarding_completed"] != tc.complete {
				t.Fatal("bad onboarding completion")
			}
			if tc.code == "valid" {
				credentials, err := db.CalendarCredentials(ctx)
				if err != nil || credentials == nil || credentials.Status != "pending" {
					t.Fatal("connection not persisted", err)
				}
				if err = syncService.Sync(ctx); err != nil {
					t.Fatal("initial sync", err)
				}
				connections, _ := db.Connections(ctx)
				if connections[0].Status != "connected" || remote.calls != 1 {
					t.Fatal("initial sync not complete", connections)
				}
			}
		})
	}
}
func TestPostgresOnboardingOAuthUnavailableAndSafeSource(t *testing.T) {
	_, svc, router, now := pgFixture(t)
	_, cookie := registerHTTP(t, router, "unavailable-onboarding@example.test")
	start := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/auth/yandex/start?calendar=1&source=onboarding", nil)
	req.AddCookie(cookie)
	router.ServeHTTP(start, req)
	if start.Header().Get("Location") != "/onboarding?error=calendar_unavailable" {
		t.Fatal("bad unavailable return")
	}
	if _, _, err := svc.StartCalendarOAuthFrom(t.Context(), "user", "https://attacker.test/", now); err == nil {
		t.Fatal(fmt.Sprint("arbitrary return source accepted"))
	}
}
