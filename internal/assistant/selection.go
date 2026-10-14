package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

const maxMutationPreview = 100

func selectionScope(events []domain.Event, cmd domain.Command) []domain.Event {
	result := []domain.Event{}
	for _, e := range events {
		if cmd.Kind != domain.EventUnknown && e.Kind != cmd.Kind {
			continue
		}
		if cmd.Action == domain.ActionComplete && e.Status != domain.StatusOpen {
			continue
		}
		if sel := cmd.Selector; sel != nil {
			if sel.Start != nil && (e.Date == nil || e.Date.Before(sel.Start.Time)) {
				continue
			}
			if sel.End != nil && (e.Date == nil || !e.Date.Before(sel.End.Time)) {
				continue
			}
			if sel.Time != nil && (e.StartTime == nil || e.StartTime.String() != sel.Time.String()) {
				continue
			}
		}
		result = append(result, e)
	}
	return result
}

func hasSelectionScope(cmd domain.Command) bool {
	s := cmd.Selector
	return cmd.Kind != domain.EventUnknown || s != nil && (s.All || s.Start != nil || s.End != nil || s.Time != nil)
}

func (s *Service) mutationCandidates(ctx context.Context, tx repository.Tx, state *domain.State, cmd domain.Command) ([]domain.Event, bool, error) {
	pool := []domain.Event{}
	if cmd.TargetID != nil {
		get := tx.Get
		if cmd.Action == domain.ActionRestore {
			get = tx.GetDeleted
		}
		e, err := get(ctx, *cmd.TargetID)
		if err != nil {
			return nil, false, err
		}
		if e != nil {
			pool = append(pool, *e)
		}
	} else if cmd.Query != "" || hasSelectionScope(cmd) {
		var err error
		f := domain.Filter{Kind: cmd.Kind, Deleted: cmd.Action == domain.ActionRestore}
		if cmd.Selector != nil {
			f.Start, f.End = cmd.Selector.Start, cmd.Selector.End
		}
		pool, err = tx.Events(ctx, f)
		if err != nil {
			return nil, false, err
		}
	} else {
		for _, id := range state.LastIDs {
			get := tx.Get
			if cmd.Action == domain.ActionRestore {
				get = tx.GetDeleted
			}
			e, err := get(ctx, id)
			if err != nil {
				return nil, false, err
			}
			if e != nil {
				pool = append(pool, *e)
			}
		}
	}
	pool = selectionScope(pool, cmd)
	if cmd.TargetID == nil && cmd.Query != "" {
		matches, certain := s.Calendar.Search.TitleMatches(pool, cmd.Query, cmd.Selector != nil && cmd.Selector.ExactTitle)
		return matches, certain, nil
	}
	return pool, true, nil
}

func askChoice(state *domain.State, cmd domain.Command, events []domain.Event, certain bool, now time.Time) domain.Response {
	intro := "Подходят несколько записей. Уточните название, дату или время:"
	if !certain {
		intro = "Нашёл похожие записи. Выберите нужную по названию, дате или кнопкой:"
	}
	extra := ""
	displayed := events
	if len(events) > 10 {
		extra = fmt.Sprintf("\nПоказаны 10 из %d. Уточните название или дату, чтобы сузить выбор.", len(events))
		displayed = events[:10]
	}
	result := ask(state, cmd, intro+"\n"+joinEvents(displayed, 10, "\n")+extra, now, "choose", events)
	result.Candidates = displayed
	return result
}

// The snapshot is persisted with the question. Confirmation never re-runs a
// broad query, so records created later cannot silently join a deletion.
func askDeletion(state *domain.State, cmd domain.Command, events []domain.Event, now time.Time) domain.Response {
	if len(events) == 1 {
		id := events[0].ID
		cmd.TargetID = &id
		result := ask(state, cmd, "Удалить "+describe(events[0])+"? Ответьте «да» или «нет».", now, "delete", events)
		result.Candidates = []domain.Event{}
		return result
	}
	text := fmt.Sprintf("Найдено записей для удаления: %d.\n", len(events)) + joinEvents(events, 8, "\n")
	if len(events) > 8 {
		text += "\nПолный список — в карточках ниже."
	}
	text += "\nУдалить все эти записи? Ответьте «да» или «нет». Будут удалены только записи из этого списка."
	result := ask(state, cmd, text, now, "delete_many", events)
	result.Candidates = []domain.Event{}
	result.Events = events
	total := len(events)
	result.Total = &total
	return result
}

