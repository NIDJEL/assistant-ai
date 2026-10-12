package search

import (
	_ "embed"
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/kljensen/snowball"
)

//go:embed data/russian.json
var dictionary []byte

type lexicon struct {
	Lemmas   map[string]string `json:"lemmas"`
	Concepts map[string]string `json:"concepts"`
	Stop     []string          `json:"stop"`
}
type Engine struct {
	dict         lexicon
	stop         map[string]bool
	conceptWords []string
}

func New() (*Engine, error) {
	e := &Engine{stop: map[string]bool{}}
	if err := json.Unmarshal(dictionary, &e.dict); err != nil {
		return nil, err
	}
	for _, w := range e.dict.Stop {
		e.stop[w] = true
	}
	for w := range e.dict.Concepts {
		e.conceptWords = append(e.conceptWords, w)
	}
	sort.Strings(e.conceptWords)
	return e, nil
}

var words = regexp.MustCompile(`[\p{L}\p{N}]+`)

func Normalize(text string) string { return strings.ReplaceAll(strings.ToLower(text), "ё", "е") }
func (e *Engine) lemma(word string) string {
	if value, ok := e.dict.Lemmas[word]; ok {
		return value
	}
	if stem, err := snowball.Stem(word, "russian", true); err == nil {
		return stem
	}
	return word
}
func (e *Engine) Tokens(text string) map[string]bool {
	result := map[string]bool{}
	for _, word := range words.FindAllString(Normalize(text), -1) {
		value := e.lemma(word)
		if e.stop[value] {
			continue
		}
		concept := e.dict.Concepts[value]
		if concept == "" && len([]rune(value)) >= 6 {
			best := 0.0
			for _, candidate := range e.conceptWords {
				score := Ratio(value, candidate)
				if score > best {
					best = score
					if score >= .88 {
						concept = e.dict.Concepts[candidate]
					}
				}
			}
		}
		if concept != "" {
			value = concept
		}
		result[value] = true
	}
	return result
}

// Ratio reproduces normalized Indel similarity on Unicode code points.
func Ratio(a, b string) float64 {
	aa, bb := []rune(a), []rune(b)
	if len(aa)+len(bb) == 0 {
		return 1
	}
	row := make([]int, len(bb)+1)
	for _, x := range aa {
		prev := 0
		for j, y := range bb {
			old := row[j+1]
			if x == y {
				row[j+1] = prev + 1
			} else if row[j] > row[j+1] {
				row[j+1] = row[j]
			}
			prev = old
		}
	}
	return 2 * float64(row[len(bb)]) / float64(len(aa)+len(bb))
}
func (e *Engine) Search(events []domain.Event, query string, limit int) []domain.Event {
	wanted := e.Tokens(query)
	result := []domain.Event{}
	if len(wanted) == 0 {
		return result
	}
	docs := make([]map[string]bool, len(events))
	freq := map[string]int{}
	for i, event := range events {
		docs[i] = e.Tokens(event.Title + " " + event.Description)
		for token := range docs[i] {
			freq[token]++
		}
	}
	weights := map[string]float64{}
	total := 0.0
	for token := range wanted {
		w := math.Log(float64(len(docs)+1)/float64(freq[token]+1)) + 1
		weights[token] = w
		total += w
	}
	for i, event := range events {
		hit := 0.0
		for token := range wanted {
			similarity := 0.0
			if docs[i][token] {
				similarity = 1
			} else if !strings.HasPrefix(token, "concept_") {
				for other := range docs[i] {
					if strings.HasPrefix(other, "concept_") {
						continue
					}
					r := Ratio(token, other)
					if r > similarity {
						similarity = r
					}
				}
			}
			if similarity >= .78 {
				hit += weights[token] * similarity
			}
		}
		score := hit / total
		if score >= .55 {
			rounded := math.Round(score*1000) / 1000
			event.Score = &rounded
			result = append(result, event)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if *a.Score != *b.Score {
			return *a.Score > *b.Score
		}
		ad, bd := "9999", "9999"
		if a.Date != nil {
			ad = a.Date.String()
		}
		if b.Date != nil {
			bd = b.Date.String()
		}
		if ad != bd {
			return ad < bd
		}
		at, bt := "", ""
		if a.StartTime != nil {
			at = a.StartTime.String()
		}
		if b.StartTime != nil {
			bt = b.StartTime.String()
		}
		if at != bt {
			return at < bt
		}
		return a.ID < b.ID
	})
	if limit >= 0 && len(result) > limit {
		return result[:limit]
	}
	return result
}
