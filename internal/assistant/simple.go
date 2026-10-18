package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

type operationInterpreter interface {
	InterpretOperation(context.Context, string, nlu.Context) (nlu.Operation, error)
}

// No slot parser is needed: optional slot fast paths are deliberately omitted.
func simpleControl(state domain.State, message string) string {
	if state.Pending == nil {
		return ""
	}
	text := strings.Trim(strings.ToLower(message), " .!?")
	if in(text, "нет", "отмена", "не надо") {
		return "cancel"
	}
	if in(state.Pending.Expectation, "delete", "delete_many", "batch", "conflict") && in(text, "да", "подтверждаю", "удаляй") {
		return "confirm"
	}
	return ""
}
func (s *Service) respondSimple(ctx context.Context, interpreter operationInterpreter, message, sid, rid string, now time.Time) (domain.Response, error) {
	now = now.In(s.Calendar.Location)
	var response domain.Response
	unlock, err := s.sessionLock(ctx, sid)
	if err != nil {
		return response, err
	}
	defer unlock()
	var snapshot domain.State
	var cached *domain.Exchange
	var compact nlu.Context
	err = s.Store.Read(ctx, func(tx repository.Tx) error {
		var err error
		cached, err = tx.Cached(ctx, sid, rid)
		if err != nil || cached != nil {
			return err
		}
		snapshot, err = tx.State(ctx, sid)
		if err != nil {
			return err
		}
		expireDialogue(&snapshot, now)
		compact, err = buildSimpleContext(ctx, tx, snapshot, now)
		return err
	})
	if err != nil {
		return response, err
	}
	if cached != nil {
		if cached.Message != message {
			return response, domain.Validation("request_id уже использован для другого сообщения")
		}
		response = s.finishExternal(ctx, cached.Response)
		err = s.RefreshZoomCards(ctx, &response)
		return response, err
	}
	started := time.Now()
	calls := 0
	op := nlu.Operation{Intent: simpleControl(snapshot, message), Confidence: "high", Arguments: json.RawMessage(`{}`)}
	var parseErr error
	if op.Intent == "" {
		calls = 1
		op, parseErr = interpreter.InterpretOperation(ctx, message, compact)
	}
	if parseErr == nil {
		_, parseErr = op.Args()
	}
	if ctx.Err() != nil {
		return response, ctx.Err()
	}
	latency := time.Since(started)
	err = s.Store.Write(ctx, func(tx repository.Tx) error {
		cached, err := tx.Cached(ctx, sid, rid)
		if err != nil {
			return err
		}
		if cached != nil {
			if cached.Message != message {
				return domain.Validation("request_id уже использован для другого сообщения")
			}
			response = cached.Response
			return nil
		}
		state, err := tx.State(ctx, sid)
		if err != nil {
			return err
		}
		expireDialogue(&state, now)
		cmd := neutral("simple_llm")
		if !sameDialogueSnapshot(snapshot, state) {
			response = answer("Контекст изменился. Повторите запрос или выберите запись заново.", domain.ActionClarify)
		} else if parseErr != nil {
			// A failed new request must not leave an older destructive preview armed.
			state.Pending = nil
			response = answer("Не удалось понять запрос. Уточните, что нужно сделать. Ничего не изменено.", domain.ActionClarify)
		} else {
			cmd, response, err = s.applySimple(ctx, tx, &state, op, message, now)
			if err != nil {
				return err
			}
		}
		if p := state.Pending; p != nil {
			if in(p.Expectation, "delete", "delete_many", "batch", "conflict") {
				p.ConfirmationState = "pending_confirmation"
			}
			if p.Expectation == "conflict" && p.Command.TargetID != nil && p.TargetSnapshot == nil {
				p.TargetSnapshot, err = tx.Get(ctx, *p.Command.TargetID)
				if err != nil {
					return err
				}
			}
		}
		rememberSimple(&state, cmd, response, now)
		response.SessionID, response.RequestID, response.Provider, response.Timezone, response.Pending = sid, rid, "simple_llm", s.Calendar.Location.String(), state.Pending != nil
		if err = tx.SaveState(ctx, sid, state, now); err != nil {
			return err
		}
		if err = s.zoomCards(ctx, tx, &response); err != nil {
			return err
		}
		if err = tx.SaveExchange(ctx, sid, rid, message, response, now); err != nil {
			return err
		}
		if s.ObserveTurn != nil {
			s.ObserveTurn(DialogueObservation{Type: op.Intent, Command: cmd, Provider: "simple_llm", InterpretationLatency: latency, SemanticCalls: calls, PlanValid: parseErr == nil, Route: RouteDecision{Path: "simple_llm", Reason: op.Intent}})
		}
		return nil
	})
	if err == nil {
		response = s.finishExternal(ctx, response)
		err = s.RefreshZoomCards(ctx, &response)
	}
	return response, err
}

