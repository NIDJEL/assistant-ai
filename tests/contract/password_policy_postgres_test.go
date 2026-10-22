package contract

import (
	"strings"
	"testing"

	"github.com/NIDJEL/assistant-ai/internal/auth"
)

func TestPostgresPasswordPolicyAndLegacyLogin(t *testing.T) {
	_, svc, router, now := pgFixture(t)
	_, session := yandexEmailAccount(t, svc, now, "strong-yandex@example.test")
	for _, password := range []string{strings.Repeat(" ", 10), strings.Repeat("\u00a0", 15), "abcdefgh9" + strings.Repeat(" ", 40), "AAAAAAAAAAAA", "abcabcabcabc", "password12345", "1234567890", "Onlyletters", "Valid!pass7\u200b"} {
		body := map[string]string{"password": password, "password_confirmation": password}
		call(t, router, session, "POST", "/api/auth/email/enable", body, 422)
		call(t, router, nil, "POST", "/api/auth/register", withLegal(map[string]string{"name": "Анна", "email": "password-policy@example.test", "password": password, "password_confirmation": password}), 422)
	}
	// Use a fresh fixture for successful requests so the weak-password matrix
	// does not exhaust the deliberately small authentication rate limit.
	db, svc, router, now := pgFixture(t)
	_, session = yandexEmailAccount(t, svc, now, "strong-yandex@example.test")
	password := "  River!coast7  "
	body := map[string]string{"password": password, "password_confirmation": password}
	call(t, router, session, "POST", "/api/auth/email/enable", body, 200)
	call(t, router, nil, "POST", "/api/auth/login", withLegal(map[string]string{"email": "strong-yandex@example.test", "password": password}), 200)
	call(t, router, nil, "POST", "/api/auth/login", withLegal(map[string]string{"email": "strong-yandex@example.test", "password": strings.TrimSpace(password)}), 401)
	_, _ = registerHTTP(t, router, "legacy-password@example.test")
	legacy := strings.Repeat(" ", 10)
	hash, err := auth.HashPassword(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB.Exec("UPDATE auth_identities SET password_hash=$1 WHERE provider='email' AND email=$2", hash, "legacy-password@example.test"); err != nil {
		t.Fatal(err)
	}
	call(t, router, nil, "POST", "/api/auth/login", withLegal(map[string]string{"email": "legacy-password@example.test", "password": legacy}), 200)
}
