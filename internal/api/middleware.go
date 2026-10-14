package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/go-chi/chi/v5/middleware"
)

func (a *API) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(wrapped, r)
		a.Logger.InfoContext(r.Context(), "http request", "request_id", middleware.GetReqID(r.Context()), "method", r.Method, "path", r.URL.Path, "status", wrapped.Status(), "duration_ms", time.Since(start).Milliseconds())
	})
}
func (a *API) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				a.Logger.ErrorContext(r.Context(), "handler panic", "request_id", middleware.GetReqID(r.Context()))
				a.fail(w, r, &domain.AppError{Code: "internal_error", Message: "Внутренняя ошибка сервера", Status: 500})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
func (a *API) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.URL.Path == "/health" || strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "microphone=(self)")
		policy := "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https://avatars.yandex.net; connect-src 'self'; frame-ancestors 'none'"
		if r.URL.Path == "/docs" {
			policy = "default-src 'self'; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; img-src 'self' data: https://avatars.yandex.net; connect-src 'self'; frame-ancestors 'none'"
		}
		w.Header().Set("Content-Security-Policy", policy)
		next.ServeHTTP(w, r)
	})
}
