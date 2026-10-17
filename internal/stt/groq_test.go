package stt

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGroqMultipartSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Error("invalid request authentication/method")
		}
		reader, err := r.MultipartReader()
		if err != nil {
			t.Error(err)
			return
		}
		fields := map[string]string{}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Error(err)
				return
			}
			raw, _ := io.ReadAll(part)
			fields[part.FormName()] = string(raw)
			if part.FormName() == "file" && (part.FileName() != "recording.webm" || part.Header.Get("Content-Type") != "audio/webm") {
				t.Error("lost file metadata")
			}
		}
		for k, v := range map[string]string{"file": "test audio", "model": DefaultModel, "language": "ru", "response_format": "json", "temperature": "0"} {
			if fields[k] != v {
				t.Errorf("field %s missing or wrong", k)
			}
		}
		io.WriteString(w, `{"text":"  Напомни завтра в десять позвонить Ивану  "}`)
	}))
	defer server.Close()
	g := NewGroq("test-api-key", "")
	g.endpoint = server.URL
	text, err := g.Transcribe(context.Background(), "recording.webm", "audio/webm;codecs=opus", []byte("test audio"))
	if err != nil || text != "Напомни завтра в десять позвонить Ивану" {
		t.Fatalf("text=%q err=%v", text, err)
	}
}

func TestGroqFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"4xx", 401, `secret provider response`}, {"5xx", 503, `secret provider response`},
		{"invalid JSON", 200, `{`}, {"empty transcript", 200, `{"text":" "}`}, {"missing transcript", 200, `{}`},
		{"oversized response", 200, strings.Repeat("x", maxResponseBytes+1)},
		{"trailing JSON", 200, `{"text":"hi"}{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); io.WriteString(w, tc.body) }))
			defer s.Close()
			g := NewGroq("test-key", "")
			g.endpoint = s.URL
			if _, err := g.Transcribe(context.Background(), "audio.wav", "audio/wav", []byte("audio")); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("unsafe error: %v", err)
			}
		})
	}
}

func TestGroqTimeoutAndCancellation(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(80 * time.Millisecond)
		io.WriteString(w, `{"text":"late"}`)
	}))
	defer s.Close()
	g := NewGroq("test-key", "")
	g.endpoint = s.URL
	g.client.Timeout = 10 * time.Millisecond
	if _, err := g.Transcribe(context.Background(), "audio.wav", "audio/wav", []byte("audio")); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("timeout=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.Transcribe(ctx, "audio.wav", "audio/wav", []byte("audio")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%v", err)
	}
}

func TestGroqInputLimitsAndNoRedirect(t *testing.T) {
	g := NewGroq("test-key", "")
	for _, tc := range []struct {
		mime  string
		audio []byte
		want  error
	}{
		{"audio/wav", nil, ErrEmpty}, {"audio/wav", make([]byte, MaxAudioBytes+1), ErrTooLarge}, {"text/plain", []byte("audio"), ErrMIME},
	} {
		if _, err := g.Transcribe(context.Background(), "audio", tc.mime, tc.audio); !errors.Is(err, tc.want) {
			t.Fatalf("err=%v want=%v", err, tc.want)
		}
	}
	called := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer destination.Close()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 307) }))
	defer s.Close()
	g.endpoint = s.URL
	if _, err := g.Transcribe(context.Background(), "audio.wav", "audio/wav", bytes.Repeat([]byte{1}, 5)); !errors.Is(err, ErrUnavailable) || called {
		t.Fatal("followed provider redirect")
	}
}
