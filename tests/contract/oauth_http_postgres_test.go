package contract

import (
	"bytes"
	"context"
	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/search"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPostgresOAuthHTTPCallbackAndSession(t *testing.T) {
	_, _, router, _ := pgFixture(t)
	start := httptest.NewRecorder()
	router.ServeHTTP(start, httptest.NewRequest("GET", "/api/auth/yandex/start", nil))
	if start.Code != 303 {
		t.Fatal(start.Code)
	}
	browser := start.Result().Cookies()[0]
	if !browser.HttpOnly || !browser.Secure || browser.MaxAge != 600 || browser.Path != "/api/auth/yandex" {
		t.Fatal("unsafe OAuth cookie")
	}
	target, _ := url.Parse(start.Header().Get("Location"))
	query := target.Query()
	query.Set("code", "valid")
	req := httptest.NewRequest("GET", "/api/auth/yandex/callback?"+query.Encode(), nil)
	req.AddCookie(browser)
	callback := httptest.NewRecorder()
	router.ServeHTTP(callback, req)
	if callback.Code != 303 || callback.Header().Get("Location") != "/onboarding" {
		t.Fatal("callback failed", callback.Code, callback.Header().Get("Location"))
	}
	var session *http.Cookie
	for _, cookie := range callback.Result().Cookies() {
		if cookie.Name == "rhythm_session" {
			session = cookie
		}
	}
	if session == nil {
		t.Fatal("callback did not issue session")
	}
	data, _ := call(t, router, session, "GET", "/api/auth/me", nil, 200)
	firstID := data["user"].(map[string]any)["id"]
	replay := httptest.NewRecorder()
	router.ServeHTTP(replay, req)
	if replay.Header().Get("Location") != "/auth?error=oauth_invalid" {
		t.Fatal("OAuth callback replay accepted")
	}
	start = httptest.NewRecorder()
	router.ServeHTTP(start, httptest.NewRequest("GET", "/api/auth/yandex/start", nil))
	target, _ = url.Parse(start.Header().Get("Location"))
	query = target.Query()
	query.Set("code", "valid")
	req = httptest.NewRequest("GET", "/api/auth/yandex/callback?"+query.Encode(), nil)
	req.AddCookie(start.Result().Cookies()[0])
	callback = httptest.NewRecorder()
	router.ServeHTTP(callback, req)
	for _, cookie := range callback.Result().Cookies() {
		if cookie.Name == "rhythm_session" {
			session = cookie
		}
	}
	data, _ = call(t, router, session, "GET", "/api/auth/me", nil, 200)
	if data["user"].(map[string]any)["id"] != firstID {
		t.Fatal("existing identity duplicated")
	}
}
func TestPostgresAuthRateLimitAndBodyLimit(t *testing.T) {
	_, _, router, _ := pgFixture(t)
	req := httptest.NewRequest("POST", "/api/auth/register", bytes.NewBufferString(`{"name":"Test","email":"test@example.test","password":"`+strings.Repeat("x", 9000)+`","password_confirmation":"abc"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 422 {
		t.Fatal("body limit not enforced", w.Code)
	}
	if strings.Contains(w.Body.String(), strings.Repeat("x", 20)) {
		t.Fatal("body echoed into error")
	}
	for i := 0; i < 19; i++ {
		w = httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/api/auth/yandex/start", nil))
		if w.Code != 303 {
			t.Fatal("unexpected pre-limit response", w.Code)
		}
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/auth/yandex/start", nil))
	if w.Code != 429 || w.Header().Get("Retry-After") != "60" {
		t.Fatal("rate limit absent", w.Code)
	}
}
func TestPostgresConcurrentAssistantInstancesKeepIdempotency(t *testing.T) {
	db, svc, _, now := pgFixture(t)
	ctx := context.Background()
	u, err := svc.Register(ctx, "Concurrent", "concurrent@example.test", "correct password 123", "correct password 123")
	if err != nil {
		t.Fatal(err)
	}
	ctx = repository.WithUser(ctx, u.ID)
	engine, err := search.New()
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Europe/Moscow")
	cal := &calendar.Service{Store: db, Search: engine, Location: loc}
	instances := []*assistant.Service{assistant.New(db, nlp.Hybrid{}, cal), assistant.New(db, nlp.Hybrid{}, cal)}
	sid, rid := uuid.NewString(), uuid.NewString()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, err := instances[index%2].Respond(ctx, "Поставь задачу проверить конкурентность", sid, rid, now)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	err = db.Read(ctx, func(tx repository.Tx) error {
		n, err := tx.Count(ctx)
		if err != nil || n != 1 {
			t.Fatal("duplicate event", n, err)
		}
		history, err := tx.History(ctx, sid, 10)
		if err != nil || len(history) != 1 {
			t.Fatal("duplicate dialogue exchange", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
