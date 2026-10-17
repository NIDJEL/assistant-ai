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

func TestTopicInflectionsAcrossKinds(t *testing.T) {
	e, err := search.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"Аудит контура", "аудиты"}, {"Замер контура", "замеры"}, {"Ревизия контура", "ревизии"}, {"Осмотр контура", "осмотры"}} {
		items := []domain.Event{{ID: 1, Title: pair[0], Kind: domain.EventTask}, {ID: 2, Title: pair[0], Kind: domain.EventMeeting}}
		if got := e.Search(items, pair[1], 10); len(got) != 2 {
			t.Errorf("%s / %s: title tokens %v query tokens %v, got %v", pair[0], pair[1], e.Tokens(pair[0]), e.Tokens(pair[1]), got)
		}
	}
}
