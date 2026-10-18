package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NIDJEL/assistant-ai/internal/auth"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/tts"
)

type fakeSynthesis struct {
	calls int
	err   error
}

func (f *fakeSynthesis) Synthesize(context.Context, string, string) ([]byte, error) {
	f.calls++
	return []byte("MP3"), f.err
}
func (f *fakeSynthesis) Voices(context.Context) ([]tts.Voice, error) {
	return []tts.Voice{{ID: tts.DefaultVoice, Name: "Дмитрий"}}, f.err
}
func ttsRequest(body string) *http.Request {
	r := httptest.NewRequest("POST", "/api/voice/synthesize", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://gena.test")
	return r.WithContext(repository.WithUser(r.Context(), "test-user"))
}
func TestTTSAPI(t *testing.T) {
	for _, tc := range []struct {
		body   string
		err    error
		status int
	}{
		{`{"text":"Привет"}`, nil, 200}, {`{"text":"Привет","voice":"invalid"}`, nil, 400}, {`{"text":""}`, nil, 400},
		{`{"text":"Привет","extra":true}`, nil, 422}, {`{"text":"Привет"}`, tts.ErrRateLimited, 429}, {`{"text":"Привет"}`, errors.New("secret provider error"), 503},
	} {
		t.Run(tc.body+http.StatusText(tc.status), func(t *testing.T) {
			f := &fakeSynthesis{err: tc.err}
			a := &API{Auth: &auth.Service{}, Synthesizer: f, Config: config.Config{PublicURL: "https://gena.test"}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
			w := httptest.NewRecorder()
			a.csrf(a.ttsLimiter()(http.HandlerFunc(a.synthesize))).ServeHTTP(w, ttsRequest(tc.body))
			if w.Code != tc.status {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "secret provider") {
				t.Fatal("error leaked")
			}
			if w.Code == 200 && (w.Header().Get("Content-Type") != "audio/mpeg" || w.Header().Get("Cache-Control") != "no-store") {
				t.Fatal("incorrect audio headers")
			}
			if w.Code < 500 && w.Code != 200 && tc.err == nil && f.calls != 0 {
				t.Fatal("invalid request reached Azure")
			}
		})
	}
}
func TestTTSProtection(t *testing.T) {
	f := &fakeSynthesis{}
	a := &API{Auth: &auth.Service{}, Synthesizer: f, Config: config.Config{PublicURL: "https://gena.test"}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	w := httptest.NewRecorder()
	r := ttsRequest(`{"text":"Привет"}`)
	r = r.WithContext(context.Background())
	a.Router().ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
	h := a.csrf(a.ttsLimiter()(http.HandlerFunc(a.synthesize)))
	for i := range 13 {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, ttsRequest(`{"text":"Привет"}`))
		want := 200
		if i == 12 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("request %d=%d", i, w.Code)
		}
	}
	r = ttsRequest(`{"text":"Привет"}`)
	r.Header.Set("Origin", "https://evil.test")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("CSRF bypass")
	}
	budget := a.ttsLimiter()(http.HandlerFunc(a.synthesize))
	for i := range 4 {
		w = httptest.NewRecorder()
		budget.ServeHTTP(w, ttsRequest(`{"text":"`+strings.Repeat("я", 2000)+`"}`))
		want := 200
		if i == 3 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("budget request %d=%d", i, w.Code)
		}
	}
}
