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
	"github.com/NIDJEL/assistant-ai/internal/social"
)

type operationInterpreter interface {
	InterpretOperation(context.Context, string, nlu.Context) (nlu.Operation, error)
}

// No slot parser is needed: optional slot fast paths are deliberately omitted.
func simpleControl(state domain.State, message string) string {
	if state.Pending == nil && state.Actions == nil {
		return ""
	}
	text := strings.Trim(strings.ToLower(message), " .!?")
	if in(text, "нет", "отмена", "не надо") {
		return "cancel"
	}
	if state.Pending != nil && in(state.Pending.Expectation, "delete", "delete_many", "batch", "conflict", "confirm_meeting", "atomic_group") && in(text, "да", "подтверждаю", "удаляй") {
		return "confirm"
	}
	return ""
}
func (s *Service) respondSimple(ctx context.Context, interpreter operationInterpreter, message, sid, rid string, now time.Time) (domain.Response, error) {
	turnStarted := time.Now()
	budget := s.TurnTimeout
	if budget <= 0 {
		budget = 45 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	ctx, metrics := nlu.WithMetrics(ctx)
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
		if err == nil {
			if session, e := social.In(ctx, tx, s.Calendar.Location); e == nil {
				if people, e := session.People(ctx); e == nil {
					for _, friend := range people.Friends {
						if len(compact.Friends) >= 100 {
							break
						}
						compact.Friends = append(compact.Friends, nlu.Friend{Name: friend.Name, Tags: friend.Tags})
					}
				}
			}
			if snapshot.Pending != nil && len(snapshot.Pending.SocialOperation) > 0 {
				var pending nlu.Operation
				if json.Unmarshal(snapshot.Pending.SocialOperation, &pending) == nil {
					compact.SocialPending = &pending
				}
			}
			if p := snapshot.Pending; p != nil && p.Scheduling != nil {
				for _, slot := range p.Scheduling.Slots {
					day := domain.Day(slot.Start.In(now.Location())).String()
					clock := slot.Start.In(now.Location()).Format("15:04")
					compact.OfferedSlots = append(compact.OfferedSlots, nlu.SocialTime{Date: &nlu.Temporal{Type: "absolute", Date: day}, Time: &clock})
				}
				for i, e := range p.Scheduling.Meetings {
					compact.MeetingChoices = append(compact.MeetingChoices, compactItem(e, i+1))
				}
			}
		}
		return err
	})
	if err != nil {
		return response, err
	}
	if cached != nil {
		if cached.Message != message || !sameChoice(cached.Response.SubmittedChoice, chatChoice(ctx)) {
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
	if choice := chatChoice(ctx); choice != nil {
		if strings.HasPrefix(choice.Action, "action_") {
			op, parseErr = actionChoiceOperation(snapshot, choice)
		} else {
			op, parseErr = choiceOperation(snapshot.Pending, choice)
		}
	} else if op.Intent == "" {
		calls = 1
		op, parseErr = interpreter.InterpretOperation(ctx, message, compact)
	}
	if parseErr == nil {
		parseErr = nlu.ValidateAt(op, now)
	}
	if ctx.Err() != nil {
		return response, ctx.Err()
	}
	if parseErr == nil && chatChoice(ctx) == nil && !in(op.Intent, "confirm", "cancel", "chat", "clarify") {
		issue, e := s.preflightSimple(ctx, snapshot, op, message, now)
		if e != nil {
			return response, e
		}
		if issue != nil {
			if repair, ok := interpreter.(interface {
				RepairValidation(context.Context, string, nlu.Context, nlu.Operation, nlu.ValidationIssue) (nlu.Operation, error)
			}); ok && metrics.Repairs == 0 {
				op, parseErr = repair.RepairValidation(ctx, message, compact, op, *issue)
			}
		}
	}
	latency := time.Since(started)
	metrics.InferenceMS = float64(latency.Microseconds()) / 1000
	if metrics.ModelCalls > 0 {
		calls = metrics.ModelCalls
	}
	if parseErr == nil {
		var e error
		ctx, e = s.prepareSearches(ctx, op, snapshot, now)
		if e != nil {
			return response, e
		}
	}
	if parseErr == nil && s.RefreshPlanning != nil {
		if op.Intent == "multi_action" {
			if args, e := op.Args(); e == nil {
				for _, step := range args.(*nlu.MultiAction).Steps {
					if _, e := step.Operation.Args(); e == nil && nlu.StepAllowed(step.Operation.Intent) {
						s.prepareSimplePlanning(ctx, step.Operation, now, nil)
					}
				}
			}
		}
		prepared := op
		planningPending := snapshot.Pending
		if op.Intent == "resume_action" {
			if args, e := op.Args(); e == nil {
				a := args.(*nlu.ResumeAction)
				prepared = a.Operation
				if snapshot.Actions != nil {
					for _, step := range snapshot.Actions.Steps {
						if step.Key == a.Key {
							planningPending = step.Pending
						}
					}
				}
			}
		}
		var planningAfter *time.Time
		if planningPending != nil && in(prepared.Intent, "choose_time", "continue", "select", "confirm") {
			raw, _ := json.Marshal(planningPending)
			var p domain.Pending
			_ = json.Unmarshal(raw, &p)
			args, _ := prepared.Args()
			if effective, _, e := socialContinuation(&p, prepared, args, now); e == nil {
				prepared = effective
				if p.Scheduling != nil {
					planningAfter = p.Scheduling.After
				}
			}
		}
		s.prepareSimplePlanning(ctx, prepared, now, planningAfter)
	}
	err = s.Store.Write(ctx, func(tx repository.Tx) error {
		cached, err := tx.Cached(ctx, sid, rid)
		if err != nil {
			return err
		}
		if cached != nil {
			if cached.Message != message || !sameChoice(cached.Response.SubmittedChoice, chatChoice(ctx)) {
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
			if chatChoice(ctx) != nil {
				response = answer(parseErr.Error(), domain.ActionClarify)
			} else {
				// A failed new request must not leave an older destructive preview armed.
				if state.Actions == nil && (state.Pending == nil || len(state.Pending.SocialOperation) == 0 && !in(state.Pending.Expectation, "fields", "reminder")) {
					state.Pending = nil
				}
				response = answer("Не удалось понять запрос. Уточните, что нужно сделать. Ничего не изменено.", domain.ActionClarify)
				if state.Actions != nil {
					for i := range state.Actions.Steps {
						step := &state.Actions.Steps[i]
						if step.Pending != nil && in(step.Pending.Expectation, "batch", "delete", "delete_many", "conflict", "confirm_meeting", "atomic_group") {
							if step.AtomicGroup != "" {
								group := state.Actions.Groups[step.AtomicGroup]
								group.Ready = false
								state.Actions.Groups[step.AtomicGroup] = group
							}
							step.Pending, step.Preview, step.Status, step.Reply = nil, nil, "failed", "Подтверждение сброшено после нераспознанного запроса. Уточните действие для нового предпросмотра."
							if step.Key == state.Actions.Active {
								state.Pending = nil
							}
						}
					}
					response.Reply += " Незавершённые действия сохранены."
				}
			}
		} else {
			cmd, response, err = s.applySimpleActions(ctx, tx, &state, op, message, now)
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
		if len(response.Operations) == 0 {
			rememberSimple(&state, cmd, response, now)
		}
		metrics.PlanSteps = len(response.Operations)
		if response.Search != nil {
			metrics.SearchCalls = response.Search.Calls
		}
		for _, step := range response.Operations {
			if in(step.Status, "waiting", "blocked", "failed") {
				metrics.WaitingSteps++
			}
		}
		rawState, e := json.Marshal(state)
		if e != nil {
			return e
		}
		metrics.StateBytes = len(rawState)
		if len(rawState) > 1<<20 {
			return domain.Validation("План слишком большой. Разделите запрос на меньшие группы.")
		}
		metrics.DurationMS = float64(time.Since(turnStarted).Microseconds()) / 1000
		response.Diagnostics = metrics
		response.SessionID, response.RequestID, response.Provider, response.Timezone, response.Pending = sid, rid, "simple_llm", s.Calendar.Location.String(), state.Pending != nil || state.Actions != nil
		response.SubmittedChoice = chatChoice(ctx)
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
			s.ObserveTurn(DialogueObservation{Metrics: *metrics, Type: op.Intent, Command: cmd, Provider: "simple_llm", InterpretationLatency: latency, SemanticCalls: calls, PlanValid: parseErr == nil, Route: RouteDecision{Path: "simple_llm", Reason: op.Intent}})
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
	r = s.syncExternal(ctx, r)
	s.refreshExecutionStatus(ctx, []*domain.Response{&r})
	return r
}

func (s *Service) syncExternal(ctx context.Context, r domain.Response) domain.Response {
	if s.Integrations == nil || len(r.Operations) == 0 && (r.Pending || !in(r.Action.String(), "create", "update")) {
		return r
	}
	for i, e := range r.Events {
		if len(r.Operations) > 0 {
			mutated := false
			for _, step := range r.Operations {
				if step.Applied && step.Status == "done" && in(step.Action.String(), "create", "update") {
					for _, id := range step.EventIDs {
						mutated = mutated || id == e.ID
					}
				}
			}
			if !mutated {
				continue
			}
		}
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
	if t.Value != "" || t.Filter != nil && t.Type != "last" {
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
		if t.Filter != nil {
			filter, err := simpleFilter(*t.Filter, now)
			if err != nil || filter.Query != "" {
				return errors.New("Уточните тип или период записи из последнего списка.")
			}
			// Narrow the active reference scope by stored fields. A task mentioning
			// a meeting in its title is still a task. Never search outside this scope.
			matching := []int64{}
			for _, id := range ids {
				e, err := tx.Get(ctx, id)
				if err != nil {
					return err
				}
				if e != nil && s.eventMatchesFilter(*e, *filter) {
					matching = append(matching, id)
				}
			}
			ids = matching
			if len(ids) == 0 {
				return errors.New("В последнем списке нет записи этого типа или периода. Уточните название или повторите поиск.")
			}
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
	if c.Recurrence != nil {
		return ""
	}
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

func (s *Service) applySimpleOperation(ctx context.Context, tx repository.Tx, state *domain.State, op nlu.Operation, message string, now time.Time) (domain.Command, domain.Response, error) {
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
	guardRecurringClock(args, message)
	op.Arguments, _ = json.Marshal(args)
	if create, ok := args.(*nlu.Create); ok && op.Intent == "create_meeting" {
		invite, decline := invitationIntent(message)
		if invite && decline {
			return clarify("Сохранить встречу только в вашем плане или отправить приглашения? Уточните участников.")
		}
		if decline {
			create.Personal, create.Invite = true, false
		} else if invite {
			create.Invite = true
		}
		if create.Personal && create.Invite {
			return clarify("Сохранить личную запись или отправить приглашения участникам?")
		}
		op.Arguments, _ = json.Marshal(create)
	}
	if handled, reply, e := s.applySocial(ctx, tx, state, op, args, now); handled {
		return c, reply, e
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
		if c.Kind == domain.EventMeeting && domain.GenericMeetingTitle(c.Title) {
			c.Title = ""
		}
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
		if a.Recurrence != nil {
			c.Recurrence, err = a.Recurrence.Bind(now, c.Date, c.Time)
			if err != nil {
				return clarify("Уточните правило повторения.")
			}
			if err = resolveSimpleRecurrence(&c, now); err != nil {
				return clarify(err.Error())
			}
		}
		if a.TimeAmbiguous {
			c.Time = nil
			r := ask(state, c, "Укажите точное время.", now, "fields", nil)
			state.Pending.RequiredSlot = "time"
			return c, r, nil
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
		f, err := simpleReadFilter(a, *state, now)
		if err != nil {
			return clarify(err.Error())
		}
		setSimpleFilter(&c, f)
		if c.Query != "" {
			c.Action = domain.ActionSearch
		}
	case *nlu.Update:
		c.Action = domain.ActionUpdate
		if err = normalizeOccurrenceTarget(a, now); err != nil {
			return clarify(err.Error())
		}
		if err = s.simpleTarget(ctx, tx, state, p, a.Target, &c, now); err != nil {
			return clarify(err.Error())
		}
		if err = simpleChanges(&c, a.Changes, now); err != nil {
			return clarify("Уточните изменения. Ничего не изменено.")
		}
		if a.Changes.TimeAmbiguous {
			c.Time = nil
			r := ask(state, c, "Укажите точное время.", now, "fields", nil)
			state.Pending.RequiredSlot = "time"
			return c, r, nil
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
			if in(p.Expectation, "fields", "reminder") {
				state.Pending = p
			}
			return clarify("Уточните дату, время, длительность или настройки напоминания.")
		}
		if a.Changes.TimeAmbiguous {
			c.Time = nil
			r := ask(state, c, "Укажите точное время.", now, "fields", nil)
			state.Pending.RequiredSlot = "time"
			return c, r, nil
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
