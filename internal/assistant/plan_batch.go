package assistant

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

func applyExecutionChanges(e *domain.Event, ch *domain.ExecutionChanges, loc *time.Location) error {
	if ch == nil {
		return nil
	}
	if ch.Status != nil {
		if *ch.Status != domain.StatusOpen && *ch.Status != domain.StatusDone {
			return errors.New("Некорректный статус.")
		}
		e.Status = *ch.Status
	}
	if ch.Priority != nil {
		if *ch.Priority < domain.PriorityNormal || *ch.Priority > domain.PriorityHigh {
			return errors.New("Некорректный приоритет.")
		}
		e.Priority = *ch.Priority
	}
	if ch.TimeShiftMinutes != nil {
		if *ch.TimeShiftMinutes == 0 || *ch.TimeShiftMinutes < -525600 || *ch.TimeShiftMinutes > 525600 {
			return errors.New("Некорректный сдвиг времени.")
		}
		if e.Recurrence != nil {
			return errors.New("Уточните отдельное новое время для повторяющейся записи.")
		}
		start, _, ok := e.Interval(loc)
		if !ok {
			return errors.New("Для сдвига у записи должны быть дата и время.")
		}
		at := start.Add(time.Duration(*ch.TimeShiftMinutes) * time.Minute)
		e.Date = domain.Day(at)
		e.StartTime = &domain.TimeOfDay{Hour: at.Hour(), Minute: at.Minute()}
		e.StartSecond = at.Second()
		e.OccurrenceAt = &at
	}
	return nil
}

func (s *Service) proposedBatchEvent(e domain.Event, c domain.Command, now time.Time) (domain.Event, error) {
	if c.Action == domain.ActionDelete {
		return e, nil
	}
	if c.Action == domain.ActionComplete {
		e.Status = domain.StatusDone
		return e, e.Validate()
	}
	if c.Action != domain.ActionUpdate {
		return e, errors.New("Некорректная массовая операция.")
	}
	if c.Title != "" {
		e.Title = c.Title
	}
	if c.ReminderEnabled != nil {
		e.ReminderEnabled = c.ReminderEnabled
	}
	if c.ReminderMinutes != nil {
		e.ReminderMinutes = c.ReminderMinutes
	}
	if c.Date != nil || c.Time != nil {
		e.OccurrenceAt = c.ExactAt
	}
	if c.Date != nil {
		e.Date = c.Date
		if err := calendar.MoveSeriesDate(&e); err != nil {
			return e, err
		}
	}
	if c.Time != nil {
		e.StartTime = c.Time
		e.StartSecond = c.StartSecond
	}
	if err := applyExecutionChanges(&e, c.Changes, s.Calendar.Location); err != nil {
		return e, err
	}
	if e.Recurrence != nil {
		if err := calendar.Rebase(&e, now, s.Calendar.Location); err != nil {
			return e, err
		}
	}
	return e, e.Validate()
}

