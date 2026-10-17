package stt

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"
)

const groqEndpoint = "https://api.groq.com/openai/v1/audio/transcriptions"
const DefaultModel = "whisper-large-v3-turbo"
const maxResponseBytes = 64 << 10

type Groq struct {
	key, model, endpoint string
	client               *http.Client
}

func NewGroq(key, model string) *Groq {
	if model == "" {
		model = DefaultModel
	}
	return &Groq{key: key, model: model, endpoint: groqEndpoint, client: &http.Client{
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (g *Groq) Transcribe(ctx context.Context, filename, contentType string, audio []byte) (string, error) {
	if err := Validate(contentType, audio); err != nil {
		return "", err
	}
	if g.key == "" {
		return "", ErrUnavailable
	}
	media, _ := AudioMIME(contentType)
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "file", "filename": filename}))
	header.Set("Content-Type", media)
	part, err := w.CreatePart(header)
	if err != nil {
		return "", ErrUnavailable
	}
	if _, err = part.Write(audio); err != nil {
		return "", ErrUnavailable
	}
	for _, field := range [][2]string{{"model", g.model}, {"language", "ru"}, {"response_format", "json"}, {"temperature", "0"}} {
		if err = w.WriteField(field[0], field[1]); err != nil {
			return "", ErrUnavailable
		}
	}
	if err = w.Close(); err != nil {
		return "", ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint, &body)
	if err != nil {
		return "", ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+g.key)
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err := g.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", ErrUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil || len(raw) > maxResponseBytes {
		return "", ErrUnavailable
	}
	var result struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &result) != nil || strings.TrimSpace(result.Text) == "" {
		return "", ErrUnavailable
	}
	return strings.TrimSpace(result.Text), nil
}
