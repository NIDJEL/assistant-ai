package tts

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func mockAzure(t *testing.T, handler http.HandlerFunc) *AzureSynthesizer {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	s, err := NewAzure("test-secret", "eastus", "")
	if err != nil {
		t.Fatal(err)
	}
	s.endpoint = server.URL
	t.Cleanup(s.Close)
	return s
}

const voiceList = `[{"ShortName":"ru-RU-DmitryNeural","Locale":"ru-RU"},{"ShortName":"ru-RU-SvetlanaNeural","Locale":"ru-RU"}]`

func TestAzureSSMLCacheAndVoices(t *testing.T) {
	var calls atomic.Int32
	s := mockAzure(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Ocp-Apim-Subscription-Key") != "test-secret" {
			t.Error("missing authentication")
		}
		if r.URL.Path == "/voices/list" {
			io.WriteString(w, voiceList)
			return
		}
		calls.Add(1)
		if r.Header.Get("Content-Type") != "application/ssml+xml" || r.Header.Get("X-Microsoft-OutputFormat") != OutputFormat {
			t.Error("wrong synthesis headers")
		}
		raw, _ := io.ReadAll(r.Body)
		if strings.Contains(string(raw), "<break") || !strings.Contains(string(raw), "&amp;") {
			t.Error("unescaped markup")
		}
		decoder := xml.NewDecoder(strings.NewReader(string(raw)))
		for {
			_, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Error(err)
				break
			}
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		io.WriteString(w, "MP3")
	})
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			audio, err := s.Synthesize(context.Background(), "**Привет** & <break/> 20.10.2026 в 10:00", "")
			if err != nil || string(audio) != "MP3" {
				t.Errorf("synthesis failed: %v", err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("cache misses=%d", calls.Load())
	}
	voices, err := s.Voices(context.Background())
	if err != nil || len(voices) != 1 || voices[0].ID != DefaultVoice {
		t.Fatal("only Dmitry should be offered")
	}
	if _, err := s.Synthesize(context.Background(), "Другой ответ & 20.10.2026 в 10:00", DefaultVoice); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("text not part of cache key")
	}
	if _, err := s.Synthesize(context.Background(), "Привет", "ru-RU-DariyaNeural"); !errors.Is(err, ErrVoice) {
		t.Fatal("removed voice accepted")
	}
	if _, err := s.Synthesize(context.Background(), "Привет", "ru-RU-SvetlanaNeural"); !errors.Is(err, ErrVoice) {
		t.Fatal("removed voice accepted")
	}
	if _, err := s.Synthesize(context.Background(), "Привет", `evil"><break/>`); !errors.Is(err, ErrVoice) {
		t.Fatal("voice injection accepted")
	}
}

func TestAzureFailuresAndLimits(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"throttle", 429, "secret provider error", ErrRateLimited}, {"upstream", 503, "secret provider error", ErrUnavailable}, {"empty", 200, "", ErrUnavailable}, {"oversized", 200, strings.Repeat("a", MaxAudioBytes+1), ErrUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := mockAzure(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/voices/list" {
					io.WriteString(w, voiceList)
					return
				}
				w.Header().Set("Content-Type", "audio/mpeg")
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			})
			_, err := s.Synthesize(context.Background(), "Привет", "")
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v", err)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("upstream body exposed")
			}
		})
	}
	s := mockAzure(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, voiceList) })
	if _, err := s.Synthesize(context.Background(), strings.Repeat("я", MaxTextRunes+1), ""); !errors.Is(err, ErrInvalidText) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Synthesize(ctx, "Привет", ""); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	s.day = time.Now().UTC().Format("2006-01-02")
	s.dailyUsed = dailyCharacters
	if _, err := s.Synthesize(context.Background(), "Привет", ""); !errors.Is(err, ErrRateLimited) {
		t.Fatal("daily budget ignored")
	}
}

func TestAzureCancellationAndCacheEviction(t *testing.T) {
	s := mockAzure(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/voices/list" {
			io.WriteString(w, voiceList)
			return
		}
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := s.Synthesize(ctx, "Привет", ""); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	s2 := mockAzure(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/voices/list" {
			io.WriteString(w, voiceList)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		io.WriteString(w, strings.Repeat("a", 1<<20))
	})
	for i := range 20 {
		if _, err := s2.Synthesize(context.Background(), "Привет "+strings.Repeat("я", i), ""); err != nil {
			t.Fatal(err)
		}
	}
	if s2.bytes > cacheBytes || s2.lru.Len() > cacheEntries || len(s2.cache) != s2.lru.Len() {
		t.Fatal("unbounded cache")
	}
}

func TestPrepareText(t *testing.T) {
	for input, want := range map[string]string{
		"## Готово\n- **Встреча** [завтра](https://example.test) в 10:00.": "Готово Встреча завтра в 10:00.",
		"Ответ.\n```json\n{\"secret\":1}\n```":                             "Ответ.",
		`{"title":"Технический JSON"}`:                                     "",
		"<p>20 октября &amp; 21 октября</p><script>code()</script>":        "20 октября & 21 октября",
	} {
		if got := PrepareText(input); got != want {
			t.Errorf("PrepareText(%q)=%q want %q", input, got, want)
		}
	}
}
