package assistant

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

const referenceTTL = 15 * time.Minute
const pendingTTL = 30 * time.Minute

// DialogueObservation is local instrumentation, never part of the API/LLM DTO.
type DialogueObservation struct {
	Metrics                       domain.TurnMetrics
	EvidenceField, EvidenceReason string
	Route                         RouteDecision
	SemanticCalls                 int
	PlanValid                     bool
	Type                          string
	Reference                     string
	Command                       domain.Command
	Provider                      string
	InterpretationLatency         time.Duration
}

type contextualInterpreter interface {
	InterpretTurn(context.Context, string, nlp.DialogueContext, time.Time) (nlp.ContextualTurn, error)
}

func freshResult(state domain.State, now time.Time) bool {
	d := state.Dialogue
	return d != nil && !d.ResultAt.IsZero() && !now.Before(d.ResultAt) && now.Sub(d.ResultAt) <= referenceTTL && d.IdleTurns < 4
}

func freshAffected(state domain.State, now time.Time) bool {
	d := state.Dialogue
	return d != nil && !d.AffectedAt.IsZero() && !now.Before(d.AffectedAt) && now.Sub(d.AffectedAt) <= referenceTTL && d.IdleTurns < 4
}

func expireDialogue(state *domain.State, now time.Time) {
	if state.Actions != nil && (now.Before(state.Actions.At) || now.Sub(state.Actions.At) > pendingTTL) {
		state.Actions, state.Pending = nil, nil
	}
	if state.Pending != nil && (now.Before(state.Pending.At) || now.Sub(state.Pending.At) > pendingTTL) {
		state.Pending = nil
	}
	if !freshResult(*state, now) {
		state.LastIDs = nil
		if freshAffected(*state, now) {
			state.Dialogue.Search = nil
			state.Dialogue.Query, state.Dialogue.Filter, state.Dialogue.PeriodStart, state.Dialogue.PeriodEnd = "", nil, nil, nil
		} else {
			state.Dialogue = nil
		}
	}
	if state.Dialogue != nil && !freshAffected(*state, now) {
		state.Dialogue.AffectedIDs = nil
	}
}

func bounded(text string, max int) string {
	r := []rune(text)
	if len(r) > max {
		return string(r[:max])
	}
	return text
}

var internalID = regexp.MustCompile(`#\d+`)

func expectedSlot(p *domain.Pending) string {
	if p.Expectation != "fields" {
		return p.Expectation
	}
	if p.Command.Action == domain.ActionUpdate {
		return "date_or_time"
	}
	for _, missing := range nlp.MissingEntities(p.Command) {
		switch missing {
		case nlp.ActionText:
			return "title"
		case nlp.DateEntity:
			if p.Command.Kind == domain.EventReminder {
				return "relative_time_or_date"
			}
			return "date"
		case nlp.TimeEntity:
			return "time"
		case nlp.DayEntity:
			return "date"
		case nlp.RecurrenceDayEntity:
			return "weekday"
		}
	}
	return "date_or_time"
}

func displayEvent(e domain.Event) string {
	s := bounded(e.Title, 200)
	if e.Date != nil {
		s += ", " + e.Date.String()
	}
	if e.StartTime != nil {
		s += " " + e.StartTime.String()
	}
	return s
}

