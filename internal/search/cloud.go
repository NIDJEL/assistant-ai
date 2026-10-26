package search

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

type QueryModel interface {
	ModelID() string
	Rewrite(context.Context, string) ([]string, error)
	Rerank(context.Context, string, []domain.Event) ([]int, error)
}
type vocabularyRewriter interface {
	RewriteWithVocabulary(context.Context, string, []string) ([]string, error)
}

func (e *Engine) searchVocabulary(events []domain.Event) []string {
	counts := map[string]int{}
	for _, event := range events {
		for _, token := range words.FindAllString(Normalize(event.Title), -1) {
			if len([]rune(token)) >= 3 && !e.stop[token] {
				counts[token]++
			}
		}
	}
	terms := make([]string, 0, len(counts))
	for token := range counts {
		terms = append(terms, token)
	}
	sort.Slice(terms, func(i, j int) bool {
		if counts[terms[i]] != counts[terms[j]] {
			return counts[terms[i]] > counts[terms[j]]
		}
		return terms[i] < terms[j]
	})
	if len(terms) > 384 {
		terms = terms[:384]
	}
	// Short real titles retain relationships which a bag of words loses.
	// This catalogue is derived only from the caller's already filtered pool.
	titles := map[string]int{}
	for _, event := range events {
		title := []rune(event.Title)
		if len(title) > 100 {
			title = title[:100]
		}
		titles[string(title)]++
	}
	names := make([]string, 0, len(titles))
	for name := range titles {
		names = append(names, name)
	}
	sort.Strings(names)
	tokens := make([][]string, len(names))
	df := map[string]int{}
	for i, name := range names {
		seen := map[string]bool{}
		for _, word := range words.FindAllString(Normalize(name), -1) {
			if len([]rune(word)) < 3 || e.stop[word] {
				continue
			}
			// Catalogue diversity is based on words, not row numbers or dates.
			onlyNumber := true
			for _, r := range word {
				if r < '0' || r > '9' {
					onlyNumber = false
					break
				}
			}
			if onlyNumber {
				continue
			}
			token := e.tokenLemma(word)
			if !seen[token] {
				seen[token] = true
				tokens[i] = append(tokens[i], token)
				df[token]++
			}
		}
	}
	selected := []string{}
	used := map[int]bool{}
	covered := map[string]int{}
	for len(selected) < min(len(names), 128) {
		best, bestScore := -1, -1.0
		for i, name := range names {
			if used[i] {
				continue
			}
			score := 0.0
			for _, token := range tokens[i] {
				score += 1.0 / (float64(1+covered[token]) * math.Sqrt(float64(df[token])))
			}
			if score > bestScore || score == bestScore && best >= 0 && titles[name] > titles[names[best]] {
				best, bestScore = i, score
			}
		}
		if best < 0 {
			break
		}
		used[best] = true
		selected = append(selected, names[best])
		for _, token := range tokens[best] {
			covered[token]++
		}
	}
	names = selected
	terms = append(terms, names...)
	return terms
}

type rewriteEntry struct {
	Queries []string
	At      time.Time
}
type Cloud struct {
	Provider QueryModel
	Budget   time.Duration
	mu       sync.Mutex
	rewrites map[string]rewriteEntry
}

// Bind a ranking to its complete filtered pool, not just the returned top ten.
// IDs and content versions are sorted so repository ordering cannot change it.
func cloudKey(events []domain.Event, query string) string {
	versions := make([]string, 0, len(events))
	for _, event := range events {
		versions = append(versions, fmt.Sprintf("%d:%x", event.ID, docHash(event)))
	}
	sort.Strings(versions)
	return fmt.Sprintf("%s:%x", query, sha256.Sum256([]byte(strings.Join(versions, ";"))))
}

func (e *Engine) EnableCloud(provider QueryModel, budget time.Duration) {
	if budget <= 0 {
		budget = 8 * time.Second
	}
	e.cloud = &Cloud{Provider: provider, Budget: budget, rewrites: map[string]rewriteEntry{}}
}