func sameRecord(a, b domain.Event) bool {
	a.Score, b.Score = nil, nil
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

func (s *Service) confirmDeletion(ctx context.Context, tx repository.Tx, state *domain.State, p *domain.Pending, now time.Time) (domain.Response, error) {
	state.Pending = nil
	if len(p.Candidates) == 0 {
		// Pending questions created by older versions have no durable snapshot.
		if p.Command.TargetID != nil {
			e, err := tx.Get(ctx, *p.Command.TargetID)
			if err != nil {
				return domain.Response{}, err
			}
			if e != nil {
				return askDeletion(state, p.Command, []domain.Event{*e}, now), nil
			}
		}
		return answer("Запись уже удалена. Ничего не изменено.", domain.ActionClarify), nil
	}
	for _, snapshot := range p.Candidates {
		e, err := tx.Get(ctx, snapshot.ID)
		if err != nil {
			return domain.Response{}, err
		}
		if e == nil || !sameRecord(snapshot, *e) {
			return answer("После уточнения записи изменились или были удалены. Ничего не удалено. Повторите запрос, чтобы проверить актуальный список.", domain.ActionClarify), nil
		}
	}
	for _, snapshot := range p.Candidates {
		if err := tx.Delete(ctx, snapshot.ID); err != nil {
			return domain.Response{}, err
		}
	}
	state.LastIDs = []int64{}
	if len(p.Candidates) == 1 {
		return answer("Удалено: "+describe(p.Candidates[0])+".", domain.ActionDelete), nil
	}
	result := answer(fmt.Sprintf("Удалено записей: %d.", len(p.Candidates)), domain.ActionDelete)
	total := len(p.Candidates)
	result.Total = &total
	return result, nil
}

func ordinal(text string, count int) int {
	text = strings.TrimSpace(strings.TrimSuffix(text, " пожалуйста"))
	for i, forms := range []string{"первую первый первая первое", "вторую второй вторая второе", "третью третий третья третье", "четвертую четвертый четвертая четвертое", "пятую пятый пятая пятое"} {
		for _, form := range strings.Fields(forms) {
			if text == form || text == form+" запись" {
				return i
			}
		}
	}
	if in(text, "последнюю", "последний", "последняя", "последнее") {
		return count - 1
	}
	return -1
}

func (s *Service) choosePending(ctx context.Context, tx repository.Tx, state *domain.State, message, low string, cmd domain.Command, now time.Time) (*domain.Event, *domain.Response, error) {
	p := state.Pending
	selected := []domain.Event{}
	if digits.MatchString(low) || cmd.TargetID != nil {
		id := int64(0)
		if cmd.TargetID != nil {
			id = *cmd.TargetID
		} else {
			id, _ = strconv.ParseInt(strings.TrimPrefix(low, "#"), 10, 64)
		}
		for _, e := range p.Candidates {
			if e.ID == id {
				selected = append(selected, e)
			}
		}
	} else if index := ordinal(low, min(len(p.Candidates), 10)); index >= 0 && index < len(p.Candidates) {
		selected = append(selected, p.Candidates[index])
	} else if isAffirmative(low) && len(p.Candidates) == 1 {
		selected = append(selected, p.Candidates[0])
	} else {
		reply := nlp.SelectionReply(message, now)
		if reply.Error != "" {
			result := answer(reply.Error, domain.ActionClarify)
			result.Candidates = p.Candidates[:min(len(p.Candidates), 10)]
			return nil, &result, nil
		}
		selected = selectionScope(p.Candidates, reply)
		if reply.Query != "" {
			matches, certain := s.Calendar.Search.TitleMatches(selected, reply.Query, reply.Selector.ExactTitle)
			if !certain {
				selected = nil
			} else {
				selected = matches
			}
		}
	}
	if len(selected) != 1 {
		if len(selected) > 1 {
			result := askChoice(state, p.Command, selected, true, now)
			return nil, &result, nil
		}
		result := answer("Не удалось однозначно выбрать запись из списка. Уточните название, дату или время либо нажмите на нужную запись.", domain.ActionClarify)
		result.Candidates = p.Candidates[:min(len(p.Candidates), 10)]
		return nil, &result, nil
	}
	get := tx.Get
	if p.Command.Action == domain.ActionRestore {
		get = tx.GetDeleted
	}
	e, err := get(ctx, selected[0].ID)
	if err != nil {
		return nil, nil, err
	}
	if e == nil || !sameRecord(selected[0], *e) {
		state.Pending = nil
		result := answer("Выбранная запись изменилась или была удалена. Ничего не изменено. Повторите запрос с её названием и датой.", domain.ActionClarify)
		return nil, &result, nil
	}
	return e, nil, nil
}
