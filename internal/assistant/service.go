package assistant

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
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
	Store       repository.Store
	Interpreter Interpreter
	Calendar    *calendar.Service
	mu          sync.Mutex
	locks       map[string]*lockEntry
}

func New(store repository.Store, interpreter Interpreter, cal *calendar.Service) *Service {
	return &Service{Store: store, Interpreter: interpreter, Calendar: cal, locks: map[string]*lockEntry{}}
}
func (s *Service) sessionLock(ctx context.Context, id string) (func(), error) {
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
	var response domain.Response
	unlock, err := s.sessionLock(ctx, sid)
	if err != nil {
		return response, err
	}
	defer unlock()
	var cached *domain.Exchange
	err = s.Store.Read(ctx, func(tx repository.Tx) error { var err error; cached, err = tx.Cached(ctx, sid, rid); return err })
	if err != nil {
		return response, err
	}
	if cached != nil {
		if cached.Message != message {
			return response, domain.Validation("request_id уже использован для другого сообщения")
		}
		return cached.Response, nil
	}
	// Both deterministic interpretation and external LLM HTTP happen before BEGIN IMMEDIATE.
	cmd, err := s.Interpreter.Interpret(ctx, message, now)
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
		if state.Pending != nil && now.Sub(state.Pending.At) > 30*time.Minute {
			state.Pending = nil
		}
		provider := cmd.Provider
		response, err = s.execute(ctx, tx, &state, cmd, message, now)
		if err != nil {
			return err
		}
		response.SessionID, response.RequestID, response.Provider, response.Timezone, response.Pending = sid, rid, provider, s.Calendar.Location.String(), state.Pending != nil
		if err = tx.SaveState(ctx, sid, state, now); err != nil {
			return err
		}
		return tx.SaveExchange(ctx, sid, rid, message, response, now)
	})
	return response, err
}

const help = "Я помогу с задачами, встречами и напоминаниями. Например: «Напомни завтра в 10 утра позвонить Ивану», «В пятницу нужно отправить отчёт», «Что у меня на эту неделю?» или «Найди задачу по договору»."

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
	return result
}