func buildDialogueContext(ctx context.Context, tx repository.Tx, state domain.State) (nlp.DialogueContext, error) {
	d := nlp.DialogueContext{}
	d.LastResultCount = len(state.LastIDs)
	if p := state.Pending; p != nil {
		d.Pending = &nlp.DialoguePending{Action: p.Command.Action.String(), Kind: p.Command.Kind.String(), Title: bounded(p.Command.Title, 500), ExpectedSlot: expectedSlot(p)}
		d.Pending.ConfirmationState = p.ConfirmationState
		if p.Command.Date != nil {
			d.Pending.Date = p.Command.Date.String()
		}
		if p.Command.Time != nil {
			d.Pending.Time = p.Command.Time.String()
		}
		for i, e := range p.Candidates {
			if i >= 8 {
				break
			}
			d.Candidates = append(d.Candidates, displayEvent(e))
		}
	}
	if v := state.Dialogue; v != nil {
		d.LastAffectedCount = len(v.AffectedIDs)
		d.Filters = v.Filter
		d.LastAction, d.LastKind, d.Query = v.LastAction.String(), v.LastKind.String(), bounded(v.Query, 300)
		if v.PeriodStart != nil {
			d.PeriodStart = v.PeriodStart.String()
		}
		if v.PeriodEnd != nil {
			d.PeriodEnd = v.PeriodEnd.String()
		}
		d.LastUserMessage = bounded(internalID.ReplaceAllString(v.LastUserMessage, "[запись]"), 500)
		d.LastAssistantMessage = bounded(internalID.ReplaceAllString(v.LastAssistantMessage, "[запись]"), 500)
	}
	if len(d.Candidates) == 0 {
		for i, id := range state.LastIDs {
			if i >= 8 {
				break
			}
			e, err := tx.Get(ctx, id)
			if err != nil {
				return d, err
			}
			if e != nil {
				d.Candidates = append(d.Candidates, displayEvent(*e))
			} else {
				d.Candidates = append(d.Candidates, "Запись больше недоступна")
			}
		}
	}
	return d, nil
}

func neutral(provider string) domain.Command {
	return domain.Command{Action: domain.ActionHelp, Duration: time.Hour, Provider: provider}
}
func pointer(s string) *string { return &s }

func chatReply(message string) string {
	switch strings.Trim(strings.ToLower(message), " ,.!?") {
	case "привет", "здравствуй", "здравствуйте", "привет гена":
		return "Привет! Я Гена. Что запланируем?"
	case "спасибо", "благодарю":
		return "Пожалуйста!"
	case "как тебя зовут", "кто ты":
		return "Я Гена, твой помощник с задачами, встречами и напоминаниями."
	case "что ты умеешь", "помощь":
		return help
	}
	return ""
}

func fastDialogue(state domain.State, message string, now time.Time) *nlp.ContextualTurn {
	low := strings.Trim(strings.ReplaceAll(strings.ToLower(message), "ё", "е"), " ,.!?")
	t := nlp.ContextualTurn{Status: "ok"}
	if reply := chatReply(message); reply != "" {
		t.Type, t.ChatResponse = "chat", &reply
		return &t
	}
	p := state.Pending
	if p != nil && p.Expectation == "reminder" {
		if _, _, ok, question := nlp.ReminderChoice(message); ok && question == "" && !nlp.HasDirective(message) {
			t.Type, t.Slots = "slot_reply", &nlp.SlotUpdate{ReminderExpression: &message}
			return &t
		}
	}
	consent := nlp.ClassifyConfirmation(message)
	if consent == nlp.ConfirmationNo {
		t.Type = "cancellation"
		return &t
	}
	if consent == nlp.ConfirmationYes && (p == nil || !in(p.Expectation, "delete", "delete_many", "conflict")) {
		t.Type, t.ChatResponse = "chat", pointer("Сейчас нет действия для подтверждения. Сформулируйте запрос или ответьте на уточнение.")
		return &t
	}
	if p == nil {
		if freshResult(state, now) && in(state.Dialogue.LastAction.String(), "list", "search") {
			if unsupportedClockFilter.MatchString(message) {
				return &nlp.ContextualTurn{Status: "ambiguous", Type: "refinement", Clarification: pointer("Фильтр по часам пока не поддерживается. Укажите период или тип записей.")}
			}
			if slots := nlp.ExtractFilterRefinement(message, now); slots != nil {
				t.Type, t.Slots = "refinement", slots
				return &t
			}
		}
		return nil
	}
	if in(p.Expectation, "delete", "delete_many", "conflict") && consent == nlp.ConfirmationYes {
		yes := true
		t.Type, t.Confirmation = "confirmation", &yes
		return &t
	}
	if p.Expectation == "choose" {
		if i := ordinal(low, min(len(p.Candidates), 8)); i >= 0 && i < min(len(p.Candidates), 5) {
			t.Type, t.Reference = "choice", &nlp.DialogueReference{Relation: []string{"first", "second", "third", "fourth", "fifth"}[i]}
			return &t
		}
	}
	if p.Expectation != "fields" && p.Expectation != "conflict" || nlp.HasDirective(message) {
		return nil
	}
	if expectedSlot(p) == "title" {
		return nil
	}
	raw, ok := nlp.ExtractExpectedTemporalSlot(message, expectedSlot(p))
	if !ok {
		return nil
	}
	u := &nlp.SlotUpdate{TemporalExpression: &raw}
	if _, err := nlp.ApplyDialogueSlots(p.Command, *u, message, now); err == nil {
		t.Type, t.Slots = "slot_reply", u
		return &t
	}
	return nil
}

