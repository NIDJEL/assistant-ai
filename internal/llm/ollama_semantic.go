package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/nlp"
)

// Ollama uses the very same SemanticCommand contract as GigaChat.
type OllamaSemantic struct {
	URL, Model string
	Client     *http.Client
}

func NewOllamaSemantic(endpoint, model string, timeout time.Duration) (*OllamaSemantic, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return nil, errors.New("semantic Ollama requires a loopback HTTP origin")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return nil, errors.New("semantic Ollama requires a literal loopback address")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &OllamaSemantic{URL: endpoint, Model: model, Client: &http.Client{Timeout: timeout, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("semantic redirects forbidden") }}}, nil
}
func (o *OllamaSemantic) Name() string    { return "ollama" }
func (o *OllamaSemantic) ModelID() string { return o.Model }
func (o *OllamaSemantic) Parse(ctx context.Context, req SemanticRequest) (nlp.SemanticCommand, ProviderMeta, error) {
	var command nlp.SemanticCommand
	var meta ProviderMeta
	payload, _ := json.Marshal(map[string]any{"model": o.Model, "messages": semanticMessages(req), "format": nlp.SemanticSchema(), "stream": false, "think": false, "options": map[string]any{"temperature": 0, "num_ctx": 4096, "num_predict": 768, "seed": 0}})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(o.URL, "/")+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return command, meta, errors.New("invalid semantic request")
	}
	request.Header.Set("Content-Type", "application/json")
	started := time.Now()
	response, err := o.Client.Do(request)
	if err != nil {
		return command, meta, safeNetworkError(ctx, err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return command, meta, errors.New("semantic Ollama HTTP failure")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, gigaBodyLimit+1))
	if err != nil || len(raw) > gigaBodyLimit {
		return command, meta, errors.New("invalid semantic response size")
	}
	var envelope struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Done   bool   `json:"done"`
		Reason string `json:"done_reason"`
		Input  int    `json:"prompt_eval_count"`
		Output int    `json:"eval_count"`
	}
	if json.Unmarshal(raw, &envelope) != nil || !envelope.Done || envelope.Reason == "length" {
		return command, meta, errors.New("incomplete semantic response")
	}
	meta = ProviderMeta{Raw: envelope.Message.Content, InputTokens: envelope.Input, OutputTokens: envelope.Output, TotalTokens: envelope.Input + envelope.Output, InferenceMS: float64(time.Since(started).Microseconds()) / 1000}
	command, err = nlp.DecodeSemantic([]byte(meta.Raw))
	if err != nil {
		return command, meta, errors.New("invalid SemanticCommand JSON")
	}
	return command, meta, nil
}