var digits = regexp.MustCompile(`^#?\d+$`)
var affirmative = regexp.MustCompile(`^да(?:[, ]+(?:пожалуйста|конечно|удали|сохрани))?$`)

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
	if in(low, "отмена", "отменить", "не надо", "нет", "стоп") {
		state.Pending = nil
		return answer("Действие отменено.", domain.ActionCancel), nil
	}
	force := false
	if p := state.Pending; p != nil {
		previous := p.Command
		yes := in(low, "да", "подтверждаю", "да все равно", "все равно", "сохранить", "удалить", "да удали") || affirmative.MatchString(low)
		switch {
		case in(p.Expectation, "delete", "conflict") && yes:
			cmd = previous
			force = true
			state.Pending = nil
		case p.Expectation == "choose" && (digits.MatchString(low) || cmd.TargetID != nil):
			id := int64(0)
			if cmd.TargetID != nil {
				id = *cmd.TargetID
			} else {
				id, _ = strconv.ParseInt(strings.TrimPrefix(low, "#"), 10, 64)
			}
			found := false
			for _, e := range p.Candidates {
				if e.ID == id {
					found = true
					break
				}
			}
			if !found {
				result := answer("Укажите номер записи из предложенного списка, например #42.", domain.ActionClarify)
				result.Candidates = p.Candidates
				return result, nil
			}
			cmd = previous
			cmd.TargetID = &id
			state.Pending = nil
		case in(p.Expectation, "fields", "conflict") && (cmd.Action == domain.ActionHelp || (p.Expectation == "fields" && previous.Title == "" && cmd.Action == domain.ActionCreate && !hasNewDirective(low))):
			t := nlp.Extract(message, now)
			if t.Error != "" {
				return answer(t.Error, domain.ActionClarify), nil
			}
			cmd = previous
			if cmd.Title == "" && p.Expectation == "fields" && t.Date == nil && t.Time == nil {
				cmd.Title = strings.TrimSpace(message)
			}
			if t.Date != nil {
				cmd.Date = t.Date
			}
			if t.Time != nil {
				cmd.Time = t.Time
			}
			state.Pending = nil
		case cmd.Action != domain.ActionHelp:
			state.Pending = nil
		default:
			result := answer("Ответьте на уточнение или напишите «отмена». Чтобы сменить действие, сформулируйте новый запрос.", domain.ActionClarify)
			result.Candidates = p.Candidates
			return result, nil
		}
	}
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
	if cmd.Action == domain.ActionCreate {
		if cmd.Title == "" || in(strings.ToLower(cmd.Title), "встреча", "задача", "напоминание") {
			cmd.Title = ""
			return ask(state, cmd, "Что нужно сделать или с кем провести встречу?", now, "fields", nil), nil
		}
		if utf8.RuneCountInString(cmd.Title) > 500 {
			return answer("Сократите название до 500 символов.", domain.ActionClarify), nil
		}
		if cmd.PeriodStart != nil && cmd.Date == nil {
			return ask(state, cmd, "На какой день в указанном периоде поставить событие?", now, "fields", nil), nil
		}
		if cmd.Kind == domain.EventMeeting || cmd.Kind == domain.EventReminder {
			if cmd.Date == nil {
				return ask(state, cmd, "На какую дату запланировать событие?", now, "fields", nil), nil
			}
			if cmd.Time == nil {
				return ask(state, cmd, "На какое время поставить событие? Например, в 15:00.", now, "fields", nil), nil
			}
		}
		if cmd.Time != nil && cmd.Date == nil {
			return ask(state, cmd, "На какую дату поставить задачу с указанным временем?", now, "fields", nil), nil
		}
		kind := cmd.Kind
		if kind == domain.EventUnknown {
			kind = domain.EventTask
		}
		e := domain.Event{Kind: kind, Title: cmd.Title, Date: cmd.Date, StartTime: cmd.Time, Duration: cmd.Duration, Status: domain.StatusOpen, Priority: domain.PriorityNormal}
		conflicts, err := s.Calendar.Conflicts(ctx, tx, e, 0)
		if err != nil {
			return domain.Response{}, err
		}
		if len(conflicts) > 0 && !force {
			return ask(state, cmd, "Это время пересекается с другой встречей: "+joinEvents(conflicts, 3, "; ")+". Укажите другое время или ответьте «да», чтобы сохранить пересечение.", now, "conflict", conflicts), nil
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
		result.Conflicts = conflicts
		return result, nil
	}
	if cmd.Action == domain.ActionList || cmd.Action == domain.ActionSearch {
		f := domain.Filter{Kind: cmd.Kind, Start: cmd.PeriodStart, End: cmd.PeriodEnd}
		intro := ""
		if cmd.Action == domain.ActionSearch && cmd.Query == "" {
			return answer("По какой теме искать? Например: «Найди задачу про презентацию».", domain.ActionClarify), nil
		}
		if cmd.Action == domain.ActionList {
			f.Status = domain.StatusOpen
			if f.Start == nil {
				f.Upcoming = domain.Day(now)
			}
		}
		events, err := tx.Events(ctx, f)
		if err != nil {
			return domain.Response{}, err
		}
		if cmd.Action == domain.ActionSearch {
			events = s.Calendar.Search.Search(events, cmd.Query, len(events))
			intro = "Нашёл записи"
			if len(events) == 0 {
				intro = "Подходящих записей не нашёл. Попробуйте другое имя или тему."
			}
		} else {
			intro = fmt.Sprintf("Запланировано событий: %d", len(events))
			if len(events) == 0 {
				intro = "На этот период событий нет."
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
			intro += "\n" + joinEvents(events, 8, "\n")
		}
		if len(events) > 8 {
			intro += fmt.Sprintf("\nЕщё %d — в списке результатов.", min(len(events), 100)-8)
		}
		if len(events) > 100 {
			intro += "\nПоказаны первые 100 записей. Уточните период или тему для остальных."
		}
		total := len(events)
		if len(events) > 100 {
			events = events[:100]
		}
		result := answer(intro, cmd.Action, events...)
		result.Total = &total
		return result, nil
	}
	candidates := []domain.Event{}
	if cmd.TargetID != nil {
		e, err := tx.Get(ctx, *cmd.TargetID)
		if err != nil {
			return domain.Response{}, err
		}
		if e != nil {
			candidates = append(candidates, *e)
		}
	} else if cmd.Query != "" && !in(strings.ToLower(cmd.Query), "встречу", "встреча", "задачу") {
		pool, err := tx.Events(ctx, domain.Filter{Kind: cmd.Kind})
		if err != nil {
			return domain.Response{}, err
		}
		candidates = s.Calendar.Search.Search(pool, cmd.Query, 10)
	} else {
		for _, id := range state.LastIDs {
			e, err := tx.Get(ctx, id)
			if err != nil {
				return domain.Response{}, err
			}
			if e != nil {
				candidates = append(candidates, *e)
			}
		}
	}
	if len(candidates) == 0 {
		return answer("Не нашёл запись для этого действия. Укажите название или номер, например #42.", domain.ActionClarify), nil
	}
	if len(candidates) > 1 {
		exact := []domain.Event{}
		for _, e := range candidates {
			if strings.EqualFold(e.Title, cmd.Query) {
				exact = append(exact, e)
			}
		}
		if len(exact) == 1 {
			candidates = exact
		} else {
			if len(candidates) > 10 {
				candidates = candidates[:10]
			}
			return ask(state, cmd, "Уточните номер записи:\n"+joinEvents(candidates, 10, "\n"), now, "choose", candidates), nil
		}
	}
	event := candidates[0]
	id := event.ID
	cmd.TargetID = &id
	if cmd.Action == domain.ActionDelete {
		if !force {
			return ask(state, cmd, "Удалить "+describe(event)+"? Ответьте «да» или «нет».", now, "delete", nil), nil
		}
		if err := tx.Delete(ctx, id); err != nil {
			return domain.Response{}, err
		}
		state.LastIDs = []int64{}
		return answer("Удалено: "+describe(event)+".", domain.ActionDelete), nil
	}
	if cmd.Action == domain.ActionComplete {
		event.Status = domain.StatusDone
		e, err := tx.Update(ctx, event, now)
		if err != nil {
			return domain.Response{}, err
		}
		state.LastIDs = []int64{id}
		return answer("Отмечено выполненным: "+describe(*e)+".", domain.ActionComplete, *e), nil
	}
	if cmd.Date == nil && cmd.Time == nil {
		return ask(state, cmd, "На какую дату или время перенести событие?", now, "fields", nil), nil
	}
	if cmd.Date != nil {
		event.Date = cmd.Date
	}
	if cmd.Time != nil {
		event.StartTime = cmd.Time
	}
	if event.StartTime != nil && event.Date == nil {
		return ask(state, cmd, "Уточните дату переноса.", now, "fields", nil), nil
	}
	conflicts, err := s.Calendar.Conflicts(ctx, tx, event, id)
	if err != nil {
		return domain.Response{}, err
	}
	if len(conflicts) > 0 && !force {
		return ask(state, cmd, "Перенос пересекается с встречей: "+joinEvents(conflicts, 3, "; ")+". Укажите другое время или ответьте «да».", now, "conflict", conflicts), nil
	}
	e, err := tx.Update(ctx, event, now)
	if err != nil {
		return domain.Response{}, err
	}
	state.LastIDs = []int64{id}
	result := answer("Перенесено: "+describe(*e)+".", domain.ActionUpdate, *e)
	result.Conflicts = conflicts
	return result, nil
}
func hasNewDirective(text string) bool {
	for _, word := range strings.FieldsFunc(text, func(r rune) bool { return r == ' ' || r == ',' || r == '.' || r == '!' }) {
		if in(word, "напомни", "добавь", "создай", "поставь", "запиши") {
			return true
		}
	}
	return false
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
	return history, state, err
}
