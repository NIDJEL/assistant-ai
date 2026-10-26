package assistant

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
)

func actionOp(intent, args string) nlu.Operation {
	return nlu.Operation{Intent: intent, Arguments: json.RawMessage(args), Confidence: "high"}
}
func multiFixture(p *scriptedOperations, steps ...nlu.ActionStep) {
	raw, _ := json.Marshal(nlu.MultiAction{Steps: steps})
	op := actionOp("multi_action", string(raw))
	data, _ := json.Marshal(op)
	p.raw = string(data)
}
func actionStep(key, source, intent, args string, deps ...string) nlu.ActionStep {
	return nlu.ActionStep{Key: key, Source: source, DependsOn: deps, Operation: actionOp(intent, args)}
}
func resumeFixture(p *scriptedOperations, key, intent, args string) {
	raw, _ := json.Marshal(nlu.ResumeAction{Key: key, Operation: actionOp(intent, args)})
	operationFixture(p, "resume_action", string(raw))
}

func TestMultiScreenshotKeepsAllThreeActionsThroughClarifications(t *testing.T) {
	s, p, now := simpleService(t)
	now = time.Date(2026, 10, 26, 4, 53, 0, 0, now.Location())
	message := "Создай встречу с Колей в 8, добавь задачу купить продукты и напомни позвонить маме в 10"
	multiFixture(p,
		actionStep("meeting", "Создай встречу с Колей в 8", "create_meeting", `{"title":"Встреча с Колей","time":"20:00"}`),
		actionStep("task", "добавь задачу купить продукты", "create_task", `{"title":"Купить продукты"}`),
		actionStep("reminder", "напомни позвонить маме в 10", "create_reminder", `{"title":"Позвонить маме","date":{"type":"relative_day","offset":0},"time":"10:00"}`))
	r := say(t, s, message, now)
	if !r.Pending || len(r.Operations) != 3 || len(readAll(t, s)) != 1 || r.Operations[1].Status != "done" || !strings.Contains(r.Reply, "08:00 или 20:00") {
		t.Fatal(r)
	}
	// Reconstruct the service: all unfinished steps must come from persisted state.
	s = New(s.Store, p, s.Calendar)
	operationFixture(p, "continue", `{"changes":{"time":"20:00"}}`)
	r = say(t, s, "20:00", now.Add(time.Second))
	if !r.Pending || len(readAll(t, s)) != 1 || !strings.Contains(r.Reply, "дату") {
		t.Fatal(r)
	}
	operationFixture(p, "continue", `{"changes":{"date":{"type":"relative_day","offset":0}}}`)
	r = say(t, s, "сегодня", now.Add(2*time.Second))
	if !r.Pending || len(readAll(t, s)) != 2 || r.Operations[0].Status != "done" || !strings.Contains(r.Reply, "10:00 или 22:00") {
		t.Fatal(r)
	}
	if len(p.contexts[len(p.contexts)-1].Actions) != 3 {
		t.Fatal("plan omitted from compact context")
	}
	operationFixture(p, "continue", `{"changes":{"time":"10:00"}}`)
	r = say(t, s, "10:00", now.Add(3*time.Second))
	if r.Pending || len(readAll(t, s)) != 3 || r.Operations[2].Status != "done" {
		t.Fatal(r)
	}
	for _, e := range readAll(t, s) {
		if e.Kind == domain.EventMeeting && e.StartTime.String() != "20:00" {
			t.Fatal(e)
		}
	}
}

func TestMultiIndependentInvalidStepCanBeCorrectedWithoutReplay(t *testing.T) {
	s, p, now := simpleService(t)
	multiFixture(p,
		actionStep("bad", "неверный срок", "create_reminder", `{"title":"Звонок","date":{"type":"absolute","date":"2026-02-30"},"time":"20:00"}`),
		actionStep("good", "купить хлеб", "create_task", `{"title":"Купить хлеб"}`),
		actionStep("unsafe", "некорректный ответ", "delete", `{"target":{"type":"last","id":123}}`))
	r := say(t, s, "неверный срок; купить хлеб; некорректный ответ", now)
	if !r.Pending || len(readAll(t, s)) != 1 || r.Operations[0].Status != "failed" || r.Operations[2].Status != "failed" {
		t.Fatal(r)
	}
	resumeFixture(p, "bad", "create_reminder", `{"title":"Звонок","date":{"type":"relative_day","offset":1},"time":"20:00"}`)
	r = say(t, s, "Исправь срок первого на завтра 20:00", now.Add(time.Second))
	if !r.Pending || len(readAll(t, s)) != 2 || r.Operations[0].Status != "done" {
		t.Fatal(r)
	}
	r = say(t, s, "отмена", now.Add(2*time.Second))
	if r.Pending || len(readAll(t, s)) != 2 {
		t.Fatal(r)
	}
}

