package llm

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
)

// GigaEmbeddings reuses the authenticated, TLS-validated GigaChat client.
type GigaEmbeddings struct {
	Client *GigaChat
	Model  string
}

func (g GigaEmbeddings) ModelID() string {
	if g.Model == "" {
		return "Embeddings-2"
	}
	return g.Model
}
func (g GigaEmbeddings) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	if g.Client == nil || len(texts) == 0 || len(texts) > 64 {
		return nil, errors.New("invalid embedding batch")
	}
	for _, text := range texts {
		if len([]rune(text)) > 2000 {
			return nil, errors.New("embedding text too long")
		}
	}
	if err := g.Client.acquire(ctx); err != nil {
		return nil, err
	}
	defer func() { <-g.Client.gate }()
	payload, _ := json.Marshal(map[string]any{"model": g.ModelID(), "input": texts})
	raw, err := g.Client.authorized(ctx, http.MethodPost, "/v1/embeddings", payload)
	if err != nil {
		return nil, err
	}
	var result struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &result) != nil || len(result.Data) != len(texts) {
		return nil, errors.New("invalid embedding response")
	}
	vectors := make([][]float64, len(texts))
	dimension := 0
	for _, item := range result.Data {
		if item.Index < 0 || item.Index >= len(texts) || vectors[item.Index] != nil || len(item.Embedding) == 0 || len(item.Embedding) > 8192 {
			return nil, errors.New("invalid embedding index or dimension")
		}
		if dimension == 0 {
			dimension = len(item.Embedding)
		}
		if len(item.Embedding) != dimension {
			return nil, errors.New("embedding dimension mismatch")
		}
		norm := 0.0
		for _, v := range item.Embedding {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, errors.New("non-finite embedding")
			}
			norm += v * v
		}
		if norm <= 0 {
			return nil, errors.New("zero embedding")
		}
		vectors[item.Index] = item.Embedding
	}
	return vectors, nil
}
