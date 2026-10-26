package search

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

type Embedder interface {
	Embed(context.Context, []string) ([][]float64, error)
	ModelID() string
}
type cacheVector struct {
	vector []float64
	at     time.Time
}
type Hybrid struct {
	Provider               Embedder
	Budget                 time.Duration
	MaxDocuments, MaxCache int
	mu                     sync.Mutex
	cache                  map[string]cacheVector
	unavailableUntil       time.Time
}
type preparedSearch struct {
	mode         string
	calls        int
	scope, query string
	hashes       map[int64][32]byte
	semantic     map[int64]float64
	warning      string
	model        string
}
type preparedKey struct{}

func (e *Engine) EnableHybrid(provider Embedder, budget time.Duration) {
	if budget <= 0 {
		budget = 8 * time.Second
	}
	e.hybrid = &Hybrid{Provider: provider, Budget: budget, MaxDocuments: 2000, MaxCache: 6000, cache: map[string]cacheVector{}}
}
func (e *Engine) HybridEnabled() bool {
	return e.cloud != nil && e.cloud.Provider != nil || e.hybrid != nil && e.hybrid.Provider != nil
}
func docText(event domain.Event) string {
	text := event.Title + " " + event.Description
	for _, tag := range event.Tags {
		text += " " + tag
	}
	r := []rune(text)
	if len(r) > 1800 {
		r = r[:1800]
	}
	return string(r)
}
func docHash(event domain.Event) [32]byte { return sha256.Sum256([]byte(docText(event))) }

func (h *Hybrid) vectors(ctx context.Context, scope string, texts []string) ([][]float64, error) {
	result := make([][]float64, len(texts))
	missing := []int{}
	now := time.Now()
	keys := make([]string, len(texts))
	h.mu.Lock()
	if now.Before(h.unavailableUntil) {
		h.mu.Unlock()
		return nil, fmt.Errorf("embedding provider cooling down")
	}
	for i, text := range texts {
		keys[i] = fmt.Sprintf("%s:%s:%x", scope, h.Provider.ModelID(), sha256.Sum256([]byte(text)))
		if v, ok := h.cache[keys[i]]; ok && now.Sub(v.at) < 30*time.Minute {
			result[i] = v.vector
		} else {
			missing = append(missing, i)
		}
	}
	h.mu.Unlock()
	for start := 0; start < len(missing); start += 32 {
		end := min(start+32, len(missing))
		batch := []string{}
		for _, index := range missing[start:end] {
			batch = append(batch, texts[index])
		}
		vectors, err := h.Provider.Embed(ctx, batch)
		if err != nil {
			h.mu.Lock()
			h.unavailableUntil = time.Now().Add(30 * time.Second)
			h.mu.Unlock()
			return nil, err
		}
		if len(vectors) != len(batch) {
			return nil, fmt.Errorf("embedding batch size mismatch")
		}
		h.mu.Lock()
		for j, index := range missing[start:end] {
			result[index] = vectors[j]
			if len(h.cache) >= h.MaxCache {
				var oldest string
				var at time.Time
				for key, item := range h.cache {
					if oldest == "" || item.at.Before(at) {
						oldest, at = key, item.at
					}
				}
				delete(h.cache, oldest)
			}
			h.cache[keys[index]] = cacheVector{vectors[j], now}
		}
		h.mu.Unlock()
	}
	return result, nil
}

func cosine(a, b []float64) (float64, bool) {
	if len(a) == 0 || len(a) != len(b) {
		return 0, false
	}
	dot, aa, bb := 0.0, 0.0, 0.0
	for i, x := range a {
		y := b[i]
		if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
			return 0, false
		}
		dot += x * y
		aa += x * x
		bb += y * y
	}
	if aa <= 0 || bb <= 0 {
		return 0, false
	}
	return dot / math.Sqrt(aa*bb), true
}

