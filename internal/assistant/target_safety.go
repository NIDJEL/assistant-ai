package assistant

import (
	"context"
	"errors"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

// This guard only refuses a contextual mutation. It never selects a record
// from natural language: a named target must be resolved by a query operation.
func (s *Service) guardContextTarget(ctx context.Context, tx repository.Tx, c domain.Command, message string) error {
	if c.TargetID == nil {
		return nil
	}
	selected, err := tx.Get(ctx, *c.TargetID)
	if err != nil || selected == nil {
		return err
	}
	engine := s.Calendar.Search
	ignored := engine.Tokens("задача встреча напоминание запись дело сделать отправить согласовать купить подготовить удалить завершить отметить выполнить перенести вернуть добавить создать поставить")
	mentions := engine.Tokens(message)
	topics := func(e domain.Event) map[string]bool {
		out := engine.Tokens(e.Title)
		for token := range out {
			if ignored[token] {
				delete(out, token)
			}
		}
		return out
	}
	current := topics(*selected)
	for token := range current {
		if mentions[token] {
			return nil
		}
	}
	events, err := tx.Events(ctx, domain.Filter{})
	if err != nil {
		return err
	}
	for _, e := range events {
		if e.ID == selected.ID {
			continue
		}
		for token := range topics(e) {
			if mentions[token] {
				return errors.New("В запросе названа другая запись. Уточните её название; предыдущая запись не изменена.")
			}
		}
	}
	return nil
}
