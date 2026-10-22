package contract

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/auth"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/user"
)

func yandexEmailAccount(t *testing.T, svc *auth.Service, now time.Time, email string) (*user.User, *http.Cookie) {
	t.Helper()
	u, err := svc.Store.OAuthUser(context.Background(), repository.Identity{Provider: "yandex", Subject: email, Email: email, Name: "Яндекс пользователь"}, "")
	if err != nil {
		t.Fatal(err)
	}
	token, err := svc.NewSession(context.Background(), u.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	return u, &http.Cookie{Name: "rhythm_session", Value: token}
}

func TestPostgresEnableEmailLoginForYandexAccount(t *testing.T) {
	db, svc, router, now := pgFixture(t)
	u, session := yandexEmailAccount(t, svc, now, "yandex-login@example.test")
	body := map[string]string{"password": "my new Gena password", "password_confirmation": "my new Gena password"}
	call(t, router, nil, "POST", "/api/auth/email/enable", body, 401)
	call(t, router, session, "POST", "/api/auth/email/enable", map[string]string{"password": "short", "password_confirmation": "short"}, 422)
	call(t, router, session, "POST", "/api/auth/email/enable", map[string]string{"password": body["password"], "password_confirmation": "a different password"}, 422)
	call(t, router, session, "POST", "/api/auth/email/enable", map[string]string{"password": body["password"], "password_confirmation": body["password"], "email": "victim@example.test"}, 422)
	request := httptest.NewRequest("POST", "/api/auth/email/enable", bytes.NewBufferString(`{"password":"my new Gena password","password_confirmation":"my new Gena password"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://other.test")
	request.AddCookie(session)
	rejected := httptest.NewRecorder()
	router.ServeHTTP(rejected, request)
	if rejected.Code != 403 {
		t.Fatal("cross-origin enrollment accepted", rejected.Code)
	}
	record, _ := call(t, router, session, "POST", "/api/events", map[string]string{"kind": "task", "title": "Мои дела сохраняются"}, 201)
	result, response := call(t, router, session, "POST", "/api/auth/email/enable", body, 200)
	account := result["user"].(map[string]any)
	if account["id"] != u.ID || len(account["providers"].([]any)) != 2 || strings.Contains(response.Body.String(), "password") {
		t.Fatal("account or response changed incorrectly", account)
	}
	renewed := response.Result().Cookies()[0]
	if renewed.Value == session.Value || !renewed.HttpOnly || !renewed.Secure {
		t.Fatal("session not rotated securely")
	}
	call(t, router, session, "GET", "/api/auth/me", nil, 401)
	call(t, router, renewed, "POST", "/api/auth/email/enable", body, 409)
	_, hash, err := db.EmailIdentity(context.Background(), u.Email)
	if err != nil || !strings.HasPrefix(hash, "$argon2id$") || hash == body["password"] {
		t.Fatal("password not stored as Argon2id", err)
	}
	signedIn, _ := call(t, router, nil, "POST", "/api/auth/login", withLegal(map[string]string{"email": strings.ToUpper(u.Email), "password": body["password"]}), 200)
	if signedIn["user"].(map[string]any)["id"] != u.ID {
		t.Fatal("email login created another account")
	}
	kept, _ := call(t, router, renewed, "GET", "/api/events", nil, 200)
	if kept["total"] != float64(1) || kept["events"].([]any)[0].(map[string]any)["id"] != record["id"] {
		t.Fatal("existing records were lost")
	}
	linked, err := svc.Store.OAuthUser(context.Background(), repository.Identity{Provider: "yandex", Subject: u.Email, Email: u.Email}, "")
	if err != nil || linked.ID != u.ID {
		t.Fatal("Yandex login stopped working", err)
	}
}

func TestPostgresEnableEmailLoginDoesNotResetOrTrustAnotherContact(t *testing.T) {
	db, svc, router, now := pgFixture(t)
	_, session := registerHTTP(t, router, "existing-email@example.test")
	body := map[string]string{"password": "a replacement password 7", "password_confirmation": "a replacement password 7"}
	call(t, router, session, "POST", "/api/auth/email/enable", body, 409)
	call(t, router, nil, "POST", "/api/auth/login", withLegal(map[string]string{"email": "existing-email@example.test", "password": "correct password 123"}), 200)
	u, session := yandexEmailAccount(t, svc, now, "original-contact@example.test")
	if _, err := db.DB.Exec("UPDATE users SET email='different-contact@example.test' WHERE id=$1", u.ID); err != nil {
		t.Fatal(err)
	}
	call(t, router, session, "POST", "/api/auth/email/enable", body, 409)
	empty, err := svc.Store.OAuthUser(context.Background(), repository.Identity{Provider: "yandex", Subject: "no-email", Name: "Без почты"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.EnableEmailLogin(context.Background(), empty.ID, body["password"], body["password_confirmation"]); err == nil {
		t.Fatal("enabled login without an email")
	}
}

func TestPostgresEnableEmailLoginConcurrentRequestsCannotReplacePassword(t *testing.T) {
	db, svc, _, now := pgFixture(t)
	u, _ := yandexEmailAccount(t, svc, now, "concurrent-email@example.test")
	passwords := []string{"first private password 7", "second private password 8"}
	outcomes := make([]error, 2)
	var group sync.WaitGroup
	for index := range passwords {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			_, outcomes[index] = svc.EnableEmailLogin(context.Background(), u.ID, passwords[index], passwords[index])
		}(index)
	}
	group.Wait()
	winner := -1
	for index, err := range outcomes {
		if err == nil {
			if winner != -1 {
				t.Fatal("both enrollments succeeded")
			}
			winner = index
		} else {
			var app *domain.AppError
			if !errors.As(err, &app) || app.Status != 409 {
				t.Fatal(err)
			}
		}
	}
	if winner == -1 {
		t.Fatal("neither enrollment succeeded")
	}
	_, hash, err := db.EmailIdentity(context.Background(), u.Email)
	if err != nil || !auth.VerifyPassword(passwords[winner], hash) || auth.VerifyPassword(passwords[1-winner], hash) {
		t.Fatal("initial password overwritten", err)
	}
}
