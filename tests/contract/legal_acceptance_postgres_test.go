package contract

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/legal"
)

func TestPostgresLegalAcceptanceRequiredAndAudited(t *testing.T) {
	db, _, router, now := pgFixture(t)
	credentials := map[string]string{"name": "Документы", "email": "legal@example.test", "password": "correct password 123", "password_confirmation": "correct password 123"}
	for _, field := range []string{"terms_accepted", "personal_data_consent", "legal_version"} {
		body := withLegal(credentials)
		delete(body, field)
		call(t, router, nil, "POST", "/api/auth/register", body, 422)
	}
	for _, field := range []string{"terms_accepted", "personal_data_consent"} {
		body := withLegal(credentials)
		body[field] = false
		call(t, router, nil, "POST", "/api/auth/register", body, 422)
	}
	body := withLegal(credentials)
	body["legal_version"] = "2025-01-01"
	call(t, router, nil, "POST", "/api/auth/register", body, 422)
	var n int
	if err := db.DB.QueryRow("SELECT count(*) FROM users").Scan(&n); err != nil || n != 0 {
		t.Fatal("rejected registration created account", n, err)
	}
	data, w := call(t, router, nil, "POST", "/api/auth/register", withLegal(credentials), 201)
	id := data["user"].(map[string]any)["id"].(string)
	if len(w.Result().Cookies()) != 1 {
		t.Fatal("session not created")
	}
	var version, action string
	var accepted time.Time
	var terms, consent bool
	if err := db.DB.QueryRow("SELECT document_version,accepted_at,action,terms_accepted,personal_data_consent FROM user_legal_acceptances WHERE user_id=$1", id).Scan(&version, &accepted, &action, &terms, &consent); err != nil || version != legal.Version || !accepted.Equal(now) || action != "email_register" || !terms || !consent {
		t.Fatal("registration evidence invalid", err)
	}
	login := map[string]string{"email": credentials["email"], "password": credentials["password"]}
	call(t, router, nil, "POST", "/api/auth/login", login, 422)
	wrong := withLegal(login)
	wrong["password"] = "wrong password"
	call(t, router, nil, "POST", "/api/auth/login", wrong, 401)
	call(t, router, nil, "POST", "/api/auth/login", withLegal(login), 200)
	if err := db.DB.QueryRow("SELECT count(*) FROM user_legal_acceptances WHERE user_id=$1", id).Scan(&n); err != nil || n != 2 {
		t.Fatal("failed login recorded consent or valid login omitted it", n, err)
	}
}

func TestPostgresLegalEvidenceFailureRollsBackRegistration(t *testing.T) {
	db, _, router, _ := pgFixture(t)
	// A failed audit write must not leave an account that cannot be retried.
	_, err := db.DB.ExecContext(context.Background(), `CREATE FUNCTION reject_legal_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit unavailable'; END $$;
CREATE TRIGGER reject_legal_test BEFORE INSERT ON user_legal_acceptances FOR EACH ROW EXECUTE FUNCTION reject_legal_test()`)
	if err != nil {
		t.Fatal(err)
	}
	call(t, router, nil, "POST", "/api/auth/register", withLegal(map[string]string{"name": "Retry", "email": "retry@example.test", "password": "correct password 123", "password_confirmation": "correct password 123"}), 500)
	var n int
	if err := db.DB.QueryRow("SELECT count(*) FROM users").Scan(&n); err != nil || n != 0 {
		t.Fatal("registration was not atomic", n, err)
	}
}

func TestPostgresOAuthConsentCannotBypassBrowserState(t *testing.T) {
	db, _, router, _ := pgFixture(t)
	get := httptest.NewRecorder()
	router.ServeHTTP(get, httptest.NewRequest("GET", "/api/auth/yandex/start", nil))
	if get.Code != 303 || get.Header().Get("Location") != "/auth?error=legal_acceptance_required" || len(get.Result().Cookies()) != 0 {
		t.Fatal("bare GET bypassed explicit confirmation")
	}
	call(t, router, nil, "POST", "/api/auth/yandex/start", map[string]any{}, 422)
	var n int
	if err := db.DB.QueryRow("SELECT count(*) FROM oauth_states").Scan(&n); err != nil || n != 0 {
		t.Fatal("invalid confirmation created OAuth state", err)
	}
	raw, _ := json.Marshal(withLegal(nil))
	req := httptest.NewRequest("POST", "/api/auth/yandex/start", strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://attacker.test")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("cross-site confirmation allowed")
	}
	data, start := call(t, router, nil, "POST", "/api/auth/yandex/start", withLegal(nil), 200)
	target, _ := url.Parse(data["redirect_url"].(string))
	query := target.Query()
	query.Set("error", "access_denied")
	callback := httptest.NewRequest("GET", "/api/auth/yandex/callback?"+query.Encode(), nil)
	callback.AddCookie(start.Result().Cookies()[0])
	w = httptest.NewRecorder()
	router.ServeHTTP(w, callback)
	if w.Header().Get("Location") != "/auth?error=oauth_cancelled" {
		t.Fatal("cancel failed")
	}
	if err := db.DB.QueryRow("SELECT count(*) FROM user_legal_acceptances").Scan(&n); err != nil || n != 0 {
		t.Fatal("cancelled OAuth recorded account confirmation", n, err)
	}
	// A pending state created before the upgrade cannot create a user without
	// explicit confirmation after the upgrade either.
	data, start = call(t, router, nil, "POST", "/api/auth/yandex/start", withLegal(nil), 200)
	if _, err := db.DB.Exec("UPDATE oauth_states SET legal_acceptance=NULL"); err != nil {
		t.Fatal(err)
	}
	target, _ = url.Parse(data["redirect_url"].(string))
	query = target.Query()
	query.Set("code", "valid")
	callback = httptest.NewRequest("GET", "/api/auth/yandex/callback?"+query.Encode(), nil)
	callback.AddCookie(start.Result().Cookies()[0])
	w = httptest.NewRecorder()
	router.ServeHTTP(w, callback)
	if w.Header().Get("Location") != "/auth?error=legal_acceptance_required" {
		t.Fatal("legacy state bypassed explicit confirmation")
	}
	if err := db.DB.QueryRow("SELECT count(*) FROM users").Scan(&n); err != nil || n != 0 {
		t.Fatal("legacy state created account", n, err)
	}
}
