package api

import (
	"net/http"
	"os"
	"path/filepath"

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
	for _, path := range []string{"/auth", "/onboarding", "/terms", "/privacy", "/", "/calendar", "/tasks", "/meetings", "/friends", "/reminders", "/archive", "/integrations", "/settings"} {
		r.Get(path, serve)
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