var contextualCue = regexp.MustCompile(`(?i)(?:^|[^\p{L}])(?:е[её]|его|эту|этот|ту|первую|вторую|третью|следующую|лучше|имел в виду|только)(?:$|[^\p{L}])|^(?:а |и |на |в |тогда |нет[, ]|не[, ])`)
var correctionCue = regexp.MustCompile(`(?i)^(?:нет[, ]|не[, ]|а |и |лучше |тогда |на |в )`)
var orphanReference = regexp.MustCompile(`(?i)^(?:а\s+)?(?:удали|перенеси|сдвинь|заверши|отметь)\s+(?:е[её]|эту|его|этот)(?:$|[^\p{L}])`)
var orphanRefinement = regexp.MustCompile(`(?i)^(?:а\s+)?(?:только|лишь|отфильтруй|ограничь)(?:$|[^\p{L}])`)
var unsupportedClockFilter = regexp.MustCompile(`(?i)^\s*(?:(?:а|ну|тогда)\s+)*(?:только\s+)?(?:после|до)\s+\d{1,2}(?::\d{2})?(?:\s+часов)?[?.! ]*$`)

func needsContext(state domain.State, message string) bool {
	if nlp.IsTimerRequest(message) {
		return true
	}
	if state.Pending != nil && !nlp.HasDirective(message) {
		return true
	}
	return state.Dialogue != nil && !state.Dialogue.ResultAt.IsZero() && contextualCue.MatchString(message)
}

func (s *Service) interpretDialogue(ctx context.Context, state domain.State, d nlp.DialogueContext, message string, now time.Time) (domain.Command, *nlp.ContextualTurn, error) {
	if turn := fastDialogue(state, message, now); turn != nil {
		return neutral("rules"), turn, nil
	}
	if state.Pending == nil && (!freshResult(state, now) || !in(state.Dialogue.LastAction.String(), "list", "search")) && orphanRefinement.MatchString(message) {
		return neutral("rules"), &nlp.ContextualTurn{Status: "ambiguous", Type: "refinement", Clarification: pointer("Уточните тему и тип записей для поиска: предыдущий поиск не был успешно выполнен или его контекст устарел.")}, nil
	}
	if state.Pending == nil && (state.Dialogue == nil || state.Dialogue.ResultAt.IsZero()) && orphanReference.MatchString(message) {
		return neutral("rules"), &nlp.ContextualTurn{Status: "ambiguous", Type: "reference", Clarification: pointer("Какую запись вы имеете в виду? Укажите название или выберите её из списка.")}, nil
	}
	if needsContext(state, message) {
		if parser, ok := s.Interpreter.(contextualInterpreter); ok {
			t, err := parser.InterpretTurn(ctx, message, d, now)
			if ctx.Err() != nil {
				return domain.Command{}, nil, ctx.Err()
			}
			if err == nil {
				return neutral("contextual_semantic"), &t, nil
			}
			c := neutral("contextual_unavailable")
			c.Error = "Не удалось безопасно понять продолжение. Уточните название, дату или время. Ничего не изменено."
			return c, nil, nil
		}
	}
	c, err := s.Interpreter.Interpret(nlp.WithPendingExpectation(ctx, pendingExpectation(state)), message, now)
	return c, nil, err
}

