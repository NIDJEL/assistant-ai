package llm

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/nlp"
	"github.com/google/uuid"
)

const gigaAuthURL = "https://ngw.devices.sberbank.ru:9443/api/v2/oauth"
const gigaBodyLimit = 1 << 20

type GigaChatConfig struct {
	AuthorizationKey                       string `json:"-"`
	BaseURL, AuthURL, Scope, Model, CAFile string
	Timeout                                time.Duration
	MaxConcurrent                          int
}

func (c GigaChatConfig) String() string { return "GigaChatConfig{credentials:redacted}" }

type tokenFlight struct {
	done  chan struct{}
	token string
	err   error
}
type GigaChat struct {
	cfg       GigaChatConfig
	client    *http.Client
	gate      chan struct{}
	mu        sync.Mutex
	token     string
	expiresAt time.Time
	flight    *tokenFlight
	now       func() time.Time
}

func NewGigaChat(cfg GigaChatConfig) (*GigaChat, error) { return newGigaChat(cfg, false) }

func newGigaChat(cfg GigaChatConfig, allowTestHTTP bool) (*GigaChat, error) {
	if cfg.AuthorizationKey == "" || strings.ContainsAny(cfg.AuthorizationKey, "\r\n") {
		return nil, errors.New("GIGACHAT_AUTHORIZATION_KEY is required and must be a single line")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.giga.chat"
	}
	if cfg.AuthURL == "" {
		cfg.AuthURL = gigaAuthURL
	}
	if cfg.Scope == "" {
		cfg.Scope = "GIGACHAT_API_PERS"
	}
	if cfg.Model == "" {
		cfg.Model = "GigaChat-3-Ultra"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxConcurrent == 0 {
		cfg.MaxConcurrent = 1
	}
	if cfg.Timeout <= 0 || cfg.Timeout > 2*time.Minute || cfg.MaxConcurrent < 1 || cfg.MaxConcurrent > 16 {
		return nil, errors.New("invalid GigaChat timeout or concurrency")
	}
	for _, raw := range []string{cfg.BaseURL, cfg.AuthURL} {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, errors.New("invalid GigaChat endpoint")
		}
		local := net.ParseIP(u.Hostname())
		if u.Scheme != "https" && !(allowTestHTTP && u.Scheme == "http" && local != nil && local.IsLoopback()) {
			return nil, errors.New("GigaChat endpoints require HTTPS")
		}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = cfg.MaxConcurrent + 2
	transport.MaxConnsPerHost = cfg.MaxConcurrent + 2
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.CAFile != "" {
		roots, err := x509.SystemCertPool()
		if err != nil {
			return nil, errors.New("cannot load system CA pool")
		}
		pem, err := os.ReadFile(cfg.CAFile)
		if err != nil || !roots.AppendCertsFromPEM(pem) {
			return nil, errors.New("cannot load GIGACHAT_CA_FILE certificates")
		}
		transport.TLSClientConfig.RootCAs = roots
	}
	client := &http.Client{Timeout: cfg.Timeout, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("GigaChat redirects forbidden") }}
	return &GigaChat{cfg: cfg, client: client, gate: make(chan struct{}, cfg.MaxConcurrent), now: time.Now}, nil
}

func (g *GigaChat) Name() string    { return "gigachat" }
func (g *GigaChat) ModelID() string { return g.cfg.Model }
func (g *GigaChat) Close()          { g.client.CloseIdleConnections() }
func (g *GigaChat) String() string  { return "GigaChat{credentials:redacted,token:redacted}" }

// Access tokens exist only here, in memory. Waiters share one refresh result.
func (g *GigaChat) accessToken(ctx context.Context) (string, error) {
	g.mu.Lock()
	if g.token != "" && g.now().Add(time.Minute).Before(g.expiresAt) {
		token := g.token
		g.mu.Unlock()
		return token, nil
	}
	if f := g.flight; f != nil {
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-f.done:
			return f.token, f.err
		}
	}
	f := &tokenFlight{done: make(chan struct{})}
	g.flight = f
	g.mu.Unlock()
	token, expires, err := g.refresh(ctx)
	g.mu.Lock()
	if err == nil {
		g.token, g.expiresAt = token, expires
	}
	f.token, f.err = token, err
	g.flight = nil
	close(f.done)
	g.mu.Unlock()
	return token, err
}

func (g *GigaChat) invalidate(token string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.token == token {
		g.token = ""
		g.expiresAt = time.Time{}
	}
}

func (g *GigaChat) refresh(ctx context.Context) (string, time.Time, error) {
	headers := http.Header{"Authorization": {"Basic " + g.cfg.AuthorizationKey}, "RqUID": {uuid.NewString()}, "Content-Type": {"application/x-www-form-urlencoded"}}
	status, body, _, err := g.exchange(ctx, http.MethodPost, g.cfg.AuthURL, []byte(url.Values{"scope": {g.cfg.Scope}}.Encode()), headers)
	if err != nil {
		return "", time.Time{}, err
	}
	if status != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("GigaChat OAuth HTTP %d", status)
	}
	var response struct {
		Token   string `json:"access_token"`
		Expires int64  `json:"expires_at"`
	}
	if json.Unmarshal(body, &response) != nil || response.Token == "" || len(response.Token) > 65536 || strings.ContainsAny(response.Token, "\r\n") {
		return "", time.Time{}, errors.New("invalid GigaChat token response")
	}
	expires := time.UnixMilli(response.Expires)
	if response.Expires < 1e12 {
		expires = time.Unix(response.Expires, 0)
	}
	if !expires.After(g.now().Add(time.Minute)) {
		return "", time.Time{}, errors.New("invalid GigaChat token expiration")
	}
	return response.Token, expires, nil
}

