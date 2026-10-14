package search

import (
	"strings"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/kljensen/snowball"
)

func titleKey(text string) string {
	return strings.Join(words.FindAllString(Normalize(text), -1), " ")
}

// TitleMatches is deliberately stricter than search: every meaningful word
// must occur in the title. Synonyms, typos and description-only hits are useful
// suggestions, but must never silently select a record for a mutation.
func (e *Engine) TitleMatches(events []domain.Event, query string, exactOnly bool) ([]domain.Event, bool) {
	exact := []domain.Event{}
	for _, event := range events {
		if titleKey(event.Title) == titleKey(query) {
			exact = append(exact, event)
		}
	}
	if exactOnly {
		return exact, true
	}
	tokens := func(text string) map[string]bool {
		result := map[string]bool{}
		for _, word := range words.FindAllString(Normalize(text), -1) {
			if strings.Contains("|а|и|в|во|на|к|с|со|по|про|о|об|от|для|из|у|за|это|", "|"+word+"|") {
				continue
			}
			word = e.lemma(word)
			if stem, err := snowball.Stem(word, "russian", true); err == nil {
				word = stem
			}
			result[word] = true
		}
		return result
	}
	wanted := tokens(query)
	result := []domain.Event{}
	if len(wanted) == 0 {
		return result, true
	}
	for _, event := range events {
		doc, matches := tokens(event.Title), true
		for token := range wanted {
			if !doc[token] {
				matches = false
				break
			}
		}
		if matches {
			score := 1.0
			event.Score = &score
			result = append(result, event)
		}
	}
	if len(result) > 0 {
		return result, true
	}
	return e.Search(events, query, 10), false
}
