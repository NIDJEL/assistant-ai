package api

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/go-chi/chi/v5"
)

func (a *API) cloudOAuthStart(w http.ResponseWriter, r *http.Request) {
	p := chi.URLParam(r, "provider")
	u, err := a.currentUser(r)
	if err != nil || u == nil {
		http.Redirect(w, r, "/auth?error=session", 303)
		return
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		http.Redirect(w, r, "/integrations?error=oauth_invalid", 303)
		return
	}
	redirect, browser, err := a.Auth.StartIntegrationOAuth(r.Context(), p, u.ID, a.Clock.Now())
	if err != nil {
		http.Redirect(w, r, "/integrations?service="+url.QueryEscape(p)+"&error=integration_unavailable", 303)
		return
	}
	a.cookie(w, "rhythm_oauth_"+p, browser, "/api/auth/"+p, 600)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, redirect, 303)
}
func (a *API) cloudOAuthCallback(w http.ResponseWriter, r *http.Request) {
	p := chi.URLParam(r, "provider")
	if p != "google" && p != "zoom" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	browser := ""
	if c, e := r.Cookie("rhythm_oauth_" + p); e == nil {
		browser = c.Value
	}
	a.cookie(w, "rhythm_oauth_"+p, "", "/api/auth/"+p, -1)
	current := ""
	if u, e := a.currentUser(r); e == nil && u != nil {
		current = u.ID
	}
	result, err := a.Auth.FinishOAuthFlow(r.Context(), p, browser, current, r.URL.Query(), a.Clock.Now())
	suffix := "&connected=1"
	if err != nil || !result.Integration {
		suffix = "&error=oauth_invalid"
		if r.URL.Query().Get("error") != "" {
			suffix = "&error=oauth_cancelled"
		}
	}
	http.Redirect(w, r, "/integrations?service="+p+suffix, 303)
}
func (a *API) cloudSync(w http.ResponseWriter, r *http.Request) {
	if a.Cloud == nil {
		a.fail(w, r, domain.Validation("Интеграции не настроены"))
		return
	}
	p := chi.URLParam(r, "provider")
	if p != "google" && p != "zoom" {
		http.NotFound(w, r)
		return
	}
	if err := a.Cloud.Sync(r.Context(), p); err != nil {
		a.fail(w, r, err)
		return
	}
	a.integrations(w, r)
}
func (a *API) cloudDisconnect(w http.ResponseWriter, r *http.Request) {
	p := chi.URLParam(r, "provider")
	if a.Cloud == nil || (p != "google" && p != "zoom") {
		http.NotFound(w, r)
		return
	}
	if err := a.Cloud.Disconnect(r.Context(), p); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(204)
}
func (a *API) googleCalendars(w http.ResponseWriter, r *http.Request) {
	if a.Cloud == nil {
		http.NotFound(w, r)
		return
	}
	if r.Method == "GET" {
		list, err := a.Cloud.Calendars(r.Context())
		if err != nil {
			a.fail(w, r, err)
			return
		}
		c, err := a.Cloud.Store.CloudConnection(r.Context(), "google")
		if err != nil {
			a.fail(w, r, err)
			return
		}
		selected := ""
		if c != nil {
			selected = c.CalendarID
		}
		write(w, 200, map[string]any{"calendars": list, "selected": selected})
		return
	}
	var body struct {
		CalendarID string `json:"calendar_id"`
	}
	if err := decode(w, r, &body); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.Cloud.SelectCalendar(r.Context(), body.CalendarID); err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, map[string]any{"selected": body.CalendarID})
}
func (a *API) zoomMeeting(w http.ResponseWriter, r *http.Request) {
	if a.Cloud == nil {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(strings.TrimSpace(chi.URLParam(r, "id")), 10, 64)
	if err != nil || id <= 0 {
		a.fail(w, r, domain.Validation("Некорректная встреча"))
		return
	}
	e, err := a.Cloud.CreateZoom(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, map[string]any{"event": e})
}
