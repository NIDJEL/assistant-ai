package contract

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/NIDJEL/assistant-ai/internal/api"
	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/auth"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/clock"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/legal"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/search"
	"github.com/NIDJEL/assistant-ai/internal/storage/postgres"

	"github.com/google/uuid"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"

	"strings"
	"sync"
	"testing"
	"time"
)

type oauthFake struct{ subject, email string }

func (p *oauthFake) AuthorizationURL(state, challenge string) string {
	return "https://provider.test/authorize?state=" + state + "&code_challenge=" + challenge
}
func (p *oauthFake) Identity(ctx context.Context, code, verifier, state string) (repository.Identity, error) {
	if code != "valid" || len(verifier) != 43 {
		return repository.Identity{}, fmt.Errorf("bad code")
	}
	return repository.Identity{Provider: "yandex", Subject: p.subject, Email: p.email, Name: "Яндекс пользователь"}, nil
}
func pgFixture(t *testing.T) (*postgres.Database, *auth.Service, http.Handler, time.Time) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required for PostgreSQL integration tests")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "p12_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := postgres.Open(context.Background(), u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	svc, err := auth.New(db, "test-only-session-secret-at-least-32-characters", map[string]auth.Provider{"yandex": &oauthFake{subject: "12345", email: "alice@example.test"}})
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Europe/Moscow")
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, loc)
	engine, err := search.New()
	if err != nil {
		t.Fatal(err)
	}
	cal := &calendar.Service{Store: db, Search: engine, Location: loc, DemoPath: filepath.Join(root(), "data/voice_assistant_events_1000.csv")}
	app := &api.API{Config: config.Config{Root: root(), Timezone: "Europe/Moscow", NLPProvider: "rules", PublicURL: "https://rhythm.test", Environment: "production"}, Auth: svc, Clock: clock.Func(func() time.Time { return now }), Calendar: cal, Assistant: assistant.New(db, nlp.Hybrid{}, cal), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	return db, svc, app.Router(), now
}
func call(t *testing.T, router http.Handler, cookie *http.Cookie, method, path string, body any, status int) (map[string]any, *httptest.ResponseRecorder) {
	t.Helper()
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != status {
		t.Fatalf("%s %s status %d expected %d: %s", method, path, w.Code, status, w.Body.String())
	}
	data := map[string]any{}
	if status != 204 {
		if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
	}
	return data, w
}
func registerHTTP(t *testing.T, r http.Handler, email string) (string, *http.Cookie) {
	data, w := call(t, r, nil, "POST", "/api/auth/register", withLegal(map[string]string{"name": "Пользователь", "email": email, "password": "correct password 123", "password_confirmation": "correct password 123"}), 201)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Path != "/" {
		t.Fatal("insecure session cookie", cookies)
	}
	return data["user"].(map[string]any)["id"].(string), cookies[0]
}
func TestPostgresAuthenticationAndIsolation(t *testing.T) {
	db, _, router, now := pgFixture(t)
	ctx := context.Background()
	aliceID, alice := registerHTTP(t, router, " Alice@EXAMPLE.test ")
	bobID, bob := registerHTTP(t, router, "bob@example.test")
	for _, path := range []string{"/api/events", "/api/tasks", "/api/meetings", "/api/reminders", "/api/calendar", "/api/archive", "/api/integrations", "/api/preferences/" + aliceID} {
		call(t, router, nil, "GET", path, nil, 401)
	}
	call(t, router, nil, "POST", "/api/chat", map[string]string{"message": "Помощь"}, 401)
	call(t, router, nil, "POST", "/api/auth/login", withLegal(map[string]string{"email": "alice@example.test", "password": "wrong"}), 401)
	data, w := call(t, router, nil, "POST", "/api/auth/login", withLegal(map[string]string{"email": "ALICE@example.test", "password": "correct password 123"}), 200)
	if data["user"].(map[string]any)["id"] != aliceID {
		t.Fatal("normalized email failed")
	}
	alice = w.Result().Cookies()[0]
	call(t, router, alice, "GET", "/api/auth/me", nil, 200)
	call(t, router, nil, "POST", "/api/auth/register", withLegal(map[string]string{"name": "Duplicate", "email": "alice@example.test", "password": "correct password 123", "password_confirmation": "correct password 123"}), 409)
	task, _ := call(t, router, alice, "POST", "/api/events", map[string]string{"kind": "task", "title": "Секретный договор Алисы"}, 201)
	meeting, _ := call(t, router, alice, "POST", "/api/events", map[string]string{"kind": "meeting", "title": "Личная встреча", "date": "2026-10-13", "time": "15:00"}, 201)
	for _, record := range []map[string]any{task, meeting} {
		path := fmt.Sprintf("/api/events/%.0f", record["id"])
		call(t, router, bob, "GET", path, nil, 404)
		call(t, router, bob, "PATCH", path, map[string]string{"title": "stolen"}, 404)
		call(t, router, bob, "DELETE", path, nil, 404)
	}
	call(t, router, bob, "POST", "/api/events", map[string]string{"kind": "task", "title": "Секретный договор Боба"}, 201)
	for _, cookie := range []*http.Cookie{alice, bob} {
		result, _ := call(t, router, cookie, "GET", "/api/tasks?q=договор", nil, 200)
		if result["total"] != float64(1) {
			t.Fatal("search tenant leak", result)
		}
	}
	sid, rid := uuid.NewString(), uuid.NewString()
	input := map[string]string{"message": "Поставь задачу проверить историю", "session_id": sid, "request_id": rid}
	call(t, router, alice, "POST", "/api/chat", input, 200)
	call(t, router, alice, "POST", "/api/chat", input, 200)
	history, _ := call(t, router, bob, "GET", "/api/history/"+sid, nil, 200)
	if len(history["exchanges"].([]any)) != 0 {
		t.Fatal("history leak")
	}
	input["message"] = "Поставь задачу проверить Боба"
	call(t, router, bob, "POST", "/api/chat", input, 200)
	prefs := map[string]any{"name": "Алиса", "revision": 1, "layouts": map[string]any{"today": map[string]any{"order": []string{"tasks"}}}}
	call(t, router, alice, "PUT", "/api/preferences/"+aliceID, prefs, 200)
	call(t, router, bob, "GET", "/api/preferences/"+aliceID, nil, 404)
	preferences, _ := call(t, router, bob, "GET", "/api/preferences/"+bobID, nil, 200)
	if len(preferences) != 0 {
		t.Fatal("preferences leak")
	}
	var count int
	if err := db.DB.QueryRow("SELECT count(*) FROM widget_layouts WHERE user_id=$1", bobID).Scan(&count); err != nil || count != 0 {
		t.Fatal("widget leak", err)
	}
	call(t, router, alice, "POST", "/api/auth/onboarding", map[string]any{}, 200)
	data, _ = call(t, router, alice, "GET", "/api/auth/me", nil, 200)
	if data["user"].(map[string]any)["onboarding_completed"] != true {
		t.Fatal("onboarding not persisted")
	}
	r := httptest.NewRequest("POST", "/api/events", strings.NewReader(`{"kind":"task","title":"csrf"}`))
	r.AddCookie(alice)
	r.Header.Set("Origin", "https://evil.test")
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, r)
	if rec.Code != 403 {
		t.Fatal("CSRF allowed")
	}
	call(t, router, alice, "POST", "/api/auth/logout", map[string]any{}, 204)
	call(t, router, alice, "GET", "/api/auth/me", nil, 401)
	if _, err := db.DB.ExecContext(ctx, "UPDATE sessions SET expires_at=$1 WHERE user_id=$2", now.Add(-time.Second), bobID); err != nil {
		t.Fatal(err)
	}
	call(t, router, bob, "GET", "/api/tasks", nil, 401)
}
func TestPostgresOAuthStatesAndLinking(t *testing.T) {
	db, svc, _, now := pgFixture(t)
	ctx := context.Background()
	email, err := svc.Register(ctx, "Алиса", "alice@example.test", "correct password 123", "correct password 123")
	if err != nil {
		t.Fatal(err)
	}
	start := func(link string) (url.Values, string) {
		accepted, err := (legal.Input{TermsAccepted: true, PersonalDataConsent: true, LegalVersion: legal.Version}).Confirm(now, "yandex_login")
		if err != nil {
			t.Fatal(err)
		}
		redirect, browser, err := svc.StartOAuth(ctx, "yandex", link, now, accepted)
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(redirect)
		if len(u.Query().Get("code_challenge")) != 43 {
			t.Fatal("PKCE missing")
		}
		q := u.Query()
		q.Set("code", "valid")
		return q, browser
	}
	q, browser := start("")
	if _, _, err = svc.FinishOAuth(ctx, "yandex", "wrong-browser", "", q, now); err == nil {
		t.Fatal("browser mismatch accepted")
	}
	q.Set("state", strings.Repeat("x", 43))
	if _, _, err = svc.FinishOAuth(ctx, "yandex", browser, "", q, now); err == nil {
		t.Fatal("invalid state accepted")
	}
	q, browser = start("")
	if _, _, err = svc.FinishOAuth(ctx, "yandex", browser, "", q, now); err == nil {
		t.Fatal("duplicate account created from matching email")
	}
	svc.Providers["yandex"] = &oauthFake{subject: "12345", email: "oauth@example.test"}
	q, browser = start("")
	oauth, _, err := svc.FinishOAuth(ctx, "yandex", browser, "", q, now)
	if err != nil {
		t.Fatal(err)
	}
	if oauth.ID == email.ID {
		t.Fatal("automatic email account linking")
	}
	if _, _, err = svc.FinishOAuth(ctx, "yandex", browser, "", q, now); err == nil {
		t.Fatal("replayed state accepted")
	}
	q, browser = start("")
	again, _, err := svc.FinishOAuth(ctx, "yandex", browser, "", q, now)
	if err != nil || again.ID != oauth.ID {
		t.Fatal("identity duplicated", err)
	}
	q, browser = start(email.ID)
	if _, _, err = svc.FinishOAuth(ctx, "yandex", browser, email.ID, q, now); err == nil {
		t.Fatal("identity transferred to another user")
	}
	svc.Providers["yandex"] = &oauthFake{subject: "new-linked-subject"}
	q, browser = start(email.ID)
	linked, isLink, err := svc.FinishOAuth(ctx, "yandex", browser, email.ID, q, now)
	if err != nil || !isLink || linked.ID != email.ID {
		t.Fatal("explicit linking failed", err)
	}
	q, browser = start("")
	if _, _, err = svc.FinishOAuth(ctx, "yandex", browser, "", q, now.Add(11*time.Minute)); err == nil {
		t.Fatal("expired state accepted")
	}
	var n int
	db.DB.QueryRow("SELECT count(*) FROM calendar_connections").Scan(&n)
	if n != 0 {
		t.Fatal("Yandex login connected calendar")
	}
}
func TestPostgresConstraintsRollbackConcurrency(t *testing.T) {
	db, svc, _, now := pgFixture(t)
	ctx := context.Background()
	u, err := svc.Register(ctx, "Тест", "test@example.test", "correct password 123", "correct password 123")
	if err != nil {
		t.Fatal(err)
	}
	ctx = repository.WithUser(ctx, u.ID)
	if err = db.Migrate(ctx); err != nil {
		t.Fatal("migration repeat", err)
	}
	if err = db.Read(context.Background(), func(repository.Tx) error { return nil }); err == nil {
		t.Fatal("unscoped read allowed")
	}
	stop := fmt.Errorf("rollback requested")
	err = db.Write(ctx, func(tx repository.Tx) error {
		_, err := tx.Create(ctx, domain.Event{Kind: domain.EventTask, Title: "Rollback", Status: domain.StatusOpen, Priority: domain.PriorityNormal, Duration: time.Hour}, now)
		if err != nil {
			return err
		}
		return stop
	})
	if err != stop {
		t.Fatal(err)
	}
	db.Read(ctx, func(tx repository.Tx) error {
		n, err := tx.Count(ctx)
		if err != nil || n != 0 {
			t.Fatal("transaction did not rollback", n, err)
		}
		return nil
	})
	if _, err = db.DB.Exec("INSERT INTO sessions(id,user_id,created_at,expires_at,last_seen_at) VALUES('invalid',$1,now(),now(),now())", uuid.NewString()); err == nil {
		t.Fatal("foreign key missing")
	}
	key := "same-import-key"
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- db.Write(ctx, func(tx repository.Tx) error {
				_, err := tx.Create(ctx, domain.Event{Kind: domain.EventTask, Title: "Concurrent", Status: domain.StatusOpen, Priority: domain.PriorityNormal, Duration: time.Hour, SourceKey: &key}, now)
				return err
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	db.Read(ctx, func(tx repository.Tx) error {
		n, err := tx.Count(ctx)
		if err != nil || n != 1 {
			t.Fatal("constraint/concurrency failed", n, err)
		}
		return nil
	})
}

func withLegal(body map[string]string) map[string]any {
	result := map[string]any{"terms_accepted": true, "personal_data_consent": true, "legal_version": legal.Version}
	for k, v := range body {
		result[k] = v
	}
	return result
}
