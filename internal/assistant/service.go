package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

type Interpreter interface {
	Interpret(context.Context, string, time.Time) (domain.Command, error)
}
type lockEntry struct {
	token chan struct{}
	refs  int
}
type Service struct {
	ReadExecutionStatus func(context.Context, []int64, time.Time) ([]domain.ExecutionStatus, error)
	TurnTimeout         time.Duration
	RefreshPlanning     func(context.Context, []string, time.Time, time.Time)
	Integrations        ExternalIntegrations
	Store               repository.Store
	Interpreter         Interpreter
	Calendar            *calendar.Service
	AskReminderSettings bool
	ObserveTurn         func(DialogueObservation)
	mu                  sync.Mutex
	locks               map[string]*lockEntry
}

type ExternalIntegrations interface {
	Ready(context.Context, string) error
	SyncEvent(context.Context, int64, bool, bool) (*domain.Event, error)
}

func New(store repository.Store, interpreter Interpreter, cal *calendar.Service) *Service {
	s := &Service{Store: store, Interpreter: interpreter, Calendar: cal, locks: map[string]*lockEntry{}}
	if reader, ok := store.(repository.ExecutionStatusReader); ok {
		s.ReadExecutionStatus = reader.ExecutionStatuses
	}
	return s
}
func (s *Service) sessionLock(ctx context.Context, id string) (func(), error) {
	id = repository.UserID(ctx) + ":" + id
	s.mu.Lock()
	e := s.locks[id]
	if e == nil {
		e = &lockEntry{token: make(chan struct{}, 1)}
		s.locks[id] = e
	}
	e.refs++
	s.mu.Unlock()
	drop := func() {
		s.mu.Lock()
		e.refs--
		if e.refs == 0 {
			delete(s.locks, id)
		}
		s.mu.Unlock()
	}
	select {
	case e.token <- struct{}{}:
		return func() { <-e.token; drop() }, nil
	case <-ctx.Done():
		drop()
		return nil, ctx.Err()
	}
}
func (s *Service) Respond(ctx context.Context, message, sid, rid string, now time.Time) (domain.Response, error) {
	if interpreter, ok := s.Interpreter.(operationInterpreter); ok {
		return s.respondSimple(ctx, interpreter, message, sid, rid, now)
	}
	now = now.In(s.Calendar.Location)
	var response domain.Response
	unlock, err := s.sessionLock(ctx, sid)
	if err != nil {
		return response, err
	}
	defer unlock()
	var cached *domain.Exchange
	var snapshot domain.State
	var dialogue nlp.DialogueContext
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
		dialogue, err = buildDialogueContext(ctx, tx, snapshot)
		return err
	})
	if err != nil {
		return response, err
	}
	if cached != nil {
		if cached.Message != message {
			return response, domain.Validation("request_id уже использован для другого сообщения")
		}
		response = cached.Response
		err = s.RefreshZoomCards(ctx, &response)
		return response, err
	}
	// Both deterministic interpretation and external LLM HTTP happen before BEGIN IMMEDIATE.
	interpretStarted := time.Now()
	var cmd domain.Command
	var turn *nlp.ContextualTurn
	var plan *nlp.SemanticPlan
	var route RouteDecision
	semanticCalls, planValid := 0, false
	var evidenceError *nlp.EvidenceError
	if planner, ok := s.Interpreter.(planInterpreter); ok {
		route, turn = exactPlanFastPath(snapshot, message, now)
		cmd = neutral(route.Path)
		if turn == nil {
			semanticCalls = 1
			parsed, parseErr := planner.ParsePlan(ctx, message, dialogue, now)
			parsed, _ = nlp.CanonicalizePlanSource(parsed, message)
			planValid = parsed.Validate() == nil
			if parseErr == nil {
				parseErr = parsed.Validate()
			}
			if parseErr == nil {
				parseErr = nlp.ValidatePlanEvidence(parsed, message)
			}
			if errors.As(parseErr, &evidenceError) {
				planValid = true
			}
			if ctx.Err() != nil {
				return response, ctx.Err()
			}
			if parseErr != nil {
				cmd.Error = "Не удалось безопасно понять запрос. Уточните формулировку. Ничего не изменено."
			} else {
				plan, planValid = &parsed, true
			}
		}
	} else {
		cmd, turn, err = s.interpretDialogue(ctx, snapshot, dialogue, message, now)
	}
	interpretLatency := time.Since(interpretStarted)
	if err != nil {
		return response, err
	}
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
		if evidenceError != nil {
			// A subsequent exact "yes" must not authorize an older preview after
			// a newly rejected mutation has changed the user's intent.
			state.Pending = nil
			cmd.Error = "Не уверен, что правильно понял дату, время или нужные записи. Уточните запрос. Ничего не изменено."
		}
		provider := cmd.Provider
		var resolved *domain.Response
		if route.Reason == "context_cancellation" && sameDialogueSnapshot(snapshot, state) {
			state.LastIDs, state.Dialogue = nil, nil
			r := answer("Сейчас нечего подтверждать.", domain.ActionHelp)
			resolved = &r
		}
		if route.Reason == "deferred_confirmation" && state.Pending != nil && sameDialogueSnapshot(snapshot, state) {
			state.Pending.ConfirmationState = "deferred_confirmation"
			r := answer("Подтверждение отложено. Предпросмотр сохранён; можно подтвердить или отменить действие.", domain.ActionClarify)
			resolved = &r
		}
		// Another service instance may have advanced this same dialogue while
		// inference was in flight. Never apply a contextual reply to a new state.
		usesPending := turn != nil && in(turn.Type, "confirmation", "choice", "slot_reply") || plan != nil && in(plan.DialogueAct, "choice", "slot_reply")
		if resolved != nil {
			// A deferred preview is a dialogue transition, never a new command.
		} else if route.Path != "" && usesPending && state.Pending != nil && state.Pending.EvidenceVersion != 1 {
			state.Pending = nil
			r := answer("Повторите исходную команду: сначала нужно проверить смысл запроса.", domain.ActionClarify)
			resolved = &r
		} else if (plan != nil || turn != nil && turn.Type != "command") && !sameDialogueSnapshot(snapshot, state) {
			r := answer("Контекст диалога изменился. Повторите продолжение или выберите запись заново.", domain.ActionClarify)
			resolved = &r
		} else if plan != nil {
			cmd, resolved, err = s.resolvePlan(ctx, tx, &state, *plan, message, now)
			if err != nil {
				return err
			}
		} else if turn != nil {
			cmd, resolved, err = s.resolveTurn(ctx, tx, &state, *turn, message, now)
			if err != nil {
				return err
			}
		}
		executionMessage := message
		if route.Path != "" {
			executionMessage = ""
		}
		if turn != nil && turn.Type == "confirmation" {
			executionMessage = message
		}
		if turn != nil && in(turn.Type, "slot_reply", "reference", "choice") {
			executionMessage = ""
		}
		if resolved != nil {
			response = *resolved
		} else if turn != nil && turn.Type == "confirmation" && route.Path != "" && state.Pending != nil && state.Pending.EvidenceVersion != 1 {
			state.Pending = nil
			response = answer("Повторите исходную команду: для этого подтверждения нужно заново проверить смысл запроса.", domain.ActionClarify)
		} else if turn != nil && turn.Type == "confirmation" && state.Pending != nil && state.Pending.Expectation == "batch" {
			response, err = s.confirmBatch(ctx, tx, &state, now)
		} else {
			response, err = s.execute(ctx, tx, &state, cmd, executionMessage, now)
		}
		if err != nil {
			return err
		}
		newCheckedCommand := plan != nil && plan.Status == "ok" && plan.DialogueAct == "command" && plan.Operation != nil && *plan.Operation != "chat"
		checkedContinuation := snapshot.Pending != nil && snapshot.Pending.EvidenceVersion == 1 && (turn != nil && in(turn.Type, "slot_reply", "choice") || plan != nil && in(plan.DialogueAct, "slot_reply", "choice"))
		if state.Pending != nil && (newCheckedCommand || checkedContinuation) {
			state.Pending.EvidenceVersion = 1
		}
		if state.Pending != nil && in(state.Pending.Expectation, "batch", "delete", "delete_many", "conflict") && state.Pending.ConfirmationState == "" {
			state.Pending.ConfirmationState = "pending_confirmation"
		}
		response.SessionID, response.RequestID, response.Provider, response.Timezone, response.Pending = sid, rid, provider, s.Calendar.Location.String(), state.Pending != nil
		rememberDialogue(&state, cmd, message, response, now, plan)
		if s.ObserveTurn != nil {
			typeName := "command"
			if turn != nil {
				typeName = turn.Type
			}
			if plan != nil {
				typeName = plan.DialogueAct
			}
			reference := ""
			if turn != nil && turn.Reference != nil {
				reference = turn.Reference.Relation
			}
			o := DialogueObservation{Type: typeName, Reference: reference, Command: cmd, Provider: provider, InterpretationLatency: interpretLatency, Route: route, SemanticCalls: semanticCalls, PlanValid: planValid}
			if evidenceError != nil {
				o.EvidenceField, o.EvidenceReason = evidenceError.Field, evidenceError.Reason
			}
			s.ObserveTurn(o)
		}
		if err = tx.SaveState(ctx, sid, state, now); err != nil {
			return err
		}
		if err = s.zoomCards(ctx, tx, &response); err != nil {
			return err
		}
		return tx.SaveExchange(ctx, sid, rid, message, response, now)
	})
	return response, err
}