// Batch conflicts are computed against the proposed final set. Original times
// of selected records are excluded, and internal overlaps are also previewed.
func (s *Service) batchConflicts(ctx context.Context, tx repository.Tx, before, after []domain.Event) ([]domain.Event, error) {
	selected := map[int64]bool{}
	conflicts := map[int64]domain.Event{}
	for _, e := range before {
		selected[e.ID] = true
	}
	for i, e := range after {
		if e.Status != domain.StatusOpen {
			continue
		}
		items, err := s.Calendar.Conflicts(ctx, tx, e, e.ID)
		if err != nil {
			return nil, err
		}
		for _, x := range items {
			if !selected[x.ID] {
				conflicts[x.ID] = x
			}
		}
		if e.Kind != domain.EventMeeting {
			continue
		}
		es, ee, ok := e.Interval(s.Calendar.Location)
		if !ok {
			continue
		}
		for _, x := range after[:i] {
			if x.Kind != domain.EventMeeting || x.Status != domain.StatusOpen {
				continue
			}
			xs, xe, ok := x.Interval(s.Calendar.Location)
			if ok && es.Before(xe) && xs.Before(ee) {
				conflicts[x.ID] = x
				conflicts[e.ID] = e
			}
		}
	}
	result := []domain.Event{}
	for _, e := range conflicts {
		result = append(result, e)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (s *Service) previewBatch(ctx context.Context, tx repository.Tx, state *domain.State, c domain.Command, now time.Time) (domain.Response, error) {
	if err := c.Validate(); err != nil {
		return answer("План не прошёл проверку. Ничего не изменено.", domain.ActionClarify), nil
	}
	items, certain, err := s.mutationCandidates(ctx, tx, state, c)
	if err != nil {
		return domain.Response{}, err
	}
	if len(items) == 0 {
		return answer("Подходящих записей нет. Ничего не изменено.", domain.ActionClarify), nil
	}
	if !certain {
		return answer("Не удалось однозначно определить группу. Уточните тему или фильтры.", domain.ActionClarify), nil
	}
	if len(items) > maxMutationPreview {
		return answer(fmt.Sprintf("Найдено %d записей. Сузьте запрос до %d записей.", len(items), maxMutationPreview), domain.ActionClarify), nil
	}
	after := []domain.Event{}
	if c.Action != domain.ActionDelete {
		for _, e := range items {
			x, err := s.proposedBatchEvent(e, c, now)
			if err != nil {
				return answer(err.Error()+" Ничего не изменено.", domain.ActionClarify), nil
			}
			after = append(after, x)
		}
	}
	conflicts, err := s.batchConflicts(ctx, tx, items, after)
	if err != nil {
		return domain.Response{}, err
	}
	text := fmt.Sprintf("Предпросмотр: записей %d. Действие: %s.\n", len(items), c.Action.String())
	if c.Action == domain.ActionUpdate {
		text += "Изменения: "
		if c.Title != "" {
			text += "название «" + c.Title + "»; "
		}
		if c.Date != nil {
			text += "дата " + c.Date.String() + "; "
		}
		if c.Time != nil {
			text += "время " + c.Time.String() + "; "
		}
		if c.Changes != nil {
			if c.Changes.TimeShiftMinutes != nil {
				text += fmt.Sprintf("сдвиг %+d мин; ", *c.Changes.TimeShiftMinutes)
			}
			if c.Changes.Status != nil {
				text += "статус " + c.Changes.Status.String() + "; "
			}
			if c.Changes.Priority != nil {
				text += "приоритет " + c.Changes.Priority.String() + "; "
			}
		}
		if c.ReminderEnabled != nil {
			if *c.ReminderEnabled {
				text += "уведомления включены; "
			} else {
				text += "уведомления выключены; "
			}
		}
		if c.ReminderMinutes != nil {
			text += fmt.Sprintf("напомнить за %d мин; ", *c.ReminderMinutes)
		}
		text += "\n"
	}
	for i, e := range items {
		if i >= 8 {
			break
		}
		text += describe(e)
		if len(after) > 0 {
			text += " → " + describe(after[i])
			if after[i].Status != e.Status {
				text += "; статус: " + after[i].Status.String()
			}
			if after[i].Priority != e.Priority {
				text += "; приоритет: " + after[i].Priority.String()
			}
		}
		text += "\n"
	}
	if len(items) > 8 {
		text += "Полный список — в карточках.\n"
	}
	if len(conflicts) > 0 {
		text += "Есть пересечения: " + joinEvents(conflicts, 8, "; ") + ". Подтверждение также разрешит эти пересечения.\n"
	}
	text += "Применить ко всем этим записям? Ответьте «да» или «нет»."
	r := ask(state, c, text, now, "batch", items)
	state.Pending.BatchAfter, state.Pending.BatchConflicts = after, conflicts
	r.Events = items
	r.Candidates = []domain.Event{}
	r.Conflicts = conflicts
	total := len(items)
	r.Total = &total
	return r, nil
}

func (s *Service) confirmBatch(ctx context.Context, tx repository.Tx, state *domain.State, now time.Time) (domain.Response, error) {
	p := state.Pending
	state.Pending = nil
	refuse := func() (domain.Response, error) {
		return answer("Список или записи изменились. Повторите запрос для нового предпросмотра. Ничего не изменено.", domain.ActionClarify), nil
	}
	if p == nil || p.Expectation != "batch" || !p.Command.Batch || len(p.Candidates) == 0 || len(p.Candidates) > maxMutationPreview {
		return refuse()
	}
	if err := p.Command.Validate(); err != nil {
		return refuse()
	}
	if p.Command.Action != domain.ActionDelete && len(p.BatchAfter) != len(p.Candidates) {
		return refuse()
	}
	for _, old := range p.Candidates {
		e, err := tx.Get(ctx, old.ID)
		if err != nil {
			return domain.Response{}, err
		}
		if e == nil || !sameRecord(old, *e) {
			return refuse()
		}
	}
	for i, after := range p.BatchAfter {
		if after.ID != p.Candidates[i].ID {
			return refuse()
		}
		if err := after.Validate(); err != nil {
			return refuse()
		}
	}
	conflicts, err := s.batchConflicts(ctx, tx, p.Candidates, p.BatchAfter)
	if err != nil {
		return domain.Response{}, err
	}
	if len(conflicts) != len(p.BatchConflicts) {
		return refuse()
	}
	for i, e := range conflicts {
		if !sameRecord(e, p.BatchConflicts[i]) {
			return refuse()
		}
	}
	result := []domain.Event{}
	state.LastIDs = nil
	for i, e := range p.Candidates {
		if p.Command.Action == domain.ActionDelete {
			if err := tx.Delete(ctx, e.ID); err != nil {
				return domain.Response{}, err
			}
		} else {
			x, err := tx.Update(ctx, p.BatchAfter[i], now)
			if err != nil {
				return domain.Response{}, err
			}
			result = append(result, *x)
			state.LastIDs = append(state.LastIDs, x.ID)
		}
	}
	total := len(p.Candidates)
	r := answer(fmt.Sprintf("Применено к %d записям.", total), p.Command.Action, result...)
	r.Total = &total
	return r, nil
}
