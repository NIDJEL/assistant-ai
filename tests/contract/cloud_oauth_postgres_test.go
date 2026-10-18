package contract

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/NIDJEL/assistant-ai/internal/auth"
	"github.com/NIDJEL/assistant-ai/internal/integrations/cloud"
	"github.com/NIDJEL/assistant-ai/internal/integrations/oauth"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

func TestPostgresCloudOAuthBoundStateAndNoLoginIdentity(t *testing.T) {
	db, a, _, now := pgFixture(t)
	user, registerErr := a.Register(context.Background(), "Cloud OAuth", "cloud-oauth@example.test", "correct password 123", "correct password 123")
	if registerErr != nil {
		t.Fatal(registerErr)
	}
	uid := user.ID
	tokenCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/token" {
			tokenCalls++
			r.ParseForm()
			if r.Form.Get("code_verifier") == "" {
				t.Error("Google PKCE missing")
			}
			fmt.Fprint(w, `{"access_token":"test-cloud-private-access","refresh_token":"test-cloud-private-refresh","expires_in":3600,"scope":"https://www.googleapis.com/auth/calendar.events https://www.googleapis.com/auth/calendar.calendarlist.readonly"}`)
		} else {
			fmt.Fprint(w, `{"sub":"google-subject","email":"google@example.test"}`)
		}
	}))
	defer server.Close()
	p := &oauth.Provider{Name: "google", ClientID: "public-test-client", ClientSecret: "private-test-client-secret", AuthorizeEndpoint: server.URL + "/authorize", TokenEndpoint: server.URL + "/token", UserEndpoint: server.URL + "/userinfo"}
	svc, err := cloud.New(db, map[string]*oauth.Provider{"google": p}, strings.Repeat("private-test-key", 3), now.Location())
	if err != nil {
		t.Fatal(err)
	}
	a.Providers = map[string]auth.Provider{"google": p}
	a.Integrations = svc
	start, browser, err := a.StartIntegrationOAuth(context.Background(), "google", uid, now)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(start)
	q := url.Values{"state": {u.Query().Get("state")}, "code": {"test-code"}}
	if _, err = a.FinishOAuthFlow(context.Background(), "google", browser, "another-user", q, now); err == nil {
		t.Fatal("callback accepted another user")
	}
	if tokenCalls != 0 {
		t.Fatal("identity mismatch called provider")
	}
	start, browser, err = a.StartIntegrationOAuth(context.Background(), "google", uid, now)
	if err != nil {
		t.Fatal(err)
	}
	u, _ = url.Parse(start)
	q.Set("state", u.Query().Get("state"))
	result, err := a.FinishOAuthFlow(context.Background(), "google", browser, uid, q, now)
	if err != nil || !result.Integration {
		t.Fatal(err)
	}
	if _, err = a.FinishOAuthFlow(context.Background(), "google", browser, uid, q, now); err == nil {
		t.Fatal("OAuth replay accepted")
	}
	c, err := db.CloudConnection(repository.WithUser(context.Background(), uid), "google")
	if err != nil || c == nil || c.Subject != "google-subject" {
		t.Fatal("connection missing", err)
	}
	var count int
	if err = db.DB.QueryRow("SELECT count(*) FROM auth_identities WHERE user_id=$1 AND provider<>'email'", uid).Scan(&count); err != nil || count != 0 {
		t.Fatal("integration became login identity", err)
	}
}