func sameDialogueSnapshot(a, b domain.State) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func nextScheduled(events []domain.Event, now time.Time, loc *time.Location) []domain.Event {
	upcoming := []domain.Event{}
	for _, e := range events {
		if start, _, ok := e.Interval(loc); ok && !start.Before(now) {
			upcoming = append(upcoming, e)
		}
	}
	sort.SliceStable(upcoming, func(i, j int) bool {
		a, _, _ := upcoming[i].Interval(loc)
		b, _, _ := upcoming[j].Interval(loc)
		return a.Before(b)
	})
	if len(upcoming) > 1 {
		upcoming = upcoming[:1]
	}
	return upcoming
}

const help = "Я Гена. Помогу с задачами, встречами и напоминаниями. Например: «Напомни завтра в 10 утра позвонить Ивану», «В пятницу нужно отправить отчёт», «Что у меня на эту неделю?» или «Найди задачу по договору»."

func answer(text string, action domain.Action, events ...domain.Event) domain.Response {
	if events == nil {
		events = []domain.Event{}
	}
	return domain.Response{Reply: text, Action: action, Events: events}
}
func describe(e domain.Event) string {
	moment := "без срока"
	if e.Date != nil {
		moment = e.Date.Format("02.01.2006")
	}
	if e.StartTime != nil {
		moment += " в " + e.StartTime.String()
	}
	return fmt.Sprintf("#%d «%s» — %s", e.ID, e.Title, moment)
}
func ask(state *domain.State, cmd domain.Command, text string, now time.Time, expect string, candidates []domain.Event) domain.Response {
	state.Pending = &domain.Pending{Command: cmd, Expectation: expect, At: now, Candidates: candidates}
	result := answer(text, domain.ActionClarify)
	if candidates == nil {
		candidates = []domain.Event{}
	}
	result.Candidates = candidates
	if expect == "reminder" {
		result.Suggestions = []string{"За 15 минут", "За час", "В момент события", "Без уведомлений"}
	}
	return result
}

