package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/nlp"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

func semanticFixture(t *testing.T, raw string) string {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatal("invalid fixture")
	}
	var fill func(map[string]any, map[string]any)
	fill = func(v, s map[string]any) {
		props := s["properties"].(map[string]any)
		for _, key := range s["required"].([]string) {
			if _, ok := v[key]; !ok {
				v[key] = nil
			}
			if key == "notification_context" || key == "all" {
				if v[key] == nil {
					v[key] = false
				}
			}
			if child, ok := v[key].(map[string]any); ok {
				fill(child, props[key].(map[string]any))
			}
		}
	}
	fill(value, nlp.SemanticSchema())
	data, _ := json.Marshal(value)
	return string(data)
}

func okSemantic(t *testing.T) string {
	return semanticFixture(t, `{"status":"ok","action":"create","kind":"task","title":"Отправить отчет"}`)
}
func tokenResponse(w http.ResponseWriter, token string) {
	_ = json.NewEncoder(w).Encode(map[string]any{"access_token": token, "expires_at": time.Now().Add(30 * time.Minute).UnixMilli()})
}
func chatResponse(w http.ResponseWriter, raw string) {
	_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": raw}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 40, "total_tokens": 140}})
}
func testGiga(t *testing.T, handler http.HandlerFunc) *GigaChat {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	g, err := newGigaChat(GigaChatConfig{AuthorizationKey: "unit-authorization-secret", BaseURL: server.URL, AuthURL: server.URL + "/oauth", Model: "unit-model"}, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(g.Close)
	return g
}

func TestGigaChatTokenConcurrentRefreshAndExpiry(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	g := testGiga(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Basic unit-authorization-secret" {
			t.Error("wrong OAuth authorization")
		}
		if _, err := uuid.Parse(r.Header.Get("RqUID")); err != nil {
			t.Error("missing RqUID")
		}
		if err := r.ParseForm(); err != nil || r.Form.Get("scope") != "GIGACHAT_API_PERS" {
			t.Error("wrong scope")
		}
		n := calls.Add(1)
		if n == 1 {
			<-release
		}
		tokenResponse(w, fmt.Sprintf("token-%d", n))
	})
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := g.accessToken(t.Context())
			if err != nil || token != "token-1" {
				t.Error("refresh failed")
			}
		}()
	}
	for calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("refresh count %d", calls.Load())
	}
	g.mu.Lock()
	g.expiresAt = time.Now().Add(30 * time.Second)
	g.mu.Unlock()
	if token, err := g.accessToken(t.Context()); err != nil || token != "token-2" {
		t.Fatal("expiry did not refresh")
	}
	g.invalidate("token-1")
	if _, err := g.accessToken(t.Context()); err != nil || calls.Load() != 2 {
		t.Fatal("stale invalidation removed fresh token")
	}
}

func TestGigaChatTokenWaitHonorsContext(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	g := testGiga(t, func(w http.ResponseWriter, r *http.Request) { close(started); <-release; tokenResponse(w, "token") })
	done := make(chan error, 1)
	go func() { _, err := g.accessToken(t.Context()); done <- err }()
	<-started
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := g.accessToken(ctx); !errors.Is(err, context.Canceled) {
		t.Error("refresh waiter ignored cancellation")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestGigaChatInvalidTokenResponseRedaction(t *testing.T) {
	for _, body := range []string{`{`, `{}`, `{"access_token":"", "expires_at":1}`, `{"access_token":"token", "expires_at":1}`, `{"access_token":4,"expires_at":9999999999999}`} {
		t.Run(fmt.Sprint(len(body)), func(t *testing.T) {
			g := testGiga(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) })
			_, err := g.accessToken(t.Context())
			if err == nil || strings.Contains(err.Error(), "unit-authorization-secret") || strings.Contains(err.Error(), body) {
				t.Fatal("invalid token response was accepted or leaked")
			}
		})
	}
	for _, status := range []int{400, 401, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			g := testGiga(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte("unit-authorization-secret access-token-secret"))
			})
			_, err := g.accessToken(t.Context())
			if err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatal("OAuth error leaked a credential")
			}
			if strings.Contains(fmt.Sprint(g, g.cfg), "unit-authorization-secret") {
				t.Fatal("credential formatting leak")
			}
		})
	}
}

