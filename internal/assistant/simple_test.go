package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

type scriptedOperations struct {
	raw      string
	calls    int
	contexts []nlu.Context
	before   func()
}

func (p *scriptedOperations) Interpret(context.Context, string, time.Time) (domain.Command, error) {
	panic("legacy parser called")
}
func (p *scriptedOperations) InterpretOperation(_ context.Context, _ string, d nlu.Context) (nlu.Operation, error) {
	p.calls++
	p.contexts = append(p.contexts, d)
	if p.before != nil {
		p.before()
	}
	return nlu.Decode([]byte(p.raw))
}
func simpleService(t *testing.T) (*Service, *scriptedOperations, time.Time) {
	s, _, now := dialogueService(t)
	p := &scriptedOperations{}
	s.Interpreter = p
	return s, p, now
}
func operationFixture(p *scriptedOperations, intent, args string) {
	p.raw = `{"intent":"` + intent + `","arguments":` + args + `,"confidence":"high","clarification":null}`
}
func TestSimpleTaskDueDateWithoutTime(t *testing.T) {
	for _, tc := range []struct{ date, want string }{
		{`{"type":"day_of_month","day":20}`, "2026-10-20"},
		{`{"type":"relative_day","offset":1}`, "2026-10-17"},
		{`{"type":"weekday","weekday":"friday","direction":"next_or_current"}`, "2026-10-16"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			s, p, now := simpleService(t)
			operationFixture(p, "create_task", `{"title":"Подготовить смету","date":`+tc.date+`,"time":null}`)
			r := say(t, s, "Мне нужно подготовить смету к сроку", now)
			if r.Action != domain.ActionCreate || r.Pending || len(r.Events) != 1 || r.Events[0].Kind != domain.EventTask || r.Events[0].Date.String() != tc.want || r.Events[0].StartTime != nil {
				t.Fatalf("%+v", r)
			}
		})
	}
}
func TestSimpleContextCorrectionDeletion(t *testing.T) {
	s, p, now := simpleService(t)
	first := seedPlanEvent(t, s, "Встреча с Алисой", "2026-10-17", "10:00", domain.EventMeeting, now)
	second := seedPlanEvent(t, s, "Встреча с Алисой", "2026-10-17", "14:00", domain.EventMeeting, now)
	operationFixture(p, "search", `{"kind":"meeting","query":"Алиса"}`)
	if r := say(t, s, "Найди встречи с Алисой", now); len(r.Events) != 2 {
		t.Fatal(r)
	}
	operationFixture(p, "update", `{"target":{"type":"ordinal","index":2},"changes":{"time":"16:00"}}`)
	if r := say(t, s, "вторую перенеси на 16", now.Add(time.Second)); len(r.Events) != 1 || r.Events[0].ID != second.ID {
		t.Fatal(r)
	}
	operationFixture(p, "update", `{"target":{"type":"last"},"changes":{"time":"17:00"}}`)
	if r := say(t, s, "нет, лучше на 17", now.Add(2*time.Second)); r.Action != domain.ActionUpdate || r.Events[0].StartTime.String() != "17:00" {
		t.Fatal(r)
	}
	compact, _ := json.Marshal(p.contexts[len(p.contexts)-1])
	if strings.Contains(string(compact), `"id"`) || strings.Contains(string(compact), `"target_id"`) || strings.Contains(string(compact), "last_user_message") {
		t.Fatal(string(compact))
	}
	if p.contexts[len(p.contexts)-1].LastAffected.Time != "16:00" || len(p.contexts[len(p.contexts)-1].LastResults) != 2 {
		t.Fatal(string(compact))
	}
	operationFixture(p, "delete", `{"target":{"type":"last"}}`)
	if r := say(t, s, "удали её", now.Add(3*time.Second)); !r.Pending || len(readAll(t, s)) != 2 {
		t.Fatal(r)
	}
	calls := p.calls
	if r := say(t, s, "да", now.Add(4*time.Second)); r.Action != domain.ActionDelete || p.calls != calls {
		t.Fatal(r)
	}
	events := readAll(t, s)
	if len(events) != 1 || events[0].ID != first.ID {
		t.Fatal(events)
	}
}
func TestSimpleMissingAndInvalidArguments(t *testing.T) {
	s, p, now := simpleService(t)
	operationFixture(p, "create_task", `{"title":"Отправить отчёт"}`)
	if r := say(t, s, "Создай задачу Отправить отчёт на завтра", now); !r.Pending || len(readAll(t, s)) != 0 {
		t.Fatal(r)
	}
	operationFixture(p, "continue", `{"changes":{"date":{"type":"relative_day","offset":1}}}`)
	if r := say(t, s, "завтра", now.Add(time.Second)); r.Action != domain.ActionCreate || r.Events[0].Date.String() != "2026-10-17" {
		t.Fatal(r)
	}
	for _, args := range []string{`{"title":"X","time":"25:00","date":{"type":"relative_day","offset":1}}`, `{"title":"X","date":{"type":"absolute","date":"2026-02-30"}}`, `{"title":"X","duration_minutes":9223372036854775807}`, `{"title":"X","id":481}`, `{"title":"X","reminder_minutes":-3}`} {
		operationFixture(p, "create_task", args)
		r := say(t, s, "поставь задачу "+args, now.Add(2*time.Second))
		if r.Action != domain.ActionClarify || len(readAll(t, s)) != 1 {
			t.Fatal(args, r)
		}
	}
}
func TestSimpleBatchPreviewStaleAndCancel(t *testing.T) {
	for _, mode := range []string{"confirm", "cancel", "stale", "invalid", "restart"} {
		t.Run(mode, func(t *testing.T) {
			s, p, now := simpleService(t)
			alice := seedPlanEvent(t, s, "Встреча с Алисой", "2026-10-17", "10:00", domain.EventMeeting, now)
			seedPlanEvent(t, s, "Встреча с Матвеем", "2026-10-17", "14:00", domain.EventMeeting, now)
			operationFixture(p, "batch_update", `{"filter":{"kind":"meeting","date":{"type":"relative_day","offset":1}},"exclude":[{"query":"Матвей"}],"changes":{"shift_minutes":60}}`)
			r := say(t, s, "все встречи завтра сдвинь на час кроме Матвея", now)
			if !r.Pending || r.Total == nil || *r.Total != 1 || readAll(t, s)[0].StartTime.String() != "10:00" {
				t.Fatal(r)
			}
			if mode == "stale" {
				err := s.Store.Write(t.Context(), func(tx repository.Tx) error {
					alice.Title = "Новое название"
					_, err := tx.Update(t.Context(), alice, now.Add(time.Second))
					return err
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			if mode == "invalid" {
				p.raw = `{"intent":"update","arguments":{"id":481},"confidence":"high"}`
				r = say(t, s, "другая непонятная команда", now.Add(time.Second))
				if r.Pending {
					t.Fatal(r)
				}
			}
			if mode == "restart" {
				s = New(s.Store, p, s.Calendar)
			}
			text := "да"
			if mode == "cancel" {
				text = "нет"
			}
			r = say(t, s, text, now.Add(2*time.Second))
			events := readAll(t, s)
			want := "10:00"
			if mode == "confirm" || mode == "restart" {
				want = "11:00"
			}
			if events[0].StartTime.String() != want || events[1].StartTime.String() != "14:00" || r.Pending {
				t.Fatal(mode, r, events)
			}
		})
	}
}
func TestSimpleInferenceRaceAndIdempotency(t *testing.T) {
	s, p, now := simpleService(t)
	operationFixture(p, "create_task", `{"title":"Отчёт"}`)
	p.before = func() {
		err := s.Store.Write(t.Context(), func(tx repository.Tx) error {
			return tx.SaveState(t.Context(), "dialogue", domain.State{Pending: &domain.Pending{At: now, Expectation: "fields", Command: neutral("simple_llm")}}, now)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if r := say(t, s, "создай отчёт", now); r.Action != domain.ActionClarify || len(readAll(t, s)) != 0 {
		t.Fatal(r)
	}
	p.before = nil
	r := say(t, s, "создай отчёт", now.Add(time.Second))
	calls := p.calls
	r2 := say(t, s, "создай отчёт", now.Add(time.Second))
	if r.Action != domain.ActionCreate || r2.Events[0].ID != r.Events[0].ID || p.calls != calls {
		t.Fatal(r, r2)
	}
}

type failSimpleStore struct{ repository.Store }
type failSimpleTx struct{ repository.Tx }

func (t failSimpleTx) SaveExchange(context.Context, string, string, string, domain.Response, time.Time) error {
	return errors.New("injected persistence failure")
}
func (s failSimpleStore) Write(ctx context.Context, fn func(repository.Tx) error) error {
	return s.Store.Write(ctx, func(tx repository.Tx) error { return fn(failSimpleTx{tx}) })
}
func TestSimpleRollbackIncludesMutationAndState(t *testing.T) {
	s, p, now := simpleService(t)
	base := s.Store
	s.Store = failSimpleStore{base}
	operationFixture(p, "create_task", `{"title":"Не сохранять при ошибке"}`)
	if _, err := s.Respond(t.Context(), "создай задачу", "rollback", "request", now); err == nil {
		t.Fatal("expected error")
	}
	s.Store = base
	if len(readAll(t, s)) != 0 {
		t.Fatal("mutation survived rollback")
	}
	base.Read(t.Context(), func(tx repository.Tx) error {
		state, err := tx.State(t.Context(), "rollback")
		if state.Dialogue != nil || state.Pending != nil {
			t.Fatal(state)
		}
		return err
	})
}

func TestSimpleChoiceCanRepeatOriginalOperation(t *testing.T) {
	for _, action := range []string{"update", "delete"} {
		t.Run(action, func(t *testing.T) {
			s, p, now := simpleService(t)
			seedPlanEvent(t, s, "Встреча с Алисой", "2026-10-17", "10:00", domain.EventMeeting, now)
			second := seedPlanEvent(t, s, "Встреча с Алисой", "2026-10-17", "14:00", domain.EventMeeting, now)
			args := `{"target":{"type":"query","value":"Алиса"}`
			if action == "update" {
				args += `,"changes":{"time":"17:00"}`
			}
			args += `}`
			operationFixture(p, action, args)
			if r := say(t, s, "исходная команда", now); !r.Pending || len(r.Candidates) != 2 {
				t.Fatal(r)
			}
			args = `{"target":{"type":"ordinal","index":2}`
			if action == "update" {
				args += `,"changes":{}`
			}
			args += `}`
			operationFixture(p, action, args)
			r := say(t, s, "вторую", now.Add(time.Second))
			if action == "update" {
				if r.Action != domain.ActionUpdate || r.Events[0].ID != second.ID || r.Events[0].StartTime.String() != "17:00" {
					t.Fatal(r)
				}
			} else {
				if !r.Pending || len(readAll(t, s)) != 2 {
					t.Fatal(r)
				}
			}
		})
	}
}

func TestSimpleLastAfterSearchUsesNewResult(t *testing.T) {
	s, p, now := simpleService(t)
	operationFixture(p, "create_task", `{"title":"Отчёт"}`)
	say(t, s, "создай отчёт", now)
	bread := seedPlanEvent(t, s, "Купить хлеб", "2026-10-17", "", domain.EventTask, now)
	operationFixture(p, "search", `{"query":"хлеб"}`)
	say(t, s, "найди хлеб", now.Add(time.Second))
	operationFixture(p, "complete", `{"target":{"type":"last"}}`)
	if r := say(t, s, "эту отметь выполненной", now.Add(2*time.Second)); r.Action != domain.ActionComplete || r.Events[0].ID != bread.ID {
		t.Fatal(r)
	}
}

func TestSimpleStaleConflictAndDeletion(t *testing.T) {
	for _, action := range []string{"update", "delete"} {
		t.Run(action, func(t *testing.T) {
			s, p, now := simpleService(t)
			target := seedPlanEvent(t, s, "Встреча с Алисой", "2026-10-17", "10:00", domain.EventMeeting, now)
			seedPlanEvent(t, s, "Встреча с Матвеем", "2026-10-17", "15:00", domain.EventMeeting, now)
			args := `{"target":{"type":"query","value":"Алиса"}`
			if action == "update" {
				args += `,"changes":{"time":"15:00"}`
			}
			args += `}`
			operationFixture(p, action, args)
			if r := say(t, s, "попросить действие", now); !r.Pending {
				t.Fatal(r)
			}
			if err := s.Store.Write(t.Context(), func(tx repository.Tx) error {
				target.Title = "Изменено другим клиентом"
				_, err := tx.Update(t.Context(), target, now.Add(time.Second))
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if r := say(t, s, "да", now.Add(2*time.Second)); r.Action != domain.ActionClarify || r.Pending || len(readAll(t, s)) != 2 {
				t.Fatal(r)
			}
			if readAll(t, s)[0].StartTime.String() != "10:00" {
				t.Fatal("stale conflict applied")
			}
		})
	}
}

type failSecondUpdateStore struct{ repository.Store }
type failSecondUpdateTx struct {
	repository.Tx
	updates int
}

func (t *failSecondUpdateTx) Update(ctx context.Context, e domain.Event, now time.Time) (*domain.Event, error) {
	t.updates++
	if t.updates == 2 {
		return nil, errors.New("injected second update failure")
	}
	return t.Tx.Update(ctx, e, now)
}
func (s failSecondUpdateStore) Write(ctx context.Context, fn func(repository.Tx) error) error {
	return s.Store.Write(ctx, func(tx repository.Tx) error { return fn(&failSecondUpdateTx{Tx: tx}) })
}
func TestSimpleBatchRollbackIsAtomic(t *testing.T) {
	s, p, now := simpleService(t)
	seedPlanEvent(t, s, "Встреча с Алисой", "2026-10-17", "10:00", domain.EventMeeting, now)
	seedPlanEvent(t, s, "Встреча с Матвеем", "2026-10-17", "15:00", domain.EventMeeting, now)
	operationFixture(p, "batch_update", `{"filter":{"kind":"meeting"},"changes":{"shift_minutes":60}}`)
	if r := say(t, s, "сдвинь все встречи на час", now); !r.Pending {
		t.Fatal(r)
	}
	base := s.Store
	s.Store = failSecondUpdateStore{base}
	if _, err := s.Respond(t.Context(), "да", "dialogue", "failed-confirm", now.Add(time.Second)); err == nil {
		t.Fatal("expected storage failure")
	}
	s.Store = base
	events := readAll(t, s)
	if events[0].StartTime.String() != "10:00" || events[1].StartTime.String() != "15:00" {
		t.Fatal("partial batch survived", events)
	}
	if r := say(t, s, "да", now.Add(2*time.Second)); r.Action != domain.ActionUpdate || r.Pending {
		t.Fatal("preview did not survive rollback", r)
	}
}

func TestSimpleLowConfidenceDoesNotMutate(t *testing.T) {
	s, p, now := simpleService(t)
	p.raw = `{"intent":"create_task","arguments":{"title":"Не уверен"},"confidence":"low","clarification":"Что создать?"}`
	if r := say(t, s, "непонятный запрос", now); r.Action != domain.ActionClarify || len(readAll(t, s)) != 0 {
		t.Fatal(r)
	}
}