func TestMultiDependentActionWaitsAndUsesItsOwnResult(t *testing.T) {
	s, p, now := simpleService(t)
	multiFixture(p,
		actionStep("a", "Создай задачу", "create_task", `{"title":""}`),
		actionStep("b", "потом заверши её", "complete", `{"target":{"type":"step","value":"a"}}`, "a"),
		actionStep("c", "и купи хлеб", "create_task", `{"title":"Купить хлеб"}`))
	r := say(t, s, "Создай задачу, потом заверши её, и купи хлеб", now)
	if !r.Pending || len(readAll(t, s)) != 1 || r.Operations[1].Status != "blocked" {
		t.Fatal(r)
	}
	operationFixture(p, "continue", `{"changes":{"title":"Отчёт"}}`)
	r = say(t, s, "Отчёт", now.Add(time.Second))
	if r.Pending || len(readAll(t, s)) != 2 || r.Operations[1].Status != "done" {
		t.Fatal(r)
	}
	for _, e := range readAll(t, s) {
		if (e.Title == "Отчёт") != (e.Status == domain.StatusDone) {
			t.Fatal(e)
		}
	}
}

func TestMultiEachDeletionRequiresSeparateConfirmation(t *testing.T) {
	s, p, now := simpleService(t)
	seedPlanEvent(t, s, "Купить хлеб", "", "", domain.EventTask, now)
	seedPlanEvent(t, s, "Отчёт", "", "", domain.EventTask, now)
	multiFixture(p,
		actionStep("a", "удали хлеб", "delete", `{"target":{"type":"query","value":"Купить хлеб"}}`),
		actionStep("b", "удали отчёт", "delete", `{"target":{"type":"query","value":"Отчёт"}}`))
	r := say(t, s, "удали хлеб; удали отчёт", now)
	if !r.Pending || len(readAll(t, s)) != 2 {
		t.Fatal(r)
	}
	r = say(t, s, "да", now.Add(time.Second))
	if !r.Pending || len(readAll(t, s)) != 1 || r.Operations[1].Status != "waiting" {
		t.Fatal(r)
	}
	r = say(t, s, "да", now.Add(2*time.Second))
	if r.Pending || len(readAll(t, s)) != 0 {
		t.Fatal(r)
	}
}

func TestMultiIdempotencyConcurrentRetry(t *testing.T) {
	s, p, now := simpleService(t)
	multiFixture(p, actionStep("a", "Хлеб", "create_task", `{"title":"Хлеб"}`), actionStep("b", "Отчёт", "create_task", `{"title":"Отчёт"}`))
	var wg sync.WaitGroup
	for range 6 {
		wg.Go(func() {
			r, err := s.Respond(t.Context(), "Хлеб; Отчёт", "dialogue", "same-request", now)
			if err != nil || len(r.Events) != 2 {
				t.Errorf("%+v %v", r, err)
			}
		})
	}
	wg.Wait()
	if p.calls != 1 || len(readAll(t, s)) != 2 {
		t.Fatal(p.calls, readAll(t, s))
	}
	if _, err := s.Respond(t.Context(), "другой текст", "dialogue", "same-request", now); err == nil {
		t.Fatal("request id reused")
	}
}

func TestMultiParseFailureRetainsEveryPendingStep(t *testing.T) {
	s, p, now := simpleService(t)
	multiFixture(p, actionStep("a", "Задача", "create_task", `{"title":""}`), actionStep("b", "Напоминание", "create_reminder", `{"title":"Позвонить"}`))
	say(t, s, "Задача; Напоминание", now)
	p.raw = `{"intent":`
	r := say(t, s, "???", now.Add(time.Second))
	if !r.Pending {
		t.Fatal(r)
	}
	operationFixture(p, "continue", `{"changes":{"title":"Отчёт"}}`)
	r = say(t, s, "Отчёт", now.Add(2*time.Second))
	if !r.Pending || len(readAll(t, s)) != 1 || len(r.Operations) != 2 {
		t.Fatal(r)
	}
}

func TestSimpleAmbiguousClockNeverCreatesSilently(t *testing.T) {
	for _, text := range []string{"Встреча завтра в 8", "Встреча завтра в восемь", "Встреча завтра в 10"} {
		t.Run(text, func(t *testing.T) {
			s, p, now := simpleService(t)
			operationFixture(p, "create_meeting", `{"title":"Обсуждение","date":{"type":"relative_day","offset":1},"time":"20:00"}`)
			r := say(t, s, text, now)
			if !r.Pending || len(readAll(t, s)) != 0 || len(r.Suggestions) != 2 {
				t.Fatal(r)
			}
			operationFixture(p, "continue", `{"changes":{"time":"08:00"}}`)
			r = say(t, s, "08:00", now.Add(time.Second))
			if r.Pending || len(r.Events) != 1 || r.Events[0].StartTime.String() != "08:00" {
				t.Fatal(r)
			}
		})
	}
}