func TestGigaChat401RefreshExactlyOnce(t *testing.T) {
	raw := okSemantic(t)
	for _, always401 := range []bool{false, true} {
		t.Run(fmt.Sprint(always401), func(t *testing.T) {
			var auth, chat atomic.Int32
			g := testGiga(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/oauth" {
					tokenResponse(w, fmt.Sprintf("token-%d", auth.Add(1)))
					return
				}
				chat.Add(1)
				if always401 || r.Header.Get("Authorization") == "Bearer token-1" {
					w.WriteHeader(401)
					return
				}
				chatResponse(w, raw)
			})
			_, _, err := g.Parse(t.Context(), SemanticRequest{Text: "нужно отправить отчет", Source: "text"})
			if (err != nil) != always401 || auth.Load() != 2 || chat.Load() != 2 {
				t.Fatalf("401 counts auth=%d chat=%d error=%v", auth.Load(), chat.Load(), err)
			}
		})
	}
}

func TestGigaChatStructuredRequestAndStrictJSON(t *testing.T) {
	valid := okSemantic(t)
	variants := map[string]string{"valid": valid, "malformed": "{", "extra": strings.TrimSuffix(valid, "}") + `,"target_id":15}`, "missing": `{"status":"unknown"}`, "enum": strings.Replace(valid, `"create"`, `"drop"`, 1), "trailing": valid + "{}", "duplicate": strings.TrimSuffix(valid, "}") + `,"status":"ok"}`, "array": "[]"}
	for name, raw := range variants {
		t.Run(name, func(t *testing.T) {
			g := testGiga(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/oauth" {
					tokenResponse(w, "token")
					return
				}
				var request struct {
					Model  string `json:"model"`
					Format struct {
						Type   string         `json:"type"`
						Schema map[string]any `json:"schema"`
						Strict bool           `json:"strict"`
					} `json:"response_format"`
				}
				if json.NewDecoder(r.Body).Decode(&request) != nil || !request.Format.Strict || request.Format.Type != "json_schema" || request.Format.Schema["additionalProperties"] != false || request.Model != "unit-model" {
					t.Error("missing structured JSON schema")
				}
				chatResponse(w, raw)
			})
			command, meta, err := g.Parse(t.Context(), SemanticRequest{Text: "нужно отправить отчет", Source: "text"})
			if (err == nil) != (name == "valid") {
				t.Fatalf("strict validation error=%v", err)
			}
			if name == "valid" && (command.Status != "ok" || meta.TotalTokens != 140) {
				t.Fatal("lost semantic result/usage")
			}
		})
	}
}

func TestGigaChatBoundedRetries(t *testing.T) {
	raw := okSemantic(t)
	for _, status := range []int{429, 500} {
		for _, recover := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/%v", status, recover), func(t *testing.T) {
				var chats atomic.Int32
				g := testGiga(t, func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/oauth" {
						tokenResponse(w, "token")
						return
					}
					n := chats.Add(1)
					if recover && n == 3 {
						chatResponse(w, raw)
						return
					}
					w.WriteHeader(status)
					_, _ = w.Write([]byte("access-token-secret"))
				})
				_, _, err := g.Parse(t.Context(), SemanticRequest{Text: "нужно отправить отчет", Source: "text"})
				if chats.Load() != 3 || (err == nil) != recover {
					t.Fatalf("retry count %d error=%v", chats.Load(), err)
				}
				if err != nil && strings.Contains(err.Error(), "secret") {
					t.Fatal("API body leaked")
				}
			})
		}
	}
}