var digits = regexp.MustCompile(`^#?\d+$`)

func isAffirmative(text string) bool {
	return nlp.ClassifyConfirmation(text) == nlp.ConfirmationYes
}

func in(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}
func joinEvents(events []domain.Event, limit int, separator string) string {
	parts := []string{}
	for i, e := range events {
		if i >= limit {
			break
		}
		parts = append(parts, describe(e))
	}
	return strings.Join(parts, separator)
}
func (s *Service) execute(ctx context.Context, tx repository.Tx, state *domain.State, cmd domain.Command, message string, now time.Time) (domain.Response, error) {
	low := strings.Trim(strings.ReplaceAll(strings.ToLower(message), "ё", "е"), " .!?")
	answeringReminder := state.Pending != nil && state.Pending.Expectation == "reminder"
	if in(low, "отмена", "отмени", "отменить", "стоп") || !answeringReminder && nlp.ClassifyConfirmation(message) == nlp.ConfirmationNo {
		state.Pending = nil
		return answer("Действие отменено.", domain.ActionCancel), nil
	}
	// A rejected/negated request is not an answer supplying missing fields.
	// In particular, "не добавляй завтра в 10" must not complete a pending create.
	if cmd.Action == domain.ActionHelp && cmd.Error != "" {
		return answer(cmd.Error, domain.ActionClarify), nil
	}
	force := false
	if p := state.Pending; p != nil {
		previous := p.Command
		yes := isAffirmative(low)
		switch {
		case p.Expectation == "reminder":
			minutes, enabled, recognized, question := nlp.ReminderChoice(message)
			if recognized {
				if question != "" {
					return answer(question, domain.ActionClarify), nil
				}
				cmd = previous
				cmd.ReminderMinutes, cmd.ReminderEnabled = minutes, enabled
				state.Pending = nil
			} else if cmd.Action == domain.ActionHelp {
				return ask(state, previous, "За сколько минут или часов напомнить? Например: «за 15 минут», «за час» или «без уведомлений».", now, "reminder", nil), nil
			} else {
				state.Pending = nil
			}
		case in(p.Expectation, "delete", "delete_many") && yes:
			return s.confirmDeletion(ctx, tx, state, p, now)
		case p.Expectation == "conflict" && yes:
			cmd = previous
			force = true
			state.Pending = nil
		case p.Expectation == "choose" && (cmd.Action == domain.ActionHelp || cmd.TargetID != nil || !nlp.HasDirective(message)):
			e, question, err := s.choosePending(ctx, tx, state, message, low, cmd, now)
			if err != nil {
				return domain.Response{}, err
			}
			if question != nil {
				return *question, nil
			}
			cmd = previous
			id := e.ID
			cmd.TargetID = &id
			cmd.Query, cmd.Selector = "", nil
			state.Pending = nil
		case in(p.Expectation, "fields", "conflict") && (cmd.Action == domain.ActionHelp || (p.Expectation == "fields" && previous.Title == "" && cmd.Action == domain.ActionCreate && !hasNewDirective(low))):
			t := nlp.ParseDateTimeReply(message, now, previous)
			if extra := nlp.Extract(t.Clean(message), now); t.Time != nil && t.Clean(message) != strings.Trim(strings.Join(strings.Fields(message), " "), " ,.!?;:-") && extra.Time != nil {
				return answer("Вижу несколько времён. Уточните одно время события.", domain.ActionClarify), nil
			}
			if t.Error != "" {
				return answer(t.Error, domain.ActionClarify), nil
			}
			cmd = previous
			if t.Recurrence != nil {
				cmd.Recurrence = t.Recurrence
			}
			reminderOnly := false
			if minutes, enabled, recognized, question := nlp.ReminderChoice(message); recognized && t.Date == nil && t.Time == nil {
				if question != "" {
					return answer(question, domain.ActionClarify), nil
				}
				cmd.ReminderMinutes, cmd.ReminderEnabled = minutes, enabled
				reminderOnly = true
			}
			if cmd.Title == "" && p.Expectation == "fields" && t.Date == nil && t.Time == nil && !reminderOnly {
				cmd.Title = strings.TrimSpace(message)
			}
			if t.Date != nil {
				cmd.Date = t.Date
			}
			if t.Date != nil || t.Time != nil {
				cmd.ExactAt = nil
				if t.Relative {
					cmd.ExactAt = t.At
				}
			}
			if t.Time != nil {
				cmd.Time = t.Time
				cmd.StartSecond = t.StartSecond
			}
			nlp.ResolveRecurrence(&cmd, now)
			state.Pending = nil
		case cmd.Action != domain.ActionHelp:
			state.Pending = nil
		default:
			result := answer("Ответьте на уточнение или напишите «отмена». Чтобы сменить действие, сформулируйте новый запрос.", domain.ActionClarify)
			result.Candidates = p.Candidates
			return result, nil
		}
	}
	return s.executeValidated(ctx, tx, state, cmd, now, force)
}

