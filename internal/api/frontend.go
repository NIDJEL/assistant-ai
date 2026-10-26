package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/go-chi/chi/v5"
)

// Only declared SPA routes fall back to index.html; API errors remain JSON.
func (a *API) frontendRoutes(r chi.Router) {
	root := filepath.Join(a.Config.Root, "web", "dist")
	index := filepath.Join(root, "index.html")
	serve := func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		if _, err := os.Stat(index); err != nil {
			a.fail(w, req, &domain.AppError{Code: "frontend_unavailable", Message: "Сначала соберите frontend: pnpm --dir web build", Status: 404})
			return
		}
		http.ServeFile(w, req, index)
	}
	for _, path := range []string{"/auth", "/onboarding", "/terms", "/privacy", "/privacy-policy", "/personal-data-consent", "/", "/calendar", "/tasks", "/meetings", "/friends", "/reminders", "/archive", "/integrations", "/settings"} {
		r.Get(path, serve)
	}
	serveStatus := func(w http.ResponseWriter, req *http.Request) {
		status, err := strconv.Atoi(chi.URLParam(req, "status"))
		if err != nil || status < 400 || status > 599 {
			status = http.StatusNotFound
		}
		if !a.frontendError(w, req, status) {
			a.fail(w, req, &domain.AppError{Code: "http_error", Message: http.StatusText(status), Status: status})
		}
	}
	for _, pattern := range []string{"/errors/{status}", "/{status:[45][0-9]{2}}"} {
		r.Get(pattern, serveStatus)
		r.Head(pattern, serveStatus)
	}
	r.Handle("/assets/*", http.StripPrefix("/assets/", http.FileServer(http.Dir(filepath.Join(root, "assets")))))
	r.Get("/favicon.svg", func(w http.ResponseWriter, req *http.Request) {
		http.ServeFile(w, req, filepath.Join(root, "favicon.svg"))
	})
	r.Get("/manifest.webmanifest", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/manifest+json")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, req, filepath.Join(root, "manifest.webmanifest"))
	})
	r.Handle("/icons/*", http.StripPrefix("/icons/", http.FileServer(http.Dir(filepath.Join(root, "icons")))))
	r.Get("/notifications-sw.js", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, req, filepath.Join(root, "notifications-sw.js"))
	})
}

// Keep API errors machine-readable, while browser navigation gets a real HTTP
// error with a standalone page that does not depend on React or API availability.
func (a *API) frontendError(w http.ResponseWriter, req *http.Request, status int) bool {
	if (req.Method != http.MethodGet && req.Method != http.MethodHead) ||
		!strings.Contains(req.Header.Get("Accept"), "text/html") ||
		strings.HasPrefix(req.URL.Path, "/api/") || req.URL.Path == "/api" {
		return false
	}
	page, err := os.ReadFile(filepath.Join(a.Config.Root, "web", "dist", "errors", strconv.Itoa(status)+".html"))
	if err != nil {
		return false
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex")
	w.WriteHeader(status)
	if req.Method != http.MethodHead {
		_, _ = w.Write(page)
	}
	return true
}
