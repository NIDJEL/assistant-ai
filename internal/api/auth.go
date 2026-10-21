package api

import (
	"github.com/NIDJEL/assistant-ai/internal/auth"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/legal"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/user"
	"github.com/go-chi/chi/v5"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const sessionCookie = "rhythm_session"

func (a *API) secureCookie() bool {
	return a.Config.Environment == "production" || strings.HasPrefix(a.Config.PublicURL, "https://")
}
func (a *API) cookie(w http.ResponseWriter, name, value, path string, seconds int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: path, MaxAge: seconds, HttpOnly: true, Secure: a.secureCookie(), SameSite: http.SameSiteLaxMode})
}
func (a *API) currentUser(r *http.Request) (*user.User, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil, nil
	}
	return a.Auth.Session(r.Context(), c.Value, a.Clock.Now())
}
func (a *API) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := a.currentUser(r)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		if u == nil {
			a.fail(w, r, &domain.AppError{Code: "unauthorized", Message: "Сессия истекла. Войдите снова.", Status: 401})
			return
		}
		next.ServeHTTP(w, r.WithContext(repository.WithUser(r.Context(), u.ID)))
	})
}
func (a *API) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
			origin := r.Header.Get("Origin")
			bad := origin != "" && origin != strings.TrimRight(a.Config.PublicURL, "/")
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				bad = true
			}
			// Only the two upload endpoints accept multipart, with a matching Origin.
			mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			upload := r.Method == http.MethodPost && (r.URL.Path == "/api/voice/transcribe" || r.URL.Path == "/api/import-csv") && mediaType == "multipart/form-data" && origin == strings.TrimRight(a.Config.PublicURL, "/")
			if r.Method != http.MethodDelete && mediaType != "application/json" && !upload {
				bad = true
			}
			if bad {
				a.fail(w, r, &domain.AppError{Code: "csrf_rejected", Message: "Обновите страницу и повторите действие", Status: 403})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

type rateWindow struct {
	count int
	until time.Time
}

// Only a configured proxy may supply X-Real-IP. The proxy must overwrite this
// header with the socket peer address, rather than forward a client-supplied value.
func (a *API) authClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	peer := net.ParseIP(host)
	for _, network := range a.Config.TrustedProxyCIDRs {
		if network.Contains(peer) {
			if client := net.ParseIP(r.Header.Get("X-Real-IP")); client != nil {
				return client.String()
			}
			break
		}
	}
	return host
}