// Errors deliberately exclude response bodies, headers, URLs and credentials.
func safeNetworkError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var cert *tls.CertificateVerificationError
	if errors.As(err, &cert) {
		return errors.New("GigaChat TLS verification failed")
	}
	var n net.Error
	if errors.As(err, &n) && n.Timeout() {
		return context.DeadlineExceeded
	}
	return errors.New("GigaChat transport failed")
}

func (g *GigaChat) exchange(ctx context.Context, method, endpoint string, payload []byte, headers http.Header) (int, []byte, time.Duration, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, 0, errors.New("invalid GigaChat request")
	}
	request.Header = headers.Clone()
	request.Header.Set("Accept", "application/json")
	response, err := g.client.Do(request)
	if err != nil {
		return 0, nil, 100 * time.Millisecond, safeNetworkError(ctx, err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, gigaBodyLimit+1))
	if err != nil {
		return response.StatusCode, nil, 0, safeNetworkError(ctx, err)
	}
	if len(raw) > gigaBodyLimit {
		return response.StatusCode, nil, 0, errors.New("GigaChat response exceeds limit")
	}
	delay := 100 * time.Millisecond
	if seconds, err := strconv.Atoi(response.Header.Get("Retry-After")); err == nil && seconds > 0 {
		delay = min(time.Duration(seconds)*time.Second, 2*time.Second)
	}
	return response.StatusCode, raw, delay, nil
}

func waitBackoff(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (g *GigaChat) authorized(ctx context.Context, method, path string, payload []byte) ([]byte, error) {
	refreshed, retries := false, 0
	for {
		token, err := g.accessToken(ctx)
		if err != nil {
			return nil, err
		}
		headers := http.Header{"Authorization": {"Bearer " + token}, "Content-Type": {"application/json"}}
		status, raw, delay, err := g.exchange(ctx, method, strings.TrimRight(g.cfg.BaseURL, "/")+path, payload, headers)
		if status == http.StatusUnauthorized {
			g.invalidate(token)
			if !refreshed {
				refreshed = true
				continue
			}
			return nil, fmt.Errorf("GigaChat API HTTP %d", status)
		}
		if err == nil && status == http.StatusOK {
			return raw, nil
		}
		retry := status == 429 || status >= 500 && status <= 599 || status == 0 && err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
		if !retry || retries >= 2 {
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("GigaChat API HTTP %d", status)
		}
		retries++
		if err := waitBackoff(ctx, delay*time.Duration(retries)); err != nil {
			return nil, err
		}
	}
}

func (g *GigaChat) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case g.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *GigaChat) Models(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, g.cfg.Timeout)
	defer cancel()
	if err := g.acquire(ctx); err != nil {
		return nil, err
	}
	defer func() { <-g.gate }()
	raw, err := g.authorized(ctx, http.MethodGet, "/v1/models", nil)
	if err != nil {
		return nil, err
	}
	var response struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &response) != nil || len(response.Data) == 0 {
		return nil, errors.New("invalid GigaChat models response")
	}
	models := make([]string, 0, len(response.Data))
	for _, m := range response.Data {
		models = append(models, m.ID)
	}
	return models, nil
}

func (g *GigaChat) Parse(ctx context.Context, req SemanticRequest) (nlp.SemanticCommand, ProviderMeta, error) {
	var command nlp.SemanticCommand
	var meta ProviderMeta
	if strings.TrimSpace(req.Text) == "" || len([]rune(req.Text)) > 2000 || req.Source != "text" && req.Source != "voice" {
		return command, meta, errors.New("invalid semantic request")
	}
	ctx, cancel := context.WithTimeout(ctx, g.cfg.Timeout)
	defer cancel()
	if err := g.acquire(ctx); err != nil {
		return command, meta, err
	}
	defer func() { <-g.gate }()
	started := time.Now()
	payload, _ := json.Marshal(map[string]any{"model": g.cfg.Model, "messages": semanticMessages(req), "stream": false, "temperature": 0, "max_tokens": 768, "response_format": map[string]any{"type": "json_schema", "schema": nlp.SemanticSchema(), "strict": true}})
	raw, err := g.authorized(ctx, http.MethodPost, "/v1/chat/completions", payload)
	meta.InferenceMS = float64(time.Since(started).Microseconds()) / 1000
	if err != nil {
		return command, meta, err
	}
	var response struct {
		Choices []struct {
			Message struct {
				Role         string          `json:"role"`
				Content      string          `json:"content"`
				FunctionCall json.RawMessage `json:"function_call"`
			} `json:"message"`
			Finish string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			Input  int `json:"prompt_tokens"`
			Output int `json:"completion_tokens"`
			Total  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(raw, &response) != nil || len(response.Choices) != 1 {
		return command, meta, errors.New("invalid GigaChat envelope")
	}
	choice := response.Choices[0]
	if choice.Message.Role != "assistant" || choice.Finish != "stop" || len(choice.Message.FunctionCall) > 0 && string(choice.Message.FunctionCall) != "null" {
		return command, meta, errors.New("incomplete or non-assistant GigaChat response")
	}
	meta.Raw = choice.Message.Content
	meta.InputTokens = response.Usage.Input
	meta.OutputTokens = response.Usage.Output
	meta.TotalTokens = response.Usage.Total
	command, err = nlp.DecodeSemantic([]byte(meta.Raw))
	if err != nil {
		return command, meta, errors.New("invalid SemanticCommand JSON")
	}
	return command, meta, nil
}
