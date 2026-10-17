package assistant

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

type scriptedPlans struct {
	plans    map[string]nlp.SemanticPlan
	calls    int
	texts    []string
	contexts []nlp.DialogueContext
}

func (p *scriptedPlans) Interpret(context.Context, string, time.Time) (domain.Command, error) {
	return domain.Command{}, errors.New("legacy interpretation must not run")
}
func (p *scriptedPlans) ParsePlan(_ context.Context, text string, d nlp.DialogueContext, _ time.Time) (nlp.SemanticPlan, error) {
	p.calls++
	p.texts = append(p.texts, text)
	p.contexts = append(p.contexts, d)
	v, ok := p.plans[text]
	if !ok {
		return nlp.SemanticPlan{}, errors.New("no fixture")
	}
	return v, nil
}
func integer(n int) *int { return &n }
func planService(t *testing.T) (*Service, *scriptedPlans, time.Time) {
	s, _, now := dialogueService(t)
	p := &scriptedPlans{plans: map[string]nlp.SemanticPlan{}}
	s.Interpreter = p
	return s, p, now
}
func simplePlan(op string) nlp.SemanticPlan {
	return nlp.SemanticPlan{Status: "ok", DialogueAct: "command", Operation: pointer(op)}
}
func period(unit string, n int) *nlp.TemporalMeaning {
	return &nlp.TemporalMeaning{Type: "calendar_period", Unit: pointer(unit), Offset: integer(n)}
}
func readAll(t *testing.T, s *Service) []domain.Event {
	t.Helper()
	var events []domain.Event
	if err := s.Store.Read(t.Context(), func(tx repository.Tx) error {
		var err error
		events, err = tx.Events(t.Context(), domain.Filter{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return events
}
func seedPlanEvent(t *testing.T, s *Service, title, date, clock string, kind domain.EventType, now time.Time) domain.Event {
	t.Helper()
	e := domain.Event{Kind: kind, Title: title, Duration: time.Hour, Status: domain.StatusOpen, Priority: domain.PriorityNormal}
	if date != "" {
		e.Date, _ = domain.ParseDate(date)
	}
	if clock != "" {
		e.StartTime, _ = domain.ParseTime(clock)
	}
	if err := s.Store.Write(t.Context(), func(tx repository.Tx) error {
		v, err := tx.Create(t.Context(), e, now)
		if v != nil {
			e = *v
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestPlanRouterExactWhitelist(t *testing.T) {
	now := time.Now()
	p := &domain.Pending{At: now, Expectation: "delete"}
	for _, tc := range []struct {
		message string
		pending *domain.Pending
		fast    bool
		reason  string
	}{
		{"да", p, true, "confirmation"}, {"нет", p, true, "cancellation"}, {"давай лучше завтра", p, false, "no_exact_fast_path"},
		{"через 10 минут", &domain.Pending{At: now, Expectation: "fields", Command: domain.Command{Action: domain.ActionCreate, Kind: domain.EventReminder, Title: "Проверить чай"}}, true, "slot_relative_time_or_date"},
		{"через 10 минут и удали задачи", p, false, "no_exact_fast_path"},
		{"Найди встречи с Алисой за прошлый месяц", nil, false, "no_exact_fast_path"},
		{"Покажи только встречи после обеда на следующей неделе", nil, false, "no_exact_fast_path"},
		{"Вторую", &domain.Pending{At: now, Expectation: "choose", Candidates: make([]domain.Event, 3)}, true, "ordinal"},
		{"Вторую перенеси на следующий четверг", &domain.Pending{At: now, Expectation: "choose", Candidates: make([]domain.Event, 3)}, false, "no_exact_fast_path"},
		{"Найди встречи с Алисой", nil, false, "no_exact_fast_path"},
		{"Напомни завтра в 10 позвонить Ивану", nil, false, "no_exact_fast_path"},
	} {
		r := Route(domain.State{Pending: tc.pending}, tc.message, now)
		if r.FastPathMatched != tc.fast || r.Reason != tc.reason {
			t.Fatalf("%q: %+v", tc.message, r)
		}
	}
}

func TestPlanOriginalMessageAndRefinement(t *testing.T) {
	s, p, now := planService(t)
	seedPlanEvent(t, s, "Встреча с Алисой", "2026-09-20", "10:00", domain.EventMeeting, now)
	seedPlanEvent(t, s, "Встреча с Алисой", "2026-10-20", "10:00", domain.EventMeeting, now)
	text := "Пожалуйста, найди мои встречи с Алисой, хочу проверить календарь"
	first := simplePlan("search")
	first.Kind = pointer("meeting")
	first.Query = pointer("Алисой")
	p.plans[text] = first
	r := say(t, s, text, now)
	if r.Action != domain.ActionSearch || len(r.Events) != 2 {
		t.Fatal(r)
	}
	ref := simplePlan("search")
	ref.DialogueAct = "refinement"
	ref.Filters = &nlp.PlanFilters{Period: period("month", -1)}
	p.plans["А только за прошлый месяц?"] = ref
	r = say(t, s, "А только за прошлый месяц?", now.Add(time.Second))
	if r.Action != domain.ActionSearch || len(r.Events) != 1 || r.Events[0].Date.String() != "2026-09-20" {
		t.Fatal(r)
	}
	if p.texts[0] != text || p.contexts[1].Query != "Алисой" || p.contexts[1].LastKind != "meeting" {
		t.Fatal(p.texts, p.contexts)
	}
}

func TestBatchPlanPreviewAndDurableConfirmation(t *testing.T) {
	for _, op := range []string{"batch_update", "batch_delete", "batch_complete"} {
		t.Run(op, func(t *testing.T) {
			s, p, now := planService(t)
			seedPlanEvent(t, s, "Встреча с Алисой", "2026-10-19", "10:00", domain.EventMeeting, now)
			seedPlanEvent(t, s, "Встреча с Матвеем", "2026-10-19", "13:00", domain.EventMeeting, now)
			plan := simplePlan(op)
			plan.Kind = pointer("meeting")
			plan.Filters = &nlp.PlanFilters{Period: period("week", 1), Exclude: []nlp.PlanExclusion{{Query: pointer("Матвеем")}}}
			if op == "batch_update" {
				plan.Changes = &nlp.PlanChanges{TimeShiftMinutes: integer(60)}
			}
			text := "Все встречи следующей недели, кроме встречи с Матвеем, сдвинь на час позже"
			plan.Evidence = []nlp.PlanEvidence{{Field: "batch.scope", Text: "Все встречи"}, {Field: "filters.period", Text: "следующей недели"}, {Field: "filters.exclude.0.query", Text: "Матвеем"}}
			if op == "batch_update" {
				plan.Evidence = append(plan.Evidence, nlp.PlanEvidence{Field: "changes.time_shift_minutes", Text: "на час позже"})
			}
			p.plans[text] = plan
			r := say(t, s, text, now)
			if r.Action != domain.ActionClarify || !r.Pending || r.Total == nil || *r.Total != 1 {
				t.Fatal(r)
			}
			before := readAll(t, s)
			if len(before) != 2 || before[0].StartTime.String() != "10:00" {
				t.Fatal(before)
			}
			// Recreate the service to prove snapshots/changes survive persistence.
			s2 := New(s.Store, p, s.Calendar)
			r = say(t, s2, "да", now.Add(time.Second))
			if r.Action.String() != op[len("batch_"):] || r.Pending || p.calls != 1 {
				t.Fatal(r, p.calls)
			}
			after := readAll(t, s)
			switch op {
			case "batch_update":
				if after[0].StartTime.String() != "11:00" || after[1].StartTime.String() != "13:00" {
					t.Fatal(after)
				}
			case "batch_delete":
				if len(after) != 1 || after[0].Title != "Встреча с Матвеем" {
					t.Fatal(after)
				}
			case "batch_complete":
				if after[0].Status != domain.StatusDone || after[1].Status != domain.StatusOpen {
					t.Fatal(after)
				}
			}
		})
	}
}

func TestBatchRejectsStaleRecordsAndNewConflicts(t *testing.T) {
	for _, newConflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "stale", true: "new conflict"}[newConflict], func(t *testing.T) {
			s, p, now := planService(t)
			e := seedPlanEvent(t, s, "Созвон", "2026-10-19", "10:00", domain.EventMeeting, now)
			v := simplePlan("batch_update")
			v.Kind = pointer("meeting")
			v.Changes = &nlp.PlanChanges{TimeShiftMinutes: integer(60)}
			v.Evidence = []nlp.PlanEvidence{{Field: "batch.scope", Text: "все встречи"}, {Field: "changes.time_shift_minutes", Text: "на час"}}
			p.plans["сдвинь все встречи на час"] = v
			say(t, s, "сдвинь все встречи на час", now)
			if newConflict {
				seedPlanEvent(t, s, "Новое пересечение", "2026-10-19", "11:00", domain.EventMeeting, now)
			} else {
				e.Title = "Изменено параллельно"
				s.Store.Write(t.Context(), func(tx repository.Tx) error { _, err := tx.Update(t.Context(), e, now); return err })
			}
			r := say(t, s, "да", now.Add(time.Second))
			if r.Action != domain.ActionClarify || r.Pending {
				t.Fatal(r)
			}
			for _, x := range readAll(t, s) {
				if x.ID == e.ID && x.StartTime.String() != "10:00" {
					t.Fatal(x)
				}
			}
		})
	}
}

func TestPlanSlotsReferencesAndFailClosed(t *testing.T) {
	s, p, now := planService(t)
	create := simplePlan("create")
	create.Kind = pointer("reminder")
	create.Title = pointer("Проверить духовку")
	p.plans["напомни мне проверить духовку"] = create
	r := say(t, s, "напомни мне проверить духовку", now)
	if !r.Pending {
		t.Fatal(r)
	}
	r = say(t, s, "через 10 минут", now.Add(time.Second))
	if r.Action != domain.ActionCreate || p.calls != 1 || r.Events[0].OccurrenceAt == nil {
		t.Fatal(r, p.calls)
	}
	move := simplePlan("update")
	move.Target = &nlp.PlanTarget{Reference: pointer("ordinal"), Ordinal: integer(1)}
	move.Changes = &nlp.PlanChanges{Time: &nlp.ClockMeaning{Hour: 16}}
	move.Evidence = []nlp.PlanEvidence{{Field: "target.reference", Text: "первую"}, {Field: "changes.time", Text: "16"}}
	p.plans["ту первую на 16 передвинь"] = move
	r = say(t, s, "ту первую на 16 передвинь", now.Add(2*time.Second))
	if r.Action != domain.ActionUpdate || r.Events[0].StartTime.String() != "16:00" {
		t.Fatal(r)
	}
	invalid := simplePlan("delete")
	p.plans["сломанная модель"] = invalid
	r = say(t, s, "сломанная модель", now.Add(3*time.Second))
	if r.Action != domain.ActionClarify || len(readAll(t, s)) != 1 {
		t.Fatal(r)
	}
}

type failingBatchStore struct{ repository.Store }
type failingBatchTx struct {
	repository.Tx
	updates int
}

func (s failingBatchStore) Write(ctx context.Context, f func(repository.Tx) error) error {
	return s.Store.Write(ctx, func(tx repository.Tx) error { return f(&failingBatchTx{Tx: tx}) })
}
func (t *failingBatchTx) Update(ctx context.Context, e domain.Event, now time.Time) (*domain.Event, error) {
	t.updates++
	if t.updates == 2 {
		return nil, errors.New("injected write failure")
	}
	return t.Tx.Update(ctx, e, now)
}

func TestBatchConfirmationIsAtomic(t *testing.T) {
	s, p, now := planService(t)
	seedPlanEvent(t, s, "Первая", "2026-10-20", "09:00", domain.EventMeeting, now)
	seedPlanEvent(t, s, "Вторая", "2026-10-20", "12:00", domain.EventMeeting, now)
	v := simplePlan("batch_update")
	v.Kind = pointer("meeting")
	v.Changes = &nlp.PlanChanges{TimeShiftMinutes: integer(60)}
	v.Evidence = []nlp.PlanEvidence{{Field: "batch.scope", Text: "встречи"}, {Field: "changes.time_shift_minutes", Text: "на час"}}
	p.plans["сдвинь встречи на час"] = v
	say(t, s, "сдвинь встречи на час", now)
	s.Store = failingBatchStore{Store: s.Store}
	if _, err := s.Respond(t.Context(), "да", "dialogue", "confirm-with-failure", now.Add(time.Second)); err == nil {
		t.Fatal("injected failure was swallowed")
	}
	events := readAll(t, s)
	if events[0].StartTime.String() != "09:00" || events[1].StartTime.String() != "12:00" {
		t.Fatal("partial batch committed", events)
	}
}

func TestBatchSnapshotDoesNotIncludeNewMatches(t *testing.T) {
	s, p, now := planService(t)
	seedPlanEvent(t, s, "Исходная", "2026-10-20", "09:00", domain.EventMeeting, now)
	v := simplePlan("batch_delete")
	v.Kind = pointer("meeting")
	v.Evidence = []nlp.PlanEvidence{{Field: "batch.scope", Text: "встречи"}}
	p.plans["удали встречи"] = v
	say(t, s, "удали встречи", now)
	seedPlanEvent(t, s, "Новая", "2026-10-20", "12:00", domain.EventMeeting, now)
	r := say(t, s, "да", now.Add(time.Second))
	events := readAll(t, s)
	if r.Action != domain.ActionDelete || len(events) != 1 || events[0].Title != "Новая" {
		t.Fatal(r, events)
	}
}

func TestBatchCancellationAndAmbiguousConsentDoNotMutate(t *testing.T) {
	s, p, now := planService(t)
	seedPlanEvent(t, s, "Запись", "2026-10-20", "09:00", domain.EventMeeting, now)
	v := simplePlan("batch_complete")
	v.Kind = pointer("meeting")
	v.Evidence = []nlp.PlanEvidence{{Field: "batch.scope", Text: "встречи"}}
	p.plans["заверши встречи"] = v
	say(t, s, "заверши встречи", now)
	r := say(t, s, "давай лучше завтра", now.Add(time.Second))
	if r.Action != domain.ActionClarify || !r.Pending || readAll(t, s)[0].Status != domain.StatusOpen {
		t.Fatal(r)
	}
	r = say(t, s, "нет", now.Add(2*time.Second))
	if r.Action != domain.ActionCancel || r.Pending || readAll(t, s)[0].Status != domain.StatusOpen {
		t.Fatal(r)
	}
}

func TestPlanCannotInventAnOrdinalFromContext(t *testing.T) {
	for _, text := range []string{"Пусть эта запись начнётся в 13:20", "Ту передвинь на 2 часа позже", "Сдвинь её во второй половине дня"} {
		t.Run(text, func(t *testing.T) {
			s, p, now := planService(t)
			seedPlanEvent(t, s, "Демо с Родионом", "2026-10-20", "09:00", domain.EventMeeting, now)
			seedPlanEvent(t, s, "Демо с Родионом", "2026-10-20", "12:00", domain.EventMeeting, now)
			find := simplePlan("search")
			find.Query = pointer("Родионом")
			p.plans["найди демо"] = find
			say(t, s, "найди демо", now)
			for _, position := range []int{1, 2} {
				v := simplePlan("update")
				v.Target = &nlp.PlanTarget{Reference: pointer("ordinal"), Ordinal: integer(position)}
				v.Changes = &nlp.PlanChanges{Time: &nlp.ClockMeaning{Hour: 19}}
				p.plans[text] = v
				r := say(t, s, text, now.Add(time.Duration(position)*time.Second))
				if r.Action != domain.ActionClarify || r.Pending {
					t.Fatal("invented ordinal executed", r)
				}
			}
			events := readAll(t, s)
			if events[0].StartTime.String() != "09:00" || events[1].StartTime.String() != "12:00" {
				t.Fatal("unexpected mutation", events)
			}
		})
	}
}