func (a *API) authLimiter() func(http.Handler) http.Handler {
	var mu sync.Mutex
	windows := map[string]rateWindow{}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/me") || strings.HasSuffix(r.URL.Path, "/onboarding") || strings.HasSuffix(r.URL.Path, "/logout") {
				next.ServeHTTP(w, r)
				return
			}
			host := a.authClientIP(r)
			now := time.Now()
			mu.Lock()
			for key, value := range windows {
				if now.After(value.until) {
					delete(windows, key)
				}
			}
			slot, exists := windows[host]
			if slot.until.IsZero() {
				slot.until = now.Add(time.Minute)
			}
			slot.count++
			limit := a.Config.AuthRateLimit
			if limit == 0 {
				limit = 20
			}
			full := !exists && len(windows) >= 10000
			limited := slot.count > limit || full
			if !full {
				windows[host] = slot
			}
			mu.Unlock()
			if limited {
				w.Header().Set("Retry-After", "60")
				a.fail(w, r, &domain.AppError{Code: "rate_limited", Message: "Слишком много попыток. Попробуйте через минуту.", Status: 429})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
func (a *API) authRoutes(r chi.Router) {
	r.Route("/api/auth", func(r chi.Router) {
		r.Use(a.authLimiter())
		r.Use(a.csrf)
		r.Post("/register", a.register)
		r.Post("/login", a.login)
		r.Post("/logout", a.logout)
		r.Get("/me", a.me)
		r.With(a.requireUser).Patch("/me", a.updateProfile)
		r.With(a.requireUser).Post("/onboarding", a.completeOnboarding)
		r.With(a.requireUser).Post("/email/enable", a.enableEmailLogin)
		r.Get("/yandex/start", a.oauthStart)
		r.Post("/yandex/start", a.oauthStart)
		r.Get("/yandex/callback", a.oauthCallback)
		r.Get("/{provider:google|zoom}/start", a.cloudOAuthStart)
		r.Get("/{provider:google|zoom}/callback", a.cloudOAuthCallback)
	})
	// Preserve an already registered Yandex callback containing a double slash.
	if callback, err := url.Parse(a.Config.YandexRedirectURI); err == nil && callback.Path == "//api/auth/yandex/callback" {
		r.Group(func(r chi.Router) {
			r.Use(a.authLimiter())
			r.Get("//api/auth/yandex/callback", a.oauthCallback)
		})
	}
}
func (a *API) signIn(w http.ResponseWriter, r *http.Request, u *user.User, status int) {
	token, err := a.Auth.NewSession(r.Context(), u.ID, a.Clock.Now())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if old, err := r.Cookie(sessionCookie); err == nil {
		if err = a.Auth.Logout(r.Context(), old.Value); err != nil {
			a.fail(w, r, err)
			return
		}
	}
	a.cookie(w, sessionCookie, token, "/", int(auth.SessionTTL.Seconds()))
	w.Header().Set("Cache-Control", "no-store")
	write(w, status, map[string]any{"user": u})
}
func (a *API) register(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	var body struct {
		Name         string `json:"name"`
		Email        string `json:"email"`
		Password     string `json:"password"`
		Confirmation string `json:"password_confirmation"`
		legal.Input
	}
	if err := decode(w, r, &body); err != nil {
		a.fail(w, r, domain.Validation("Проверьте поля регистрации"))
		return
	}
	accepted, err := body.Confirm(a.Clock.Now(), "email_register")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	u, err := a.Auth.Register(r.Context(), body.Name, body.Email, body.Password, body.Confirmation, accepted)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.signIn(w, r, u, 201)
}
func (a *API) login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		legal.Input
	}
	if err := decode(w, r, &body); err != nil {
		a.fail(w, r, domain.Validation("Проверьте почту и пароль"))
		return
	}
	accepted, err := body.Confirm(a.Clock.Now(), "email_login")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	u, err := a.Auth.Login(r.Context(), body.Email, body.Password)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if err = a.Auth.Store.RecordLegalAcceptance(r.Context(), u.ID, accepted); err != nil {
		a.fail(w, r, err)
		return
	}
	a.signIn(w, r, u, 200)
}
func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err = a.Auth.Logout(r.Context(), c.Value); err != nil {
			a.fail(w, r, err)
			return
		}
	}
	a.cookie(w, sessionCookie, "", "/", -1)
	w.WriteHeader(204)
}
func (a *API) me(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	u, err := a.currentUser(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if u == nil {
		a.fail(w, r, &domain.AppError{Code: "unauthorized", Message: "Войдите, чтобы продолжить работу", Status: 401})
		return
	}
	write(w, 200, map[string]any{"user": u})
}
func (a *API) updateProfile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name   *string `json:"name"`
		Avatar *string `json:"avatar_url"`
	}
	if err := decode(w, r, &body); err != nil {
		a.fail(w, r, err)
		return
	}
	u, err := a.Auth.UpdateProfile(r.Context(), repository.UserID(r.Context()), body.Name, body.Avatar)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, map[string]any{"user": u})
}
func (a *API) completeOnboarding(w http.ResponseWriter, r *http.Request) {
	u, err := a.Auth.Update(r.Context(), repository.UserID(r.Context()), "", true)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, map[string]any{"user": u})
}