func pendingExpectation(state domain.State) string {
	if state.Pending != nil {
		return state.Pending.Expectation
	}
	return ""
}

func (s *Service) resolveReference(ctx context.Context, tx repository.Tx, state *domain.State, ref nlp.DialogueReference, pending bool, now time.Time) (*domain.Event, error) {
	ids := state.LastIDs
	if pending {
		ids = nil
		for i, e := range state.Pending.Candidates {
			if i >= 8 {
				break
			}
			ids = append(ids, e.ID)
		}
	} else if !freshResult(*state, now) {
		return nil, nil
	}
	index := -1
	switch ref.Relation {
	case "last":
		if len(ids) == 1 {
			index = 0
		}
	case "first":
		index = 0
	case "second":
		index = 1
	case "third":
		index = 2
	case "fourth":
		index = 3
	case "fifth":
		index = 4
	case "previous":
		return nil, nil // No invisible previous target is guessed.
	case "matching":
		pool := []domain.Event{}
		for i, id := range ids {
			if i >= 8 {
				break
			}
			e, err := tx.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			if e != nil {
				pool = append(pool, *e)
			}
		}
		matches, certain := s.Calendar.Search.TitleMatches(pool, *ref.Query, false)
		if certain && len(matches) == 1 {
			return &matches[0], nil
		}
		return nil, nil
	}
	if index < 0 || index >= min(len(ids), 8) {
		return nil, nil
	}
	return tx.Get(ctx, ids[index])
}