// executeValidated receives Go-resolved commands, with no natural-language input.
// Pending dialogue is handled by the caller; this is the shared product executor.
func (s *Service) executeValidated(ctx context.Context, tx repository.Tx, state *domain.State, cmd domain.Command, now time.Time, force bool) (domain.Response, error) {
	if cmd.Error != "" {
		if cmd.Action == domain.ActionCreate || cmd.Action == domain.ActionUpdate {
			question := cmd.Error
			cmd.Error = ""
			return ask(state, cmd, question, now, "fields", nil), nil
		}
		return answer(cmd.Error, domain.ActionClarify), nil
	}
	if cmd.Action == domain.ActionHelp {
		return answer(help, domain.ActionHelp), nil
	}
	if cmd.Batch {
		return s.previewBatch(ctx, tx, state, cmd, now)
	}
	if cmd.Action == domain.ActionCreate {
		question := ""
		if cmd.Provider == "simple_llm" {
			question = simpleCreationQuestion(cmd)
		} else {
			question = creationQuestion(cmd)
		}
		if question != "" {
			return ask(state, cmd, question, now, "fields", nil), nil
		}
		reminderAt := time.Time{}
		if cmd.Date != nil && cmd.Time != nil {
			reminderAt = cmd.Date.At(*cmd.Time, now.Location()).Add(time.Duration(cmd.StartSecond) * time.Second)
		}
		if cmd.ExactAt != nil {
			reminderAt = *cmd.ExactAt
		}
		if cmd.Kind == domain.EventReminder && cmd.Date != nil && cmd.Time != nil && cmd.Recurrence == nil && !reminderAt.After(now) {
			cmd.Date = nil
			r := ask(state, cmd, "Это время уже прошло. На какую дату напомнить «"+cmd.Title+"» — сегодня или завтра?", now, "fields", nil)
			state.Pending.RequiredSlot = "date"
			return r, nil
		}
		if utf8.RuneCountInString(cmd.Title) > 500 {
			return answer("Сократите название до 500 символов.", domain.ActionClarify), nil
		}
		if s.AskReminderSettings && cmd.Recurrence == nil && (cmd.Kind != domain.EventReminder || cmd.NotificationContext) && cmd.Date != nil && cmd.Time != nil && cmd.ReminderMinutes == nil && (cmd.ReminderEnabled == nil || *cmd.ReminderEnabled) {
			return ask(state, cmd, "За сколько минут или часов напомнить о «"+cmd.Title+"»? Например: «за 15 минут», «за час», «в момент события» или «без уведомлений».", now, "reminder", nil), nil
		}
		kind := cmd.Kind
		if kind == domain.EventUnknown {
			kind = domain.EventTask
		}
		e := domain.Event{EventDetails: domain.EventDetails{Participants: cmd.Participants, CalendarProvider: cmd.CalendarProvider, ConferenceProvider: cmd.ConferenceProvider, ReminderMinutes: cmd.ReminderMinutes, ReminderEnabled: cmd.ReminderEnabled, StartSecond: cmd.StartSecond, Recurrence: cmd.Recurrence, OccurrenceAt: cmd.ExactAt}, Kind: kind, Title: cmd.Title, Date: cmd.Date, StartTime: cmd.Time, Duration: cmd.Duration, Status: domain.StatusOpen, Priority: domain.PriorityNormal}
		if err := e.Validate(); err != nil {
			return answer(err.Error(), domain.ActionClarify), nil
		}
		conflicts, err := s.Calendar.Conflicts(ctx, tx, e, 0)
		if err != nil {
			return domain.Response{}, err
		}
		if len(conflicts) > 0 && !force && ctx.Value(atomicRunKey{}) != true {
			return ask(state, cmd, "Это время пересекается с другой встречей: "+joinEvents(conflicts, 3, "; ")+". Укажите другое время или ответьте «да», чтобы сохранить пересечение.", now, "conflict", conflicts), nil
		}
		if e.Recurrence != nil {
			next, err := e.Recurrence.Next(now)
			if err != nil {
				return domain.Response{}, err
			}
			e.SetOccurrence(next)
		}
		event, err := tx.Create(ctx, e, now)
		if err != nil {
			return domain.Response{}, err
		}
		state.LastIDs = []int64{event.ID}
		label, saved := "Задача", "сохранена"
		if kind == domain.EventMeeting {
			label = "Встреча"
		} else if kind == domain.EventReminder {
			label, saved = "Напоминание", "сохранено"
		}
		suffix := ""
		if cmd.Date != nil && cmd.Date.Before(domain.Day(now).Time) {
			suffix = " Указанная дата уже прошла."
		}
		result := answer(label+" "+saved+": "+describe(*event)+"."+suffix, domain.ActionCreate, *event)
		if reply := relativeReminderReply(cmd, now); reply != "" {
			result.Reply = reply
		}
		if event.HasReminder() && event.Date != nil && event.StartTime != nil && (s.AskReminderSettings || event.ReminderMinutes != nil) {
			trigger := event.Date.At(*event.StartTime, s.Calendar.Location).Add(-time.Duration(event.ReminderLead()) * time.Minute)
			result.Reply += " Напомню " + trigger.Format("02.01 в 15:04") + "."
		}
		result.Conflicts = conflicts
		return result, nil
	}
	if cmd.Action == domain.ActionList || cmd.Action == domain.ActionSearch {
		f := domain.Filter{Kind: cmd.Kind, Start: cmd.PeriodStart, End: cmd.PeriodEnd}
		intro := ""
		var searchStatus *domain.SearchStatus
		if cmd.Action == domain.ActionSearch && cmd.Query == "" {
			return answer("По какой теме искать? Например: «Найди задачу про презентацию».", domain.ActionClarify), nil
		}
		if cmd.Action == domain.ActionList {
			f.Status = domain.StatusOpen
			if cmd.Filter != nil && cmd.Filter.Status != nil {
				f.Status = *cmd.Filter.Status
			}
			if f.Start == nil {
				f.Upcoming = domain.Day(now)
			}
		}
		events, err := tx.Events(ctx, f)
		if err != nil {
			return domain.Response{}, err
		}
		// Query is handled by the established search engine below; structured
		// constraints are deterministic and never recovered from message text.
		if cmd.Filter != nil {
			filter := *cmd.Filter
			filter.Query = ""
			events = s.filterEvents(events, &filter)
		}
		if cmd.Action == domain.ActionSearch {
			events, searchStatus = s.Calendar.Search.SearchContext(ctx, events, cmd.Query, len(events))
			if cmd.Next {
				events = nextScheduled(events, now, s.Calendar.Location)
			}
			intro = "Нашёл записи"
			if len(events) == 0 {
				intro = "Подходящих записей не нашёл. Попробуйте другое имя или тему."
			}
		} else {
			if cmd.Next {
				events = nextScheduled(events, now, s.Calendar.Location)
			}
			intro = fmt.Sprintf("Запланировано событий: %d", len(events))
			if len(events) == 0 {
				intro = "На этот период событий нет."
				if period := readPeriodLabel(cmd, now); period == "Сегодня" || period == "Завтра" {
					intro = period + " ничего не запланировано."
				}
			}
		}
		if len(events) == 0 && cmd.Provider == "simple_llm" && cmd.Kind == domain.EventTask && (cmd.Filter == nil || !cmd.Filter.StrictKind) {
			wide := f
			wide.Kind = domain.EventUnknown
			others, e := tx.Events(ctx, wide)
			if e != nil {
				return domain.Response{}, e
			}
			if cmd.Filter != nil {
				filter := *cmd.Filter
				filter.Kind, filter.Query = domain.EventUnknown, ""
				others = s.filterEvents(others, &filter)
			}
			if cmd.Action == domain.ActionSearch {
				others, searchStatus = s.Calendar.Search.SearchContext(ctx, others, cmd.Query, len(others))
			}
			if cmd.Next {
				others = nextScheduled(others, now, s.Calendar.Location)
			}
			if len(others) > 0 {
				events = others
				intro = "Задач по запросу нет. Есть другие подходящие дела:"
			}
		}
		state.LastIDs = []int64{}
		for i, e := range events {
			if i >= 30 {
				break
			}
			state.LastIDs = append(state.LastIDs, e.ID)
		}
		if len(events) > 0 {
			if intro == "Нашёл записи" {
				intro = "Вот что нашёл:"
			}
			if strings.HasPrefix(intro, "Запланировано событий:") {
				intro = fmt.Sprintf("%s — %d %s:", readPeriodLabel(cmd, now), len(events), readKindLabel(cmd.Kind, len(events)))
			}
			for i, event := range events {
				if i >= 100 {
					break
				}
				intro += "\n" + relativeDescription(event, now)
			}
		}
		if len(events) > 100 {
			intro += fmt.Sprintf("\nЕщё %d. Уточните период, чтобы увидеть остальные дела.", len(events)-100)
		}
		total := len(events)
		if len(events) > 100 {
			events = events[:100]
		}
		result := answer(intro, cmd.Action, events...)
		result.Search = searchStatus
		if searchStatus != nil && searchStatus.Warning != "" {
			result.Reply += "\n" + searchStatus.Warning
		}
		result.Total = &total
		return result, nil
	}
	candidates, certain, err := s.mutationCandidates(ctx, tx, state, cmd)
	if err != nil {
		return domain.Response{}, err
	}
	if len(candidates) == 0 {
		return answer("Не нашёл подходящую запись. Укажите название и, если нужно, дату: например, «удали напоминание позвонить маме завтра».", domain.ActionClarify), nil
	}
	if !certain {
		return askChoice(state, cmd, candidates, false, now), nil
	}
	if cmd.Action == domain.ActionDelete && cmd.Selector != nil && cmd.Selector.All {
		if len(candidates) > maxMutationPreview {
			return answer(fmt.Sprintf("Найдено записей: %d. Уточните период или название, чтобы проверить и удалить не больше %d записей за один раз.", len(candidates), maxMutationPreview), domain.ActionClarify), nil
		}
		return askDeletion(state, cmd, candidates, now), nil
	}
	if len(candidates) > 1 {
		return askChoice(state, cmd, candidates, true, now), nil
	}
	event := candidates[0]
	id := event.ID
	cmd.TargetID = &id
	if cmd.Action == domain.ActionRestore {
		if event.Status == domain.StatusOpen {
			conflicts, err := s.Calendar.Conflicts(ctx, tx, event, id)
			if err != nil {
				return domain.Response{}, err
			}
			if len(conflicts) > 0 && !force {
				return ask(state, cmd, "Восстановление пересекается с встречей: "+joinEvents(conflicts, 3, "; ")+". Ответьте «да», чтобы восстановить пересечение.", now, "conflict", conflicts), nil
			}
		}
		e, err := tx.Restore(ctx, id, now)
		if err != nil {
			return domain.Response{}, err
		}
		if !force {
			conflicts, e := s.Calendar.RestoredChildrenConflicts(ctx, tx, id)
			if e != nil {
				return domain.Response{}, e
			}
			if len(conflicts) > 0 {
				return domain.Response{}, &domain.AppError{Code: "meeting_conflict", Message: "Отдельное повторение серии пересекается с другой встречей", Status: 409, Conflicts: conflicts}
			}
		}
		if e.Recurrence != nil && e.Status == domain.StatusOpen {
			if err := calendar.Resume(e, now); err != nil {
				return domain.Response{}, err
			}
			e, err = tx.Update(ctx, *e, now)
			if err != nil {
				return domain.Response{}, err
			}
		}
		state.LastIDs = []int64{id}
		return answer("Восстановлено: "+describe(*e)+".", domain.ActionRestore, *e), nil
	}
	if cmd.Action == domain.ActionDelete {
		if ctx.Value(atomicRunKey{}) == true {
			if err := tx.Delete(ctx, id); err != nil {
				return domain.Response{}, err
			}
			return answer("Будет удалено: "+describe(event)+".", domain.ActionDelete, event), nil
		}
		return askDeletion(state, cmd, candidates, now), nil
	}
	if cmd.Action == domain.ActionComplete {
		event.Status = domain.StatusDone
		event.LastNotificationAt = nil
		e, err := tx.Update(ctx, event, now)
		if err != nil {
			return domain.Response{}, err
		}
		state.LastIDs = []int64{id}
		return answer("Отмечено выполненным: "+describe(*e)+".", domain.ActionComplete, *e), nil
	}
	if cmd.Action != domain.ActionUpdate {
		return answer("Уточните действие. Ничего не изменено.", domain.ActionClarify), nil
	}
	var series *domain.Event
	if cmd.Changes != nil && cmd.Changes.Scope == "occurrence" {
		parent, child, err := s.Calendar.PrepareOccurrence(ctx, tx, event, cmd.Changes.OccurrenceDate, cmd.Changes.OccurrenceAt)
		if err != nil {
			return answer(err.Error(), domain.ActionClarify), nil
		}
		series, event = &parent, child
	}
	if cmd.Date == nil && cmd.Time == nil && strings.TrimSpace(cmd.Title) == "" && cmd.ReminderEnabled == nil && cmd.ReminderMinutes == nil && cmd.Changes == nil {
		return ask(state, cmd, "На какую дату или время перенести событие?", now, "fields", nil), nil
	}
	if cmd.Title != "" {
		event.Title = strings.TrimSpace(cmd.Title)
	}
	if cmd.ReminderEnabled != nil {
		event.ReminderEnabled = cmd.ReminderEnabled
	}
	if cmd.ReminderMinutes != nil {
		event.ReminderMinutes = cmd.ReminderMinutes
	}
	if cmd.Date != nil || cmd.Time != nil {
		event.OccurrenceAt = cmd.ExactAt
	}
	if cmd.Date != nil {
		event.Date = cmd.Date
		if err := calendar.MoveSeriesDate(&event); err != nil {
			return answer(err.Error(), domain.ActionClarify), nil
		}
	}
	if cmd.Time != nil {
		event.StartTime = cmd.Time
		event.StartSecond = cmd.StartSecond
	}
	if err := applyExecutionChanges(&event, cmd.Changes, s.Calendar.Location); err != nil {
		return answer(err.Error(), domain.ActionClarify), nil
	}
	if cmd.Changes != nil && cmd.Changes.Status != nil && *cmd.Changes.Status == domain.StatusDone {
		event.LastNotificationAt = nil
	}
	if event.StartTime != nil && event.Date == nil {
		return ask(state, cmd, "Уточните дату переноса.", now, "fields", nil), nil
	}
	if event.Recurrence != nil && (cmd.Date != nil || cmd.Time != nil || cmd.Changes != nil && (cmd.Changes.Recurrence != nil || cmd.Changes.TimeShiftMinutes != nil)) {
		if err := calendar.Rebase(&event, now, s.Calendar.Location); err != nil {
			return answer(err.Error(), domain.ActionClarify), nil
		}
	}
	if err := event.Validate(); err != nil {
		return answer(err.Error(), domain.ActionClarify), nil
	}
	conflicts, err := s.Calendar.Conflicts(ctx, tx, event, event.ID)
	if err != nil {
		return domain.Response{}, err
	}
	if len(conflicts) > 0 && !force && ctx.Value(atomicRunKey{}) != true {
		return ask(state, cmd, "Перенос пересекается с встречей: "+joinEvents(conflicts, 3, "; ")+". Укажите другое время или ответьте «да».", now, "conflict", conflicts), nil
	}
	var e *domain.Event
	if series != nil {
		if _, err = tx.Update(ctx, *series, now); err != nil {
			return domain.Response{}, err
		}
		event.LastNotificationAt = nil
		if event.ID == 0 {
			e, err = tx.Create(ctx, event, now)
		} else {
			e, err = tx.Update(ctx, event, now)
		}
	} else {
		e, err = tx.Update(ctx, event, now)
	}
	if err != nil {
		return domain.Response{}, err
	}
	state.LastIDs = []int64{e.ID}
	label := "Перенесено: "
	if cmd.Changes != nil && cmd.Changes.Status != nil {
		label = "Вернул в работу: "
		if *cmd.Changes.Status == domain.StatusDone {
			label = "Отмечено выполненным: "
		}
	}
	if cmd.Title != "" {
		label = "Изменено: "
	}
	result := answer(label+describe(*e)+".", domain.ActionUpdate, *e)
	if series != nil {
		result.Reply = "Изменено одно повторение: " + describe(*e) + ". Остальная серия сохранена."
		if e.Status == domain.StatusDone {
			result.Reply = "Отменено одно повторение: " + describe(*e) + ". Остальная серия сохранена."
		}
	}
	result.Conflicts = conflicts
	return result, nil
}
func hasNewDirective(text string) bool {
	return nlp.HasDirective(text)
}
func (s *Service) History(ctx context.Context, id string) ([]domain.Exchange, domain.State, error) {
	var history []domain.Exchange
	var state domain.State
	err := s.Store.Read(ctx, func(tx repository.Tx) error {
		var err error
		history, err = tx.History(ctx, id, 50)
		if err != nil {
			return err
		}
		state, err = tx.State(ctx, id)
		return err
	})
	if err == nil {
		responses := make([]*domain.Response, len(history))
		for i := range history {
			responses[i] = &history[i].Response
			if err = s.RefreshZoomCards(ctx, &history[i].Response); err != nil {
				break
			}
		}
		if err == nil {
			s.refreshExecutionStatus(ctx, responses)
		}
	}
	return history, state, err
}
