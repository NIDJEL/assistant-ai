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
	for _, name := range []string{"yandex", "vk"} {
		t.Run(name, func(t *testing.T) {
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
					if name == "vk" {
						if r.Form.Get("device_id") != "device" || r.Form.Get("state") != "state" {
							t.Error("VK required fields missing")
						}
					}
					json.NewEncoder(w).Encode(map[string]any{"access_token": "provider-secret-token", "state": "state", "user_id": 123})
					return
				}
				infoCalls++
				if name == "yandex" {
					if r.Header.Get("Authorization") != "OAuth provider-secret-token" || r.URL.Query().Get("oauth_token") != "" {
						t.Error("Yandex token must only be in authorization header")
					}
					json.NewEncoder(w).Encode(map[string]any{"id": "123", "default_email": "alice@example.test", "real_name": "Алиса", "default_avatar_id": "avatar"})
				} else {
					r.ParseForm()
					if r.Form.Get("client_id") != "client" || r.Form.Get("access_token") != "provider-secret-token" {
						t.Error("VK user_info form invalid")
					}
					json.NewEncoder(w).Encode(map[string]any{"user": map[string]any{"user_id": 123, "first_name": "Алиса", "last_name": "Ритм", "email": "alice@example.test"}})
				}
			}))
			defer server.Close()
			p := &Provider{Name: name, ClientID: "client", ClientSecret: "secret", RedirectURI: "https://rhythm.test/callback", TokenEndpoint: server.URL + "/token", UserEndpoint: server.URL + "/info", AuthorizeEndpoint: server.URL + "/authorize", Client: server.Client()}
			identity, err := p.Identity(context.Background(), "code", "verifier", "state", "device")
			if err != nil || identity.Subject != "123" || identity.Provider != name {
				t.Fatal(identity, err)
			}
			if tokenCalls != 1 || infoCalls != 1 {
				t.Fatal("unexpected HTTP exchange count")
			}
			method := "S256"
			if name == "vk" {
				method = "s256"
			}
			if !strings.Contains(p.AuthorizationURL("state", "challenge"), "code_challenge_method="+method) {
				t.Fatal("PKCE missing")
			}
		})
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
func TestVKRejectsTokenStateMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "token", "state": "other"})
	}))
	defer server.Close()
	p := &Provider{Name: "vk", TokenEndpoint: server.URL, Client: server.Client()}
	if _, err := p.Identity(context.Background(), "code", "verifier", "state", "device"); err == nil {
		t.Fatal("token state mismatch accepted")
	}
}