// Prepare performs external I/O before opening a write transaction. Only a
// scoped, content-versioned ranking enters the executor via context.
func (e *Engine) Prepare(ctx context.Context, events []domain.Event, query string) context.Context {
	if !e.HybridEnabled() || query == "" {
		return ctx
	}
	if e.cloud != nil {
		return e.prepareCloud(ctx, events, query)
	}
	all, _ := ctx.Value(preparedKey{}).(map[string]preparedSearch)
	copyMap := map[string]preparedSearch{}
	for key, v := range all {
		copyMap[key] = v
	}
	if previous, ok := copyMap[query]; ok && previous.scope == repository.UserID(ctx) {
		return ctx
	}
	prepared := preparedSearch{scope: repository.UserID(ctx), query: query, hashes: map[int64][32]byte{}, semantic: map[int64]float64{}, model: e.hybrid.Provider.ModelID()}
	child, cancel := context.WithTimeout(ctx, e.hybrid.Budget)
	defer cancel()
	documents := events
	if len(documents) > e.hybrid.MaxDocuments {
		documents = documents[:e.hybrid.MaxDocuments]
		prepared.warning = "Часть истории проверена только по тексту: достигнут предел семантического поиска."
	}
	texts := []string{query}
	for _, event := range documents {
		texts = append(texts, docText(event))
		prepared.hashes[event.ID] = docHash(event)
	}
	vectors, err := e.hybrid.vectors(child, prepared.scope, texts)
	if err != nil {
		prepared.warning = "Смысловой поиск временно недоступен. Использован поиск по словам и синонимам."
	} else {
		for i, event := range documents {
			score, valid := cosine(vectors[0], vectors[i+1])
			if valid && score >= 0.35 {
				prepared.semantic[event.ID] = score
			}
		}
	}
	copyMap[query] = prepared
	return context.WithValue(ctx, preparedKey{}, copyMap)
}

func (e *Engine) SearchContext(ctx context.Context, events []domain.Event, query string, limit int) ([]domain.Event, *domain.SearchStatus) {
	lexical := e.Search(events, query, len(events))
	all, _ := ctx.Value(preparedKey{}).(map[string]preparedSearch)
	key := query
	if e.cloud != nil {
		key = cloudKey(events, query)
	}
	p, ok := all[key]
	if !ok || p.scope != repository.UserID(ctx) {
		status := &domain.SearchStatus{Mode: "lexical"}
		if e.cloud != nil {
			for _, prior := range all {
				if prior.query == query && prior.scope == repository.UserID(ctx) {
					status.Warning = "Набор записей изменился. Использован поиск по словам и синонимам."
					break
				}
			}
		}
		return trimSearch(lexical, limit), status
	}
	status := &domain.SearchStatus{Mode: "hybrid", Model: p.model, Warning: p.warning}
	status.Calls = p.calls
	if p.mode == "cloud" {
		result := []domain.Event{}
		for _, event := range events {
			if p.hashes[event.ID] == docHash(event) && p.semantic[event.ID] > 0 {
				score := p.semantic[event.ID]
				event.Score = &score
				result = append(result, event)
			}
		}
		sort.Slice(result, func(i, j int) bool { return *result[i].Score > *result[j].Score })
		status.Mode = "cloud"
		return trimSearch(result, limit), status
	}
	if len(p.semantic) == 0 {
		status.Mode = "lexical"
		return trimSearch(lexical, limit), status
	}
	semantic := []domain.Event{}
	for _, event := range events {
		if p.hashes[event.ID] == docHash(event) && p.semantic[event.ID] > 0 {
			semantic = append(semantic, event)
		}
	}
	sort.SliceStable(semantic, func(i, j int) bool { return p.semantic[semantic[i].ID] > p.semantic[semantic[j].ID] })
	scores := map[int64]float64{}
	items := map[int64]domain.Event{}
	for i, event := range lexical {
		scores[event.ID] += 1.0 / float64(60+i+1)
		items[event.ID] = event
	}
	for i, event := range semantic {
		scores[event.ID] += .85 / float64(60+i+1)
		items[event.ID] = event
	}
	result := []domain.Event{}
	for id, event := range items {
		value := scores[id]
		event.Score = &value
		result = append(result, event)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if *result[i].Score != *result[j].Score {
			return *result[i].Score > *result[j].Score
		}
		return result[i].ID < result[j].ID
	})
	return trimSearch(result, limit), status
}
func trimSearch(items []domain.Event, limit int) []domain.Event {
	if limit >= 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}
