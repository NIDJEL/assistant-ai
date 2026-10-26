package api

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/auth"
	"github.com/NIDJEL/assistant-ai/internal/clock"
	"github.com/NIDJEL/assistant-ai/internal/config"
)

func TestRetiredIntegrationEndpointsAreNotExposed(t *testing.T) {
	app := API{
		Auth:   &auth.Service{},
		Config: config.Config{Root: t.TempDir()},
		Clock:  clock.Func(func() time.Time { return time.Unix(0, 0) }),
		Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
	}
	router := app.Router()
	for _, endpoint := range []struct{ method, path string }{
		{"GET", "/api/auth/google/start"}, {"GET", "/api/auth/google/callback"},
		{"GET", "/api/auth/zoom/start"}, {"GET", "/api/auth/zoom/callback"},
		{"GET", "/api/integrations/google/calendars"}, {"PUT", "/api/integrations/google/calendar"},
		{"POST", "/api/integrations/google/sync"}, {"POST", "/api/integrations/zoom/sync"},
		{"DELETE", "/api/integrations/google"}, {"DELETE", "/api/integrations/zoom"},
		{"POST", "/api/events/1/zoom"}, {"POST", "/api/webhooks/zoom"},
	} {
		t.Run(endpoint.method+endpoint.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(endpoint.method, endpoint.path, nil))
			if response.Code != 404 {
				t.Fatalf("retired endpoint returned %d: %s", response.Code, response.Body.String())
			}
		})
	}
}
