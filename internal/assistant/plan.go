package assistant

import (
	"context"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

func (s *Service) resolvePlan(ctx context.Context, tx repository.Tx, state *domain.State, p nlp.SemanticPlan, message string, now time.Time) (domain.Command, *domain.Response, error) {
	p, _ = nlp.CanonicalizePlanSource(p, message)
	c := neutral("semantic_llm")
	clarify := func(text string) (domain.Command, *domain.Response, error) {
		r := answer(text, domain.ActionClarify)
		return c, &r, nil
	}
	if err := p.Validate(); err != nil {
		return clarify("Не удалось проверить план. Уточните запрос. Ничего не изменено.")
	}
	if err := nlp.ValidatePlanEvidence(p, message); err != nil {
		state.Pending = nil
		return clarify("Не удалось подтвердить смысл плана исходной фразой. Уточните запрос. Ничего не изменено.")
	}
	if p.Status != "ok" {
		q := "Уточните действие и нужную запись."
		if p.Clarification != nil {
			q = *p.Clarification
		}
		return clarify(q)
	}
	if p.Target != nil && p.Target.Ordinal != nil {
		count := len(state.LastIDs)
		if p.DialogueAct == "choice" && state.Pending != nil {
			count = len(state.Pending.Candidates)
		}
		if !nlp.HasOrdinalEvidence(message, *p.Target.Ordinal, count) {
			return clarify("Нельзя однозначно выбрать запись. Укажите порядковый номер словами или выберите запись из списка.")
		}
	}
	if *p.Operation == "chat" {
		r := answer(*p.ChatResponse, domain.ActionHelp)
		return c, &r, nil
	}
	if p.DialogueAct == "slot_reply" {
		pending := state.Pending
		if pending == nil || !in(pending.Expectation, "fields", "reminder", "conflict") {
			return clarify("Сейчас нет подходящего уточнения. Назовите действие и запись.")
		}
		ch := p.Changes
		if ch.Status != nil || ch.Priority != nil || ch.TimeShiftMinutes != nil {
			return clarify("Ответьте на ожидаемое уточнение.")
		}
		if pending.Expectation == "reminder" && (ch.Title != nil || ch.Date != nil || ch.Time != nil || ch.Delay != nil) {
			return clarify("За сколько напомнить?")
		}
		if pending.Expectation != "reminder" && ch.Title != nil && pending.Command.Title != "" {
			return clarify("Уточните дату или время; для переименования задайте новую команду.")
		}
		c = pending.Command
		if err := nlp.ApplyNormalizedChanges(&c, *ch, now); err != nil {
			return clarify("Уточните дату или время. Ничего не изменено.")
		}
		c.Error = ""
		state.Pending = nil
		return c, nil, nil
	}
	if p.DialogueAct == "choice" {
		if state.Pending == nil || state.Pending.Expectation != "choose" {
			return clarify("Сейчас нет списка для выбора.")
		}
		index := planReferenceIndex(*p.Target, len(state.Pending.Candidates))
		if index < 0 || index >= min(len(state.Pending.Candidates), 8) {
			return clarify("Выберите одну запись из показанного списка.")
		}
		snapshot := state.Pending.Candidates[index]
		e, err := tx.Get(ctx, snapshot.ID)
		if err != nil {
			return c, nil, err
		}
		if e == nil || !sameRecord(snapshot, *e) {
			state.Pending = nil
			return clarify("Выбранная запись изменилась. Повторите запрос.")
		}
		c = state.Pending.Command
		c.TargetID = &e.ID
		c.Query = ""
		c.Selector = nil
		c.Filter = nil
		state.Pending = nil
		return c, nil, nil
	}
	// A read-only followup may select a displayed result without inventing a
	// query or converting the user's selection into an update.
	if *p.Operation == "search" && p.Target != nil {
		ids, fresh := planResultIDs(*state, *p.Target, now)
		if !fresh {
			return clarify("Сначала найдите записи или назовите нужную запись.")
		}
		index := planReferenceIndex(*p.Target, len(ids))
		if index < 0 || index >= min(len(ids), 8) {
			return clarify("Уточните номер записи из показанного списка.")
		}
		e, err := tx.Get(ctx, ids[index])
		if err != nil {
			return c, nil, err
		}
		if e == nil {
			return clarify("Запись уже недоступна. Повторите поиск.")
		}
		if p.Kind != nil && *p.Kind != "any" && *p.Kind != e.Kind.String() {
			return clarify("Тип выбранной записи не совпадает.")
		}
		c.Action, c.Kind = domain.ActionSearch, e.Kind
		c.Query, c.Filter = state.Dialogue.Query, state.Dialogue.Filter
		c.PeriodStart, c.PeriodEnd = state.Dialogue.PeriodStart, state.Dialogue.PeriodEnd
		state.Pending, state.LastIDs = nil, []int64{e.ID}
		r := answer(describe(*e), domain.ActionSearch, *e)
		return c, &r, nil
	}
	var err error
	c, err = nlp.MaterializePlan(p, now)
	if err != nil {
		return clarify("Уточните дату, время или фильтры. Ничего не изменено.")
	}
	if p.DialogueAct == "correction" {
		if !freshAffected(*state, now) || len(state.Dialogue.AffectedIDs) != 1 || !in(state.Dialogue.LastAction.String(), "create", "update", "complete") || state.Pending != nil {
			return clarify("Уточните запись для исправления: нет единственной недавно изменённой записи.")
		}
		id := state.Dialogue.AffectedIDs[0]
		e, err := tx.Get(ctx, id)
		if err != nil {
			return c, nil, err
		}
		if e == nil || c.Kind != domain.EventUnknown && c.Kind != e.Kind {
			return clarify("Уточните запись для исправления.")
		}
		c.TargetID, c.Kind = &id, e.Kind
	}
	if p.DialogueAct == "refinement" {
		if !freshResult(*state, now) || state.Dialogue.Search == nil {
			return clarify("Уточните тему поиска: предыдущий поиск отсутствует или устарел.")
		}
		d := state.Dialogue.Search
		base := domain.ExecutionFilter{Query: d.Query, Kind: d.Kind, Start: d.Start, End: d.End}
		if d.Filter != nil {
			base = *d.Filter
		}
		if p.Kind != nil {
			base.Kind = c.Kind
		}
		if p.Query != nil {
			base.Query = c.Query
		}
		if f := p.Filters; f != nil {
			if f.Status != nil {
				base.Status = c.Filter.Status
			}
			if f.Priority != nil {
				base.Priority = c.Filter.Priority
			}
			if f.Period != nil {
				base.Start, base.End = c.Filter.Start, c.Filter.End
			}
			if f.TimeFrom != nil {
				base.TimeFrom = c.Filter.TimeFrom
			}
			if f.TimeBefore != nil {
				base.TimeBefore = c.Filter.TimeBefore
			}
			if len(f.Exclude) > 0 {
				base.Exclude = append(append([]domain.ExecutionFilter{}, base.Exclude...), c.Filter.Exclude...)
			}
		}
		c.Action = d.Action
		if p.Query != nil {
			c.Action = domain.ActionSearch
		}
		c.Query, c.Kind, c.PeriodStart, c.PeriodEnd, c.Filter = base.Query, base.Kind, base.Start, base.End, &base
	}
	if p.Target != nil && p.Target.Query == nil {
		ids, fresh := planResultIDs(*state, *p.Target, now)
		if !fresh {
			return clarify("Уточните, какую запись вы имеете в виду.")
		}
		index := planReferenceIndex(*p.Target, len(ids))
		if index < 0 || index >= min(len(ids), 8) {
			return clarify("Нельзя однозначно выбрать запись. Укажите название или номер из списка.")
		}
		id := ids[index]
		e, err := tx.Get(ctx, id)
		if err != nil {
			return c, nil, err
		}
		if e == nil {
			return clarify("Запись уже недоступна. Повторите поиск.")
		}
		if c.Kind != domain.EventUnknown && c.Kind != e.Kind {
			return clarify("Тип выбранной записи не совпадает.")
		}
		c.Kind = e.Kind
		c.TargetID = &id
	}
	state.Pending = nil
	return c, nil, nil
}

func planResultIDs(state domain.State, target nlp.PlanTarget, now time.Time) ([]int64, bool) {
	if target.Reference != nil && *target.Reference == "last" && freshAffected(state, now) && in(state.Dialogue.LastAction.String(), "create", "update", "complete") {
		return state.Dialogue.AffectedIDs, true
	}
	return state.LastIDs, freshResult(state, now)
}

func planReferenceIndex(t nlp.PlanTarget, count int) int {
	if t.Reference == nil {
		return -1
	}
	if *t.Reference == "last" && count == 1 {
		return 0
	}
	if *t.Reference == "ordinal" && t.Ordinal != nil {
		return *t.Ordinal - 1
	}
	return -1
}

func (s *Service) filterEvents(events []domain.Event, f *domain.ExecutionFilter) []domain.Event {
	if f == nil {
		return events
	}
	result := []domain.Event{}
	for _, e := range events {
		if s.eventMatchesFilter(e, *f) {
			result = append(result, e)
		}
	}
	return result
}
func (s *Service) eventMatchesFilter(e domain.Event, f domain.ExecutionFilter) bool {
	if f.Kind != domain.EventUnknown && e.Kind != f.Kind || f.Start != nil && (e.Date == nil || e.Date.Before(f.Start.Time)) || f.End != nil && (e.Date == nil || !e.Date.Before(f.End.Time)) {
		return false
	}
	if f.Status != nil && *f.Status != domain.StatusUnknown && e.Status != *f.Status || f.Priority != nil && e.Priority != *f.Priority {
		return false
	}
	clock := func(t *domain.TimeOfDay) int { return t.Hour*60 + t.Minute }
	if f.TimeFrom != nil && (e.StartTime == nil || clock(e.StartTime) < clock(f.TimeFrom)) || f.TimeBefore != nil && (e.StartTime == nil || clock(e.StartTime) >= clock(f.TimeBefore)) {
		return false
	}
	if f.Query != "" {
		matches, certain := s.Calendar.Search.TitleMatches([]domain.Event{e}, f.Query, false)
		if !certain || len(matches) != 1 {
			return false
		}
	}
	for _, x := range f.Exclude {
		if s.eventMatchesFilter(e, x) {
			return false
		}
	}
	return true
}