func TestGigaChatTimeoutCancellationAndSemaphore(t *testing.T) {
	raw := okSemantic(t)
	var active, peak, calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	g := testGiga(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth" {
			tokenResponse(w, "token")
			return
		}
		calls.Add(1)
		n := active.Add(1)
		defer active.Add(-1)
		for {
			old := peak.Load()
			if old >= n || peak.CompareAndSwap(old, n) {
				break
			}
		}
		once.Do(func() { close(started) })
		select {
		case <-release:
			chatResponse(w, raw)
		case <-r.Context().Done():
		}
	})
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := g.Parse(t.Context(), SemanticRequest{Text: "нужно отправить отчет", Source: "text"}); err != nil {
				t.Error(err)
			}
		}()
	}
	<-started
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	_, _, err := g.Parse(ctx, SemanticRequest{Text: "нужно отправить отчет", Source: "text"})
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Error("queued request ignored deadline")
	}
	close(release)
	wg.Wait()
	if peak.Load() != 1 || calls.Load() != 8 {
		t.Fatalf("peak=%d calls=%d", peak.Load(), calls.Load())
	}
}

func TestGigaChatRequestTimeoutBodyLimitAndTLS(t *testing.T) {
	for _, mode := range []string{"timeout", "size", "incomplete"} {
		t.Run(mode, func(t *testing.T) {
			raw := okSemantic(t)
			g := testGiga(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/oauth" {
					tokenResponse(w, "token")
					return
				}
				switch mode {
				case "timeout":
					time.Sleep(80 * time.Millisecond)
				case "size":
					_, _ = w.Write([]byte(strings.Repeat("x", gigaBodyLimit+1)))
				case "incomplete":
					_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": raw}, "finish_reason": "length"}}})
				}
			})
			if mode == "timeout" {
				g.cfg.Timeout = 20 * time.Millisecond
			}
			_, _, err := g.Parse(t.Context(), SemanticRequest{Text: "нужно отправить отчет", Source: "text"})
			if err == nil {
				t.Fatal("invalid response accepted")
			}
			if mode == "timeout" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal(err)
			}
		})
	}
	if _, err := NewGigaChat(GigaChatConfig{AuthorizationKey: "test", BaseURL: "http://api.giga.chat"}); err == nil {
		t.Fatal("HTTP allowed")
	}
	if _, err := NewGigaChat(GigaChatConfig{AuthorizationKey: "test", CAFile: "not-existing"}); err == nil {
		t.Fatal("bad CA allowed")
	}
	g := testGiga(t, func(w http.ResponseWriter, r *http.Request) { tokenResponse(w, "token") })
	transport := g.client.Transport.(*http.Transport)
	if transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("TLS verification disabled")
	}
}

func TestGigaChatIntegration(t *testing.T) {
	if os.Getenv("GIGACHAT_INTEGRATION") != "1" {
		t.Skip("requires explicit GIGACHAT_INTEGRATION=1")
	}
	file, err := godotenv.Read("../../.env")
	if err != nil {
		t.Fatal("cannot load local integration configuration")
	}
	value := func(key string) string {
		if v, ok := os.LookupEnv(key); ok {
			return v
		}
		return file[key]
	}
	g, err := NewGigaChat(GigaChatConfig{AuthorizationKey: value("GIGACHAT_AUTHORIZATION_KEY"), BaseURL: value("GIGACHAT_BASE_URL"), Scope: value("GIGACHAT_SCOPE"), Model: value("GIGACHAT_MODEL"), CAFile: value("GIGACHAT_CA_FILE")})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	if models, err := g.Models(t.Context()); err != nil || len(models) == 0 {
		t.Fatal("live models check failed")
	}
	s, _, err := g.Parse(t.Context(), SemanticRequest{Text: "Напомни завтра в 9 проверить почту", Source: "text"})
	if err != nil || s.Status != "ok" {
		t.Fatal("live structured check failed")
	}
}

func TestGigaChatReusableConnectionsAndModels(t *testing.T) {
	var mu sync.Mutex
	peers := map[string]bool{}
	raw := okSemantic(t)
	g := testGiga(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		peers[r.RemoteAddr] = true
		mu.Unlock()
		switch r.URL.Path {
		case "/oauth":
			tokenResponse(w, "token")
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"unit-model"}]}`))
		default:
			chatResponse(w, raw)
		}
	})
	if ids, err := g.Models(t.Context()); err != nil || len(ids) != 1 || ids[0] != "unit-model" {
		t.Fatal("models failed")
	}
	for range 3 {
		if _, _, err := g.Parse(t.Context(), SemanticRequest{Text: "нужно отправить отчет", Source: "text"}); err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(peers) != 1 {
		t.Fatal("connections not reused")
	}
}