// Resolution runs inside the existing user-scoped transaction. Models cannot
// pick IDs, extend confirmations or execute around the domain executor.
func (s *Service) resolveTurn(ctx context.Context, tx repository.Tx, state *domain.State, t nlp.ContextualTurn, message string, now time.Time) (domain.Command, *domain.Response, error) {
	c := neutral("dialogue")
	clarify := func(text string) (domain.Command, *domain.Response, error) {
		r := answer(text, domain.ActionClarify)
		return c, &r, nil
	}
	if err := t.Validate(); err != nil {
		return clarify("Уточните продолжение. Ничего не изменено.")
	}
	if t.Status != "ok" {
		q := "Уточните действие и нужную запись."
		if t.Clarification != nil {
			q = *t.Clarification
		}
		return clarify(q)
	}
	switch t.Type {
	case "chat":
		reply := help
		if t.ChatResponse != nil {
			reply = *t.ChatResponse
		}
		r := answer(reply, domain.ActionHelp)
		return c, &r, nil
	case "cancellation":
		state.Pending = nil
		state.LastIDs, state.Dialogue = nil, nil
		r := answer("Действие отменено.", domain.ActionCancel)
		return c, &r, nil
	case "confirmation":
		consent := nlp.ClassifyConfirmation(message)
		if state.Pending != nil && in(state.Pending.Expectation, "delete", "delete_many", "conflict") && consent == nlp.ConfirmationNo {
			state.Pending = nil
			r := answer("Действие отменено.", domain.ActionCancel)
			return c, &r, nil
		}
		if state.Pending == nil || !in(state.Pending.Expectation, "delete", "delete_many", "conflict", "batch") || !*t.Confirmation || consent != nlp.ConfirmationYes {
			return clarify("Ответьте «да» или «нет» на подтверждение.")
		}
		return state.Pending.Command, nil, nil // Existing executor verifies the persisted snapshot.
	case "command":
		cmd, err := nlp.MapSemantic(*t.Command, message, now)
		if err != nil {
			return clarify("Уточните новую команду. Ничего не изменено.")
		}
		state.Pending = nil
		return cmd, nil, nil
	case "slot_reply":
		p := state.Pending
		if p == nil || !in(p.Expectation, "fields", "reminder", "conflict") {
			return clarify("Сейчас нет подходящего уточнения. Назовите действие и запись.")
		}
		if nlp.HasDirective(message) && t.Slots.ReminderExpression == nil {
			return clarify("Уточните, это новая команда или ответ на вопрос.")
		}
		if p.Expectation == "reminder" && (t.Slots.Title != nil || t.Slots.TemporalExpression != nil || t.Slots.PeriodExpression != nil || t.Slots.Kind != nil) {
			return clarify("За сколько напомнить? Например, «за 30 минут».")
		}
		if t.Slots.Kind != nil || t.Slots.PeriodExpression != nil {
			return clarify("Уточните дату или время события.")
		}
		// A model cannot choose one time from a contradictory/negated reply by
		// returning just its clean substring. Validate the complete expected span.
		if t.Slots.TemporalExpression != nil {
			if raw, ok := nlp.ExtractExpectedTemporalSlot(message, expectedSlot(p)); ok {
				if _, err := nlp.ApplyDialogueSlots(p.Command, nlp.SlotUpdate{TemporalExpression: &raw}, message, now); err != nil {
					return clarify("Уточните одну дату или время. Ничего не изменено.")
				}
			}
		}
		cmd, err := nlp.ApplyDialogueSlots(p.Command, *t.Slots, message, now)
		if err != nil {
			return clarify("Не понял дату или время. Уточните, пожалуйста.")
		}
		cmd.Error = ""
		state.Pending = nil
		return cmd, nil, nil
	case "refinement":
		d := state.Dialogue
		if !freshResult(*state, now) || !in(d.LastAction.String(), "list", "search") || t.Slots.Title != nil || t.Slots.TemporalExpression != nil || t.Slots.ReminderExpression != nil {
			return clarify("Уточните период и тип записей для поиска.")
		}
		c = domain.Command{Action: d.LastAction, Kind: d.LastKind, Query: d.Query, PeriodStart: d.PeriodStart, PeriodEnd: d.PeriodEnd, Duration: time.Hour}
		if t.Slots.Kind != nil {
			_, _, explicitKind, _ := nlp.DetectIntent(message)
			if explicitKind.String() != *t.Slots.Kind {
				return clarify("Уточните тип записей для поиска.")
			}
		}
		cmd, err := nlp.ApplyDialogueSlots(c, *t.Slots, message, now)
		if err != nil {
			return clarify("Этот фильтр пока не поддерживается. Укажите период или тип записи.")
		}
		return cmd, nil, nil
	case "reference", "choice":
		pending := t.Type == "choice"
		if pending && (state.Pending == nil || state.Pending.Expectation != "choose") {
			return clarify("Сейчас нет списка для выбора.")
		}
		e, err := s.resolveReference(ctx, tx, state, *t.Reference, pending, now)
		if err != nil {
			return c, nil, err
		}
		if e == nil {
			return clarify("Какую запись вы имеете в виду? Укажите название или выберите её из списка.")
		}
		if pending {
			for _, snapshot := range state.Pending.Candidates {
				if snapshot.ID == e.ID && !sameRecord(snapshot, *e) {
					state.Pending = nil
					return clarify("Выбранная запись изменилась. Повторите запрос для нового списка.")
				}
			}
			c = state.Pending.Command
		} else {
			a := *t.Reference.Action
			_, detected, _, negated := nlp.DetectIntent(message)
			correction := a == "update" && in(state.Dialogue.LastAction.String(), "create", "update") && correctionCue.MatchString(message)
			if negated || detected.String() != a && !correction {
				return clarify("Уточните действие над этой записью.")
			}
			action := map[string]domain.Action{"update": domain.ActionUpdate, "delete": domain.ActionDelete, "complete": domain.ActionComplete}[a]
			c = domain.Command{Action: action, Duration: time.Hour}
			if t.Slots != nil {
				if t.Slots.Kind != nil || t.Slots.PeriodExpression != nil {
					return clarify("Уточните изменение даты, времени или названия.")
				}
				c, err = nlp.ApplyDialogueSlots(c, *t.Slots, message, now)
				if err != nil {
					return clarify("Уточните изменение даты или времени.")
				}
			}
		}
		if c.Kind != domain.EventUnknown && c.Kind != e.Kind {
			return clarify("Тип записи не совпадает. Уточните нужную запись.")
		}
		c.Kind = e.Kind
		c.TargetID, c.Query, c.Selector = &e.ID, "", nil
		state.Pending = nil
		return c, nil, nil
	}
	return clarify("Уточните запрос.")
}

