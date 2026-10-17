package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/NIDJEL/assistant-ai/internal/auth"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/stt"
)

type transcribeFunc func(context.Context, string, string, []byte) (string, error)

func (f transcribeFunc) Transcribe(c context.Context, n, m string, a []byte) (string, error) {
	return f(c, n, m, a)
}

func audioRequest(t *testing.T, mime string, audio []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="voice.webm"`)
	h.Set("Content-Type", mime)
	part, err := writer.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	part.Write(audio)
	writer.Close()
	r := httptest.NewRequest("POST", "/api/voice/transcribe", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	r.Header.Set("Origin", "https://gena.test")
	return r
}
func TestVoiceEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name, mime   string
		size, status int
		providerErr  bool
	}{
		{"webm", "audio/webm;codecs=opus", 10, 200, false}, {"ogg", "audio/ogg", 10, 200, false}, {"wav", "audio/wav", 10, 200, false}, {"mp3", "audio/mpeg", 10, 200, false}, {"mp4", "audio/mp4", 10, 200, false},
		{"empty", "audio/webm", 0, 400, false}, {"too large", "audio/webm", stt.MaxAudioBytes + 1, 413, false}, {"unsupported MIME", "text/plain", 10, 415, false}, {"unavailable", "audio/webm", 10, 503, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			calls := 0
			a := &API{Auth: &auth.Service{}, Config: config.Config{PublicURL: "https://gena.test"}, Logger: slog.New(slog.NewTextHandler(&logs, nil)), Transcriber: transcribeFunc(func(ctx context.Context, n, m string, a []byte) (string, error) {
				calls++
				if tc.providerErr {
					return "", errors.New("provider secret must not leak")
				}
				return "Позвонить Ивану", nil
			})}
			r := audioRequest(t, tc.mime, make([]byte, tc.size))
			w := httptest.NewRecorder()
			a.csrf(http.HandlerFunc(a.transcribe)).ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if tc.status == 200 && !strings.Contains(w.Body.String(), "Позвонить Ивану") {
				t.Fatal(w.Body.String())
			}
			if tc.status < 500 && tc.status != 200 && calls != 0 {
				t.Fatal("invalid upload reached provider")
			}
			if strings.Contains(w.Body.String()+logs.String(), "provider secret") {
				t.Fatal("provider response leaked")
			}
		})
	}
}
func TestVoiceAuthAndCSRF(t *testing.T) {
	a := &API{Auth: &auth.Service{}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Config: config.Config{PublicURL: "https://gena.test", AuthRateLimit: 20}}
	w := httptest.NewRecorder()
	a.Router().ServeHTTP(w, audioRequest(t, "audio/webm", []byte("audio")))
	if w.Code != 401 {
		t.Fatalf("anonymous status=%d", w.Code)
	}
	for _, origin := range []string{"", "https://evil.test"} {
		r := audioRequest(t, "audio/webm", []byte("audio"))
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		a.csrf(http.HandlerFunc(a.transcribe)).ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("origin=%q status=%d", origin, w.Code)
		}
	}
	r := audioRequest(t, "audio/webm", []byte("audio"))
	r.URL.Path = "/api/chat"
	w = httptest.NewRecorder()
	a.csrf(http.HandlerFunc(a.transcribe)).ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("multipart bypassed JSON-only chat")
	}
}