func (e *Engine) prepareCloud(ctx context.Context, events []domain.Event, query string) context.Context {
	all, _ := ctx.Value(preparedKey{}).(map[string]preparedSearch)
	copyMap := map[string]preparedSearch{}
	for key, p := range all {
		copyMap[key] = p
	}
	scope := repository.UserID(ctx)
	poolKey := cloudKey(events, query)
	if previous, ok := copyMap[poolKey]; ok && previous.scope == scope {
		return ctx
	}
	p := preparedSearch{scope: scope, query: query, mode: "cloud", model: e.cloud.Provider.ModelID(), hashes: map[int64][32]byte{}, semantic: map[int64]float64{}}
	if len(events) == 0 {
		copyMap[poolKey] = p
		return context.WithValue(ctx, preparedKey{}, copyMap)
	}
	child, cancel := context.WithTimeout(ctx, e.cloud.Budget)
	defer cancel()
	var vocabulary []string
	if _, ok := e.cloud.Provider.(vocabularyRewriter); ok {
		vocabulary = e.searchVocabulary(events)
	}
	key := fmt.Sprintf("%s:%s:%s:%x", scope, p.model, query, sha256.Sum256([]byte(strings.Join(vocabulary, ";"))))
	e.cloud.mu.Lock()
	entry, cached := e.cloud.rewrites[key]
	e.cloud.mu.Unlock()
	queries := entry.Queries
	if !cached || time.Since(entry.At) > 5*time.Minute {
		p.calls++
		var err error
		if provider, ok := e.cloud.Provider.(vocabularyRewriter); ok {
			queries, err = provider.RewriteWithVocabulary(child, query, vocabulary)
		} else {
			queries, err = e.cloud.Provider.Rewrite(child, query)
		}
		if err != nil {
			p.warning = "Смысловой поиск временно недоступен. Использован поиск по словам и синонимам."
			p.mode = "lexical"
			copyMap[poolKey] = p
			return context.WithValue(ctx, preparedKey{}, copyMap)
		}
		if len(queries) < 1 || len(queries) > 6 {
			queries = nil
		}
		if len(queries) > 0 {
			e.cloud.mu.Lock()
			if len(e.cloud.rewrites) >= 1000 {
				oldest := ""
				var at time.Time
				for k, v := range e.cloud.rewrites {
					if oldest == "" || v.At.Before(at) {
						oldest, at = k, v.At
					}
				}
				delete(e.cloud.rewrites, oldest)
			}
			e.cloud.rewrites[key] = rewriteEntry{append([]string{}, queries...), time.Now()}
			e.cloud.mu.Unlock()
		}
	}
	scores := map[int64]float64{}
	items := map[int64]domain.Event{}
	seen := map[string]bool{}
	for _, q := range append([]string{query}, queries...) {
		q = strings.TrimSpace(q)
		if q == "" || len([]rune(q)) > 300 || seen[q] {
			continue
		}
		seen[q] = true
		weight := 1.0
		if q == query {
			weight = .2
		}
		for i, event := range e.Search(events, q, 100) {
			scores[event.ID] += weight / float64(60+i+1)
			items[event.ID] = event
		}
	}
	candidates := make([]domain.Event, 0, len(items))
	for _, event := range items {
		candidates = append(candidates, event)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if scores[candidates[i].ID] != scores[candidates[j].ID] {
			return scores[candidates[i].ID] > scores[candidates[j].ID]
		}
		return candidates[i].ID < candidates[j].ID
	})
	if len(candidates) > 40 {
		candidates = candidates[:40]
	}
	if len(candidates) > 0 {
		p.calls++
		indices, err := e.cloud.Provider.Rerank(child, query, candidates)
		used := map[int]bool{}
		if len(indices) > 10 {
			err = fmt.Errorf("too many search indices")
		}
		for _, index := range indices {
			if index < 1 || index > len(candidates) || used[index] {
				err = fmt.Errorf("invalid search index")
			}
			used[index] = true
		}
		if err != nil {
			p.warning = "Уточняющее ранжирование недоступно. Использованы смысловые поисковые фразы."
			indices = make([]int, min(len(candidates), 10))
			for i := range indices {
				indices[i] = i + 1
			}
		}
		for rank, index := range indices {
			event := candidates[index-1]
			p.hashes[event.ID] = docHash(event)
			p.semantic[event.ID] = float64(len(indices) - rank)
		}
	}
	copyMap[poolKey] = p
	return context.WithValue(ctx, preparedKey{}, copyMap)
}