func (s *Service) finishExternal(ctx context.Context, r domain.Response) domain.Response {
	if s.Integrations == nil || r.Pending || !in(r.Action.String(), "create", "update") {
		return r
	}
	for i, e := range r.Events {
		google, zoom := e.CalendarProvider == "google", e.ConferenceProvider == "zoom"
		if !google && !zoom {
			continue
		}
		current, err := s.Integrations.SyncEvent(ctx, e.ID, google, zoom)
		if err != nil {
			r.Reply += " Изменение сохранено в Гене; синхронизация с внешним сервисом ещё не подтверждена. Проверьте раздел «Интеграции»."
			continue
		}
		if current != nil {
			r.Events[i] = *current
			if google {
				r.Reply += " Событие отправлено в Google Calendar."
			}
		}
	}
	return r
}

func (s *Service) simpleTarget(ctx context.Context, tx repository.Tx, state *domain.State, pending *domain.Pending, t nlu.Target, c *domain.Command, now time.Time) error {
	if t.Type == "query" {
		if strings.TrimSpace(t.Value) == "" || t.Index != 0 {
			return errors.New("Назовите нужную запись.")
		}
		c.Query = t.Value
		if t.Filter != nil {
			f, err := simpleFilter(*t.Filter, now)
			if err != nil {
				return err
			}
			if f.Query != "" {
				return errors.New("Укажите один поисковый запрос.")
			}
			f.Query = t.Value
			setSimpleFilter(c, f)
		}
		return nil
	}
	if t.Value != "" || t.Filter != nil {
		return errors.New("Некорректная ссылка на запись.")
	}
	if t.Type == "ordinal" && pending != nil && pending.Expectation == "choose" && pending.Command.Provider == "simple_llm" {
		if t.Index < 1 || t.Index > min(len(pending.Candidates), 8) {
			return errors.New("Выберите запись из предложенного списка.")
		}
		snapshot := pending.Candidates[t.Index-1]
		e, err := tx.Get(ctx, snapshot.ID)
		if err != nil {
			return err
		}
		if e == nil || !sameRecord(snapshot, *e) {
			return errors.New("Запись изменилась. Повторите запрос.")
		}
		if c.Action == pending.Command.Action {
			*c = pending.Command
		}
		c.Query, c.Filter, c.Selector = "", nil, nil
		c.TargetID, c.Kind = &e.ID, e.Kind
		return nil
	}
	ids := state.LastIDs
	if t.Type == "last" {
		if t.Index != 0 {
			return errors.New("Некорректная ссылка на запись.")
		}
		if freshAffected(*state, now) && in(state.Dialogue.LastAction.String(), "create", "update", "complete") && len(state.Dialogue.AffectedIDs) > 0 {
			ids = state.Dialogue.AffectedIDs
		} else if !freshResult(*state, now) {
			ids = nil
		}
		if len(ids) != 1 {
			return errors.New("Какую запись вы имеете в виду? Укажите название или номер из списка.")
		}
	} else if t.Type == "ordinal" {
		if !freshResult(*state, now) || t.Index < 1 || t.Index > min(len(ids), 8) {
			return errors.New("Укажите номер из последнего списка.")
		}
		ids = []int64{ids[t.Index-1]}
	} else {
		return errors.New("Назовите запись, которую нужно изменить.")
	}
	e, err := tx.Get(ctx, ids[0])
	if err != nil {
		return err
	}
	if e == nil {
		return errors.New("Запись уже недоступна. Повторите поиск.")
	}
	c.TargetID, c.Kind = &e.ID, e.Kind
	return nil
}

var obviousClock = regexp.MustCompile(`\b[0-2]?[0-9]:[0-5][0-9]\b`)