// Apply execution outcomes only; a safe failure is not a replacement command.
func rememberDialogue(state *domain.State, cmd domain.Command, message string, response domain.Response, now time.Time, plan *nlp.SemanticPlan) {
	d := state.Dialogue
	if d == nil {
		d = &domain.DialogueState{LastAction: domain.ActionHelp}
		state.Dialogue = d
	}
	d.LastUserMessage, d.LastAssistantMessage = bounded(message, 500), bounded(response.Reply, 500)
	if in(response.Action.String(), "create", "update", "complete", "list", "search") {
		keepSearch := false
		if response.Action == domain.ActionUpdate && d.Search != nil && plan != nil && plan.Changes != nil {
			ch := *plan.Changes
			ch.Title, ch.Date, ch.Time, ch.Delay = nil, nil, nil, nil
			keepSearch = ch == (nlp.PlanChanges{}) && (plan.DialogueAct == "correction" || plan.Target != nil && plan.Target.Query == nil)
		}
		d.LastAction, d.LastKind, d.ResultAt, d.IdleTurns = response.Action, cmd.Kind, now, 0
		d.Query, d.PeriodStart, d.PeriodEnd = "", nil, nil
		if in(response.Action.String(), "list", "search") {
			d.Query, d.PeriodStart, d.PeriodEnd = cmd.Query, cmd.PeriodStart, cmd.PeriodEnd
			d.Filter = cmd.Filter
			d.Search = &domain.DialogueSearch{Action: response.Action, Kind: cmd.Kind, Query: cmd.Query, Filter: cmd.Filter, Start: cmd.PeriodStart, End: cmd.PeriodEnd, IDs: append([]int64{}, state.LastIDs...), At: now}
		} else {
			d.Filter = nil
			d.AffectedIDs = nil
			for _, e := range response.Events {
				d.AffectedIDs = append(d.AffectedIDs, e.ID)
			}
			d.AffectedAt = now
			if keepSearch {
				search := d.Search
				d.Query, d.Filter, d.PeriodStart, d.PeriodEnd = search.Query, search.Filter, search.Start, search.End
				d.ResultAt = search.At
				state.LastIDs = append([]int64{}, search.IDs...)
			} else {
				d.Search = nil
			}
		}
		if len(response.Events) == 1 && response.Action != domain.ActionList && response.Action != domain.ActionSearch {
			d.LastKind = response.Events[0].Kind
		}
	} else {
		// Clarifications preserve the result without extending its TTL.
		if response.Action == domain.ActionHelp {
			d.IdleTurns++
		}
	}
	if response.Action == domain.ActionDelete || response.Action == domain.ActionCancel {
		state.LastIDs, state.Dialogue = nil, nil
	}
}

func relativeReminderReply(cmd domain.Command, now time.Time) string {
	if cmd.ExactAt == nil || cmd.Kind != domain.EventReminder {
		return ""
	}
	delay := cmd.ExactAt.Sub(now)
	if delay > 0 && delay%time.Minute == 0 {
		if delay%time.Hour == 0 {
			hours := int(delay / time.Hour)
			return fmt.Sprintf("Готово. Через %d %s, в %s, напомню %s.", hours, russianCases(hours, "час", "часа", "часов"), cmd.ExactAt.Format("15:04"), lowerFirst(cmd.Title))
		}
		return fmt.Sprintf("Готово. Через %d минут напомню %s.", int(delay/time.Minute), lowerFirst(cmd.Title))
	}
	return ""
}

func lowerFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToLower(string(r[:1])) + string(r[1:])
}