func TestMultiPersistenceFailureRollsBackAllEffectsAndCanRetry(t *testing.T) {
	s, p, now := simpleService(t)
	multiFixture(p, actionStep("a", "Хлеб", "create_task", `{"title":"Хлеб"}`), actionStep("b", "Отчёт", "create_task", `{"title":"Отчёт"}`))
	base := s.Store
	s.Store = failSimpleStore{base}
	if _, err := s.Respond(t.Context(), "Хлеб; Отчёт", "dialogue", "retry", now); err == nil {
		t.Fatal("expected persistence failure")
	}
	s.Store = base
	if len(readAll(t, s)) != 0 {
		t.Fatal("partial commit")
	}
	r, err := s.Respond(t.Context(), "Хлеб; Отчёт", "dialogue", "retry", now)
	if err != nil || r.Pending || len(readAll(t, s)) != 2 {
		t.Fatal(r, err)
	}
}

func TestMultiFrozenDelayRetainsSeconds(t *testing.T) {
	s, p, now := simpleService(t)
	now = now.Add(27 * time.Second)
	multiFixture(p, actionStep("a", "Таймер на 10 секунд", "create_reminder", `{"title":"Пицца","date":{"type":"relative_delay","unit":"second","value":10}}`), actionStep("b", "Хлеб", "create_task", `{"title":"Хлеб"}`))
	r := say(t, s, "Таймер на 10 секунд; Хлеб", now)
	if r.Pending || len(r.Events) != 2 {
		t.Fatal(r)
	}
	for _, e := range r.Events {
		if e.Kind == domain.EventReminder && (e.OccurrenceAt == nil || !e.OccurrenceAt.Equal(now.Add(10*time.Second))) {
			t.Fatal(e)
		}
	}
}

func TestMultiCannotEmbedConfirmation(t *testing.T) {
	s, p, now := simpleService(t)
	seedPlanEvent(t, s, "Отчёт", "", "", domain.EventTask, now)
	multiFixture(p, actionStep("a", "удали отчёт", "delete", `{"target":{"type":"query","value":"Отчёт"}}`), actionStep("b", "подтверди", "confirm", `{}`))
	r := say(t, s, "удали отчёт и подтверди", now)
	if !r.Pending || len(readAll(t, s)) != 1 || r.Operations[0].Status != "waiting" || r.Operations[1].Status != "failed" {
		t.Fatal(r)
	}
	p.raw = `{broken`
	r = say(t, s, "неразобранная поправка", now.Add(time.Second))
	if !r.Pending {
		t.Fatal(r)
	}
	operationFixture(p, "confirm", `{}`)
	r = say(t, s, "да", now.Add(2*time.Second))
	if len(readAll(t, s)) != 1 {
		t.Fatal("invalidated preview was confirmed", r)
	}
}

func TestSimpleDurationIsNotAmbiguousClock(t *testing.T) {
	s, p, now := simpleService(t)
	operationFixture(p, "create_task", `{"title":"Разминка","duration_minutes":5}`)
	r := say(t, s, "Добавь задачу сделать разминку на 5 минут", now)
	if r.Pending || len(readAll(t, s)) != 1 {
		t.Fatal(r)
	}
}

func TestMultiSyncsCreatedEventWhilePendingButNotOnLaterSearch(t *testing.T) {
	s, p, now := simpleService(t)
	external := &cloudFixture{}
	s.Integrations = external
	multiFixture(p,
		actionStep("a", "Встреча", "create_meeting", `{"title":"План","date":{"type":"relative_day","offset":1},"time":"17:00","calendar":"google"}`),
		actionStep("b", "Задача", "create_task", `{"title":""}`),
		actionStep("c", "Поиск", "search", `{"kind":"meeting","query":"План"}`, "b"))
	r := say(t, s, "Встреча; Задача; Поиск", now)
	if !r.Pending || external.syncs != 1 {
		t.Fatal(r, external.syncs)
	}
	operationFixture(p, "continue", `{"changes":{"title":"Отчёт"}}`)
	r = say(t, s, "Отчёт", now.Add(time.Second))
	if r.Pending || external.syncs != 1 {
		t.Fatal("a later read retriggered external mutation", r, external.syncs)
	}
}

func TestMultiIndependentReferenceDoesNotDriftToNewlyCreatedRecord(t *testing.T) {
	s, p, now := simpleService(t)
	original := seedPlanEvent(t, s, "Изначальная", "", "", domain.EventTask, now)
	operationFixture(p, "search", `{"kind":"task","query":"Изначальная"}`)
	say(t, s, "Найди изначальную", now)
	multiFixture(p, actionStep("a", "Новая", "create_task", `{"title":"Новая"}`), actionStep("b", "заверши её", "complete", `{"target":{"type":"last"}}`))
	r := say(t, s, "Новая; заверши её", now.Add(time.Second))
	if r.Pending {
		t.Fatal(r)
	}
	for _, e := range readAll(t, s) {
		if (e.ID == original.ID) != (e.Status == domain.StatusDone) {
			t.Fatal(e)
		}
	}
}
