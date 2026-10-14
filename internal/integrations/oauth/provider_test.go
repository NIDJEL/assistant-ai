package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestProviderExchanges(t *testing.T) {
	tokenCalls, infoCalls := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/token" {
			tokenCalls++
			r.ParseForm()
			for k, v := range map[string]string{"grant_type": "authorization_code", "client_id": "client", "client_secret": "secret", "redirect_uri": "https://rhythm.test/callback", "code_verifier": "verifier", "code": "code"} {
				if r.Form.Get(k) != v {
					t.Errorf("%s mismatch", k)
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": "provider-secret-token"})
			return
		}
		infoCalls++
		if r.Header.Get("Authorization") != "OAuth provider-secret-token" || r.URL.Query().Get("oauth_token") != "" {
			t.Error("token must only be in authorization header")
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "123", "default_email": "alice@example.test", "real_name": "Алиса", "default_avatar_id": "avatar"})
	}))
	defer server.Close()
	p := &Provider{Name: "yandex", ClientID: "client", ClientSecret: "secret", RedirectURI: "https://rhythm.test/callback", TokenEndpoint: server.URL + "/token", UserEndpoint: server.URL + "/info", AuthorizeEndpoint: server.URL + "/authorize", Client: server.Client()}
	identity, err := p.Identity(context.Background(), "code", "verifier", "state")
	if err != nil || identity.Subject != "123" || identity.Provider != "yandex" {
		t.Fatal(identity, err)
	}
	if tokenCalls != 1 || infoCalls != 1 {
		t.Fatal("unexpected exchange count")
	}
	if !strings.Contains(p.AuthorizationURL("state", "challenge"), "code_challenge_method=S256") {
		t.Fatal("PKCE missing")
	}
}

func TestYandexCalendarScopesAndRejectedRefresh(t *testing.T) {
	p := &Provider{Name: "yandex", AuthorizeEndpoint: "https://oauth.yandex.ru/authorize"}
	login, _ := url.Parse(p.AuthorizationURL("state", "challenge"))
	calendar, _ := url.Parse(p.CalendarAuthorizationURL("state", "challenge"))
	if strings.Contains(login.Query().Get("scope"), "calendar:all") || !strings.Contains(calendar.Query().Get("scope"), "calendar:all") {
		t.Fatal("login and calendar permissions mixed")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"refresh-secret"}`))
	}))
	defer server.Close()
	p.TokenEndpoint = server.URL
	_, err := p.Refresh(context.Background(), "refresh-secret")
	if !errors.Is(err, ErrTokenRejected) || strings.Contains(err.Error(), "refresh-secret") {
		t.Fatal("refresh rejection not sanitized")
	}
}
