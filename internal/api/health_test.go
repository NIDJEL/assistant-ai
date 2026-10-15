package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/auth"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/clock"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

type healthStore struct {
	repository.Store
	pinged bool
}

func (s *healthStore) Ping(context.Context) error { s.pinged = true; return nil }

func TestPublicHealthWithAuthReportsEffectiveProvider(t *testing.T) {
	for _, semantic := range []string{"gigachat", "rules", ""} {
		t.Run(semantic, func(t *testing.T) {
			store := &healthStore{}
			app := API{
				Auth: &auth.Service{}, Calendar: &calendar.Service{Store: store},
				Config: config.Config{NLPProvider: "rules", SemanticProvider: semantic},
				Clock:  clock.Func(func() time.Time { return time.Unix(0, 0) }),
			}
			response := httptest.NewRecorder()
			app.health(response, httptest.NewRequest("GET", "/health", nil))
			var body struct {
				Provider string `json:"nlp_provider"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			want := semantic
			if want == "" {
				want = "rules"
			}
			if response.Code != 200 || body.Provider != want || !store.pinged {
				t.Fatalf("status=%d provider=%q database ping=%v; want 200/%q/true", response.Code, body.Provider, store.pinged, want)
			}
		})
	}
}