func (a *API) enableEmailLogin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	var body struct {
		Password     string `json:"password"`
		Confirmation string `json:"password_confirmation"`
	}
	if err := decode(w, r, &body); err != nil {
		a.fail(w, r, domain.Validation("Проверьте пароль и его подтверждение"))
		return
	}
	u, err := a.Auth.EnableEmailLogin(r.Context(), repository.UserID(r.Context()), body.Password, body.Confirmation)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.signIn(w, r, u, 200)
}
func (a *API) oauthStart(w http.ResponseWriter, r *http.Request) {
	provider := "yandex"
	link := ""
	calendarFlow := provider == "yandex" && r.URL.Query().Get("calendar") == "1"
	source := "integrations"
	if r.URL.Query().Get("source") == "onboarding" {
		source = "onboarding"
	}
	if r.URL.Query().Get("link") == "1" || calendarFlow {
		u, err := a.currentUser(r)
		if err != nil || u == nil {
			http.Redirect(w, r, "/auth?error=session", 303)
			return
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Redirect(w, r, "/settings?error=oauth_invalid", 303)
			return
		}
		link = u.ID
	}
	var redirect, browser string
	var err error
	var accepted []legal.Acceptance
	if link == "" {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/auth?error=legal_acceptance_required", 303)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2048)
		var body legal.Input
		if err = decode(w, r, &body); err != nil {
			a.fail(w, r, domain.Validation("Подтвердите ознакомление с документами"))
			return
		}
		confirmation, confirmErr := body.Confirm(a.Clock.Now(), "yandex_login")
		if confirmErr != nil {
			a.fail(w, r, confirmErr)
			return
		}
		accepted = append(accepted, confirmation)
	}
	if calendarFlow {
		redirect, browser, err = a.Auth.StartCalendarOAuthFrom(r.Context(), link, source, a.Clock.Now())
	} else {
		redirect, browser, err = a.Auth.StartOAuth(r.Context(), provider, link, a.Clock.Now(), accepted...)
	}
	if err != nil {
		if r.Method == http.MethodPost {
			a.fail(w, r, err)
			return
		}
		destination := "/auth"
		if link != "" {
			destination = "/settings"
		}
		if calendarFlow {
			destination = "/integrations?service=yandex&error=calendar_unavailable"
			if source == "onboarding" {
				destination = "/onboarding?error=calendar_unavailable"
			}
		} else {
			destination += "?error=oauth_unavailable"
		}
		http.Redirect(w, r, destination, 303)
		return
	}
	a.cookie(w, "rhythm_oauth_"+provider, browser, a.oauthCookiePath(provider), 600)
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodPost {
		write(w, 200, map[string]string{"redirect_url": redirect})
		return
	}
	http.Redirect(w, r, redirect, 303)
}
func (a *API) oauthCallback(w http.ResponseWriter, r *http.Request) {
	provider := "yandex"
	browser := ""
	if c, err := r.Cookie("rhythm_oauth_" + provider); err == nil {
		browser = c.Value
	}
	a.cookie(w, "rhythm_oauth_"+provider, "", a.oauthCookiePath(provider), -1)
	current := ""
	u, err := a.currentUser(r)
	if err != nil {
		http.Redirect(w, r, "/auth?error=oauth_invalid", 303)
		return
	}
	if u != nil {
		current = u.ID
	}
	result, err := a.Auth.FinishOAuthFlow(r.Context(), provider, browser, current, r.URL.Query(), a.Clock.Now())
	if err != nil {
		code := "oauth_invalid"
		if app, ok := err.(*domain.AppError); ok {
			code = app.Code
		}
		destination := "/auth"
		if current != "" {
			destination = "/settings"
			if u != nil && !u.OnboardingCompleted {
				destination = "/onboarding"
			}
		}
		if result.Calendar {
			if result.Source == "onboarding" {
				http.Redirect(w, r, "/onboarding?error="+url.QueryEscape(code), 303)
				return
			}
			http.Redirect(w, r, "/integrations?service=yandex&error="+code, 303)
			return
		}
		http.Redirect(w, r, destination+"?error="+code, 303)
		return
	}
	if result.Calendar {
		if result.Source == "onboarding" {
			if _, err := a.Auth.Update(r.Context(), result.User.ID, "", true); err != nil {
				http.Redirect(w, r, "/onboarding?error=calendar_completion", 303)
				return
			}
			http.Redirect(w, r, "/?calendar=connected", 303)
			return
		}
		http.Redirect(w, r, "/integrations?service=yandex&connected=1", 303)
		return
	}
	u = result.User
	if result.Linked {
		http.Redirect(w, r, "/settings?linked="+provider, 303)
		return
	}
	token, err := a.Auth.NewSession(r.Context(), u.ID, a.Clock.Now())
	if err != nil {
		http.Redirect(w, r, "/auth?error=oauth_invalid", 303)
		return
	}
	if old, err := r.Cookie(sessionCookie); err == nil {
		if err = a.Auth.Logout(r.Context(), old.Value); err != nil {
			http.Redirect(w, r, "/auth?error=oauth_invalid", 303)
			return
		}
	}
	a.cookie(w, sessionCookie, token, "/", int(auth.SessionTTL.Seconds()))
	destination := "/"
	if !u.OnboardingCompleted {
		destination = "/onboarding"
	}
	http.Redirect(w, r, destination, 303)
}

func (a *API) oauthCookiePath(provider string) string {
	if provider == "yandex" {
		if callback, err := url.Parse(a.Config.YandexRedirectURI); err == nil && callback.Path == "//api/auth/yandex/callback" {
			return "//api/auth/yandex"
		}
	}
	return "/api/auth/" + provider
}
