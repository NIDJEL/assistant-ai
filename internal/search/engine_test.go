package search_test

import (
	"sync"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/search"
)

func TestInflectionsDescriptionAndConcurrentNormalization(t *testing.T) {
	e, err := search.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"Иваном", "Ивану", "Ивана"} {
		tokens := e.Tokens(word)
		if !tokens["иван"] {
			t.Fatalf("inflection %s: %v", word, tokens)
		}
	}
	events := []domain.Event{{ID: 1, Title: "Документы", Description: "Проверить договор с Иваном", Kind: domain.EventTask, Duration: time.Hour}}
	result := e.Search(events, "контракт Ивану", 10)
	if len(result) != 1 || result[0].ID != 1 {
		t.Fatal("description search or synonym failed")
	}
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				if len(e.Search(events, "контракт Иваном", 10)) != 1 {
					t.Error("concurrent search changed the result")
				}
				e.Tokens("неизвестными морфологическими словоформами")
			}
		}()
	}
	wg.Wait()
}