func omittedSimpleSchedule(c domain.Command, a *nlu.Create, message string) string {
	for _, token := range strings.Fields(strings.ToLower(message)) {
		token = strings.Trim(token, ",.!?;:«»\"")
		if a.Date == nil && in(token, "завтра", "послезавтра") {
			return "date"
		}
	}
	if a.Time == nil && c.Time == nil && obviousClock.MatchString(message) {
		return "time"
	}
	return ""
}

func (s *Service) applySimple(ctx context.Context, tx repository.Tx, state *domain.State, op nlu.Operation, message string, now time.Time) (domain.Command, domain.Response, error) {
	c := neutral("simple_llm")
	clarify := func(text string) (domain.Command, domain.Response, error) {
		return c, answer(text, domain.ActionClarify), nil
	}
	args, err := op.Args()
	if err != nil {
		state.Pending = nil
		return clarify("Некорректные аргументы. Ничего не изменено.")
	}
	if op.Confidence == "low" || op.Intent == "clarify" {
		if p := state.Pending; p != nil && in(p.Expectation, "batch", "delete", "delete_many", "conflict") {
			state.Pending = nil
		}
		q := "Уточните действие и нужную запись."
		if op.Clarification != nil {
			q = *op.Clarification
		}
		return clarify(q)
	}
	if op.Intent == "chat" {
		return c, answer(args.(*nlu.Chat).Reply, domain.ActionHelp), nil
	}
	if op.Intent == "cancel" {
		state.Pending = nil
		return c, answer("Действие отменено.", domain.ActionCancel), nil
	}
	if op.Intent == "confirm" {
		if state.Pending == nil || state.Pending.Command.Provider != "simple_llm" {
			state.Pending = nil
			return clarify("Сейчас нет подтверждённого предпросмотра. Повторите исходный запрос.")
		}
		c = state.Pending.Command
		r, err := s.confirmSimple(ctx, tx, state, now)
		return c, r, err
	}
	// Preserve a pending command only for an explicit typed continuation/choice.
	p := state.Pending
	state.Pending = nil
	switch a := args.(type) {
	case *nlu.Create:
		c.Action = domain.ActionCreate
		c.Kind, _ = domain.ParseEventType(strings.TrimPrefix(op.Intent, "create_"))
		c.Title, c.ReminderMinutes, c.ReminderEnabled = a.Title, a.ReminderMinutes, a.ReminderEnabled
		if (a.Calendar != "" && a.Calendar != "google") || (a.Conference != "" && a.Conference != "zoom") {
			return clarify("Выберите Google Calendar или Zoom.")
		}
		c.CalendarProvider, c.ConferenceProvider = a.Calendar, a.Conference
		if a.Conference != "" && c.Kind != domain.EventMeeting {
			return clarify("Zoom можно добавить к встрече.")
		}
		for _, provider := range []string{a.Calendar, a.Conference} {
			if provider != "" {
				if s.Integrations == nil {
					return clarify("Подключите " + provider + " в разделе «Интеграции».")
				}
				if e := s.Integrations.Ready(ctx, provider); e != nil {
					return clarify(e.Error())
				}
			}
		}
		if a.DurationMinutes != nil {
			if *a.DurationMinutes < 5 || *a.DurationMinutes > 1440 {
				return clarify("Длительность должна быть от 5 минут до суток.")
			}
			c.Duration = time.Duration(*a.DurationMinutes) * time.Minute
		}
		if err = simpleDate(&c, a.Date, a.Time, now); err != nil {
			return clarify("Уточните дату или время.")
		}
		if missing := omittedSimpleSchedule(c, a, message); missing != "" {
			q := "На какую дату поставить запись?"
			if missing == "time" {
				q = "Во сколько поставить запись?"
			}
			r := ask(state, c, q, now, "fields", nil)
			state.Pending.RequiredSlot = missing
			return c, r, nil
		}
	case *nlu.Read:
		c.Action = domain.ActionList
		c.Next = a.Next
		f, err := simpleFilter(a.Filter, now)
		if err != nil {
			return clarify("Уточните фильтры поиска.")
		}
		if a.Inherit {
			if !freshResult(*state, now) || state.Dialogue.Search == nil {
				return clarify("Сначала задайте поиск, который нужно уточнить.")
			}
			q := state.Dialogue.Search
			base := domain.ExecutionFilter{Kind: q.Kind, Query: q.Query, Start: q.Start, End: q.End}
			if q.Filter != nil {
				base = *q.Filter
			}
			if a.Kind != "" {
				base.Kind = f.Kind
			}
			if a.Conference != "" {
				base.Conference = f.Conference
			}
			if a.Query != "" {
				base.Query = f.Query
			}
			if a.Date != nil || a.Period != nil {
				base.Start, base.End = f.Start, f.End
			}
			if a.Status != nil {
				base.Status = f.Status
			}
			f = &base
		}
		setSimpleFilter(&c, f)
		if c.Query != "" {
			c.Action = domain.ActionSearch
		}
	case *nlu.Update:
		c.Action = domain.ActionUpdate
		if err = s.simpleTarget(ctx, tx, state, p, a.Target, &c, now); err != nil {
			return clarify(err.Error())
		}
		if err = simpleChanges(&c, a.Changes, now); err != nil {
			return clarify("Уточните изменения. Ничего не изменено.")
		}
	case *nlu.Mutation:
		if op.Intent == "select" {
			if p == nil || p.Expectation != "choose" || p.Command.Provider != "simple_llm" || a.Target.Type != "ordinal" || a.Target.Value != "" || a.Target.Filter != nil || a.Target.Index < 1 || a.Target.Index > min(len(p.Candidates), 8) {
				return clarify("Выберите запись из предложенного списка.")
			}
			snapshot := p.Candidates[a.Target.Index-1]
			current, err := tx.Get(ctx, snapshot.ID)
			if err != nil {
				return c, domain.Response{}, err
			}
			if current == nil || !sameRecord(snapshot, *current) {
				return clarify("Запись изменилась. Повторите запрос.")
			}
			c = p.Command
			c.Query, c.Filter, c.Selector = "", nil, nil
			c.TargetID = &current.ID
		} else {
			c.Action = domain.ActionDelete
			if op.Intent == "complete" {
				c.Action = domain.ActionComplete
			}
			if err = s.simpleTarget(ctx, tx, state, p, a.Target, &c, now); err != nil {
				return clarify(err.Error())
			}
		}
	case *nlu.Continue:
		if p == nil || !in(p.Expectation, "fields", "reminder", "conflict") || p.Command.Provider != "simple_llm" {
			return clarify("Сейчас нет команды для такого уточнения.")
		}
		c = p.Command
		c.Error = ""
		if err = simpleChanges(&c, a.Changes, now); err != nil {
			return clarify("Уточните дату, время или настройки напоминания.")
		}
		if p.RequiredSlot == "date" && a.Changes.Date == nil || p.RequiredSlot == "time" && a.Changes.Time == nil && (a.Changes.Date == nil || a.Changes.Date.Type != "relative_delay") {
			state.Pending = p
			return clarify("Ответьте на ожидаемое уточнение даты или времени.")
		}
	case *nlu.BatchUpdate:
		c.Action, c.Batch = domain.ActionUpdate, true
		if err = s.prepareSimpleBatch(&c, a.Filter, a.Exclude, now); err != nil {
			return clarify("Уточните группу записей.")
		}
		if a.Changes == (nlu.Changes{}) {
			return clarify("Что изменить у выбранных записей?")
		}
		if err = simpleChanges(&c, a.Changes, now); err != nil {
			return clarify("Уточните изменения группы.")
		}
	case *nlu.Batch:
		c.Action, c.Batch = domain.ActionDelete, true
		if op.Intent == "batch_complete" {
			c.Action = domain.ActionComplete
		}
		if err = s.prepareSimpleBatch(&c, a.Filter, a.Exclude, now); err != nil {
			return clarify("Уточните группу записей.")
		}
	default:
		return clarify("Уточните действие.")
	}
	if err = c.Validate(); err != nil {
		return clarify("Аргументы не прошли проверку. Ничего не изменено.")
	}
	r, err := s.executeValidated(ctx, tx, state, c, now, false)
	return c, r, err
}
func (s *Service) prepareSimpleBatch(c *domain.Command, f nlu.Filter, exclude []nlu.Filter, now time.Time) error {
	if len(exclude) > 20 {
		return errors.New("too many exclusions")
	}
	v, err := simpleFilter(f, now)
	if err != nil {
		return err
	}
	for _, x := range exclude {
		e, err := simpleFilter(x, now)
		if err != nil {
			return err
		}
		v.Exclude = append(v.Exclude, *e)
	}
	setSimpleFilter(c, v)
	return c.Validate()
}
