package contract

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/search"
)

func selectionEvent(t *testing.T, ctx context.Context, cal *calendar.Service, now time.Time, title string, kind domain.EventType, offset int, clock string) domain.Event {
	t.Helper()
	var day *domain.Date
	if offset >= 0 {
		day = domain.Day(now).AddDays(offset)
	}
	var at *domain.TimeOfDay
	if clock != "" {
		at, _ = domain.ParseTime(clock)
	}
	e, err := cal.Create(ctx, domain.Event{Kind: kind, Title: title, Date: day, StartTime: at}, now, true)
	if err != nil {
		t.Fatal(err)
	}
	return *e
}

func selectionMessage(t *testing.T, ctx context.Context, s *assistant.Service, now time.Time, sid, rid, text string) domain.Response {
	t.Helper()
	r, err := s.Respond(ctx, text, sid, rid, now)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestDeleteAllTomorrowUsesPreviewSnapshotAndSurvivesRestart(t *testing.T) {
	db, cal, now := services(t)
	ctx := context.Background()
	one := selectionEvent(t, ctx, cal, now, "Пара", domain.EventReminder, 1, "12:50")
	two := selectionEvent(t, ctx, cal, now, "Купить хлеб", domain.EventTask, 1, "")
	keep := selectionEvent(t, ctx, cal, now, "Пара", domain.EventReminder, 2, "12:50")
	selectionEvent(t, ctx, cal, now, "Без даты", domain.EventTask, -1, "")
	s := assistant.New(db, nlp.Hybrid{}, cal)
	r := selectionMessage(t, ctx, s, now, "bulk", "preview", "Удали пожалуйста все записи на завтра")
	if !r.Pending || r.Action != domain.ActionClarify || len(r.Events) != 2 || r.Total == nil || *r.Total != 2 {
		t.Fatalf("missing bounded preview: %+v", r)
	}
	if count, _ := cal.Count(ctx); count != 4 {
		t.Fatal("mutated before confirmation")
	}
	late := selectionEvent(t, ctx, cal, now, "Новая запись после вопроса", domain.EventTask, 1, "")
	s = assistant.New(db, nlp.Hybrid{}, cal)
	r = selectionMessage(t, ctx, s, now, "bulk", "confirm", "Да, удали все пожалуйста")
	if r.Action != domain.ActionDelete || r.Pending || r.Total == nil || *r.Total != 2 {
		t.Fatalf("confirmation failed: %+v", r)
	}
	for _, id := range []int64{one.ID, two.ID} {
		if _, err := cal.Get(ctx, id); err == nil {
			t.Fatal("previewed record remains", id)
		}
	}
	for _, id := range []int64{keep.ID, late.ID} {
		if _, err := cal.Get(ctx, id); err != nil {
			t.Fatal("deleted an unpreviewed record", id)
		}
	}
	r = selectionMessage(t, ctx, s, now, "bulk", "confirm", "Да, удали все пожалуйста")
	if r.Action != domain.ActionDelete {
		t.Fatal("duplicate request was not cached")
	}
	if count, _ := cal.Count(ctx); count != 3 {
		t.Fatal("duplicate confirmation deleted more records")
	}
}

func TestChangedDeletionPreviewIsAllOrNothing(t *testing.T) {
	for _, single := range []bool{false, true} {
		t.Run(fmt.Sprint(single), func(t *testing.T) {
			db, cal, now := services(t)
			ctx := context.Background()
			one := selectionEvent(t, ctx, cal, now, "Пара", domain.EventReminder, 1, "12:50")
			if !single {
				selectionEvent(t, ctx, cal, now, "Купить хлеб", domain.EventTask, 1, "")
			}
			s := assistant.New(db, nlp.Hybrid{}, cal)
			selectionMessage(t, ctx, s, now, "changed", "preview", "Удали все записи на завтра")
			newTitle := "Пара перенесена"
			// Deliberately use the same timestamp: UpdatedAt alone is insufficient.
			if _, err := cal.Update(ctx, one.ID, calendar.Patch{Title: &newTitle}, now); err != nil {
				t.Fatal(err)
			}
			r := selectionMessage(t, ctx, s, now, "changed", "yes", "да")
			if r.Action != domain.ActionClarify || r.Pending || !strings.Contains(r.Reply, "Ничего не удалено") {
				t.Fatalf("stale preview accepted: %+v", r)
			}
			want := 2
			if single {
				want = 1
			}
			if count, _ := cal.Count(ctx); count != want {
				t.Fatal("partial deletion", count)
			}
		})
	}
}

func TestNamedSelectionClarifiesByDateTimeAndOrdinal(t *testing.T) {
	for _, choice := range []string{"ту, что завтра в 15:00", "вторую", "Позвонить Оле"} {
		t.Run(choice, func(t *testing.T) {
			db, cal, now := services(t)
			ctx := context.Background()
			selectionEvent(t, ctx, cal, now, "Позвонить маме", domain.EventReminder, 0, "10:00")
			want := selectionEvent(t, ctx, cal, now, "Позвонить Оле", domain.EventReminder, 1, "15:00")
			s := assistant.New(db, nlp.Hybrid{}, cal)
			r := selectionMessage(t, ctx, s, now, "named", "start", "Удали напоминание позвонить")
			if !r.Pending || len(r.Candidates) != 2 {
				t.Fatalf("ambiguity not exposed: %+v", r)
			}
			s = assistant.New(db, nlp.Hybrid{}, cal)
			r = selectionMessage(t, ctx, s, now, "named", "choose", choice)
			if !r.Pending || !strings.Contains(r.Reply, want.Title) || !strings.HasPrefix(r.Reply, "Удалить") {
				t.Fatalf("natural choice rejected: %+v", r)
			}
			r = selectionMessage(t, ctx, s, now, "named", "yes", "да")
			if r.Action != domain.ActionDelete {
				t.Fatalf("deletion failed: %+v", r)
			}
			if _, err := cal.Get(ctx, want.ID); err == nil {
				t.Fatal("wrong record selected")
			}
			if count, _ := cal.Count(ctx); count != 1 {
				t.Fatal("deleted multiple records")
			}
		})
	}
}

func TestTitleAndDateScopeCannotFallBackToRecentRecord(t *testing.T) {
	db, cal, now := services(t)
	ctx := context.Background()
	want := selectionEvent(t, ctx, cal, now, "Позвонить маме", domain.EventReminder, 1, "15:00")
	keep := selectionEvent(t, ctx, cal, now, "Позвонить маме", domain.EventReminder, 2, "15:00")
	s := assistant.New(db, nlp.Hybrid{}, cal)
	selectionMessage(t, ctx, s, now, "scope", "list", "Покажи записи на послезавтра")
	r := selectionMessage(t, ctx, s, now, "scope", "delete", "Удали уведомление позвонить маме завтра в 15:00")
	if !r.Pending || !strings.Contains(r.Reply, fmt.Sprintf("#%d ", want.ID)) {
		t.Fatalf("wrong date scope: %+v", r)
	}
	selectionMessage(t, ctx, s, now, "scope", "yes", "да")
	if _, err := cal.Get(ctx, keep.ID); err != nil {
		t.Fatal("deleted the recent result instead")
	}
	r = selectionMessage(t, ctx, s, now, "scope", "missing", "Удали напоминание позвонить маме сегодня")
	if r.Pending || r.Action != domain.ActionClarify {
		t.Fatal("fell back from an empty date scope")
	}
}

func TestWeakMatchesNeverAutoCompleteOrMove(t *testing.T) {
	for _, action := range []string{"Заверши дело проверка договоров", "Перенеси дело проверка договоров на завтра", "Удали все дела проверка договоров"} {
		t.Run(action, func(t *testing.T) {
			db, cal, now := services(t)
			ctx := context.Background()
			e := selectionEvent(t, ctx, cal, now, "Проверка отчёта", domain.EventTask, 2, "")
			s := assistant.New(db, nlp.Hybrid{}, cal)
			r := selectionMessage(t, ctx, s, now, "weak", "start", action)
			if r.Action != domain.ActionClarify {
				t.Fatalf("weak match mutated: %+v", r)
			}
			current, _ := cal.Get(ctx, e.ID)
			if current == nil || current.Status != domain.StatusOpen || current.Date.String() != e.Date.String() {
				t.Fatal("record changed")
			}
		})
	}
}

func TestMoveDistinguishesSourceDateFromDestination(t *testing.T) {
	db, cal, now := services(t)
	ctx := context.Background()
	want := selectionEvent(t, ctx, cal, now, "Позвонить маме", domain.EventReminder, 1, "10:00")
	keep := selectionEvent(t, ctx, cal, now, "Позвонить маме", domain.EventReminder, 2, "10:00")
	s := assistant.New(db, nlp.Hybrid{}, cal)
	r := selectionMessage(t, ctx, s, now, "move", "start", "Перенеси завтрашнее напоминание позвонить маме на пятницу в 17:00")
	if r.Action != domain.ActionUpdate || len(r.Events) != 1 || r.Events[0].ID != want.ID || r.Events[0].Date.String() != "2026-10-16" || r.Events[0].StartTime.String() != "17:00" {
		t.Fatalf("source/destination mixed up: %+v", r)
	}
	current, _ := cal.Get(ctx, keep.ID)
	if current.Date.String() != keep.Date.String() {
		t.Fatal("moved another record")
	}
}

func TestBulkDeletionKindCancellationAndLimit(t *testing.T) {
	db, cal, now := services(t)
	ctx := context.Background()
	selectionEvent(t, ctx, cal, now, "Позвонить маме", domain.EventReminder, 1, "10:00")
	selectionEvent(t, ctx, cal, now, "Проверить почту", domain.EventTask, 1, "")
	s := assistant.New(db, nlp.Hybrid{}, cal)
	r := selectionMessage(t, ctx, s, now, "kind", "preview", "Удали все напоминания на завтра")
	if !r.Pending || !strings.Contains(r.Reply, "Позвонить маме") || strings.Contains(r.Reply, "Проверить почту") {
		t.Fatal("wrong type preview", r)
	}
	selectionMessage(t, ctx, s, now, "kind", "cancel", "нет")
	selectionMessage(t, ctx, s, now, "kind", "late-yes", "да")
	if count, _ := cal.Count(ctx); count != 2 {
		t.Fatal("cancelled deletion executed")
	}
	for i := range 100 {
		selectionEvent(t, ctx, cal, now, fmt.Sprintf("Задача %d", i), domain.EventTask, 1, "")
	}
	r = selectionMessage(t, ctx, s, now, "limit", "preview", "Удали все записи на завтра")
	if r.Pending || r.Action != domain.ActionClarify || !strings.Contains(r.Reply, "102") {
		t.Fatal("silently truncated deletion", r)
	}
}

func TestPostgresBulkDeletionIsAccountScopedAndDurable(t *testing.T) {
	db, auth, _, now := pgFixture(t)
	// Registration creates separate user scopes; names and dates may coincide.
	u, err := auth.Register(context.Background(), "Первый", "select-one@example.test", "test password 12345", "test password 12345")
	if err != nil {
		t.Fatal(err)
	}
	v, err := auth.Register(context.Background(), "Второй", "select-two@example.test", "test password 12345", "test password 12345")
	if err != nil {
		t.Fatal(err)
	}
	ctx := repository.WithUser(context.Background(), u.ID)
	other := repository.WithUser(context.Background(), v.ID)
	engine, _ := search.New()
	cal := &calendar.Service{Store: db, Search: engine, Location: now.Location()}
	one := selectionEvent(t, ctx, cal, now, "Позвонить маме", domain.EventReminder, 1, "10:00")
	two := selectionEvent(t, ctx, cal, now, "Пара", domain.EventTask, 1, "")
	keep := selectionEvent(t, other, cal, now, "Позвонить маме", domain.EventReminder, 1, "10:00")
	s := assistant.New(db, nlp.Hybrid{}, cal)
	r := selectionMessage(t, ctx, s, now, "pg-selection", "start", "Удали все записи на завтра")
	if !r.Pending || len(r.Events) != 2 {
		t.Fatal("wrong tenant preview", r)
	}
	s = assistant.New(db, nlp.Hybrid{}, cal)
	r = selectionMessage(t, ctx, s, now, "pg-selection", "yes", "да")
	if r.Action != domain.ActionDelete {
		t.Fatal("postgres snapshot not durable", r)
	}
	for _, id := range []int64{one.ID, two.ID} {
		if _, err := cal.Get(ctx, id); err == nil {
			t.Fatal("selected record remains")
		}
	}
	if _, err := cal.Get(other, keep.ID); err != nil {
		t.Fatal("another account changed", err)
	}
}

func TestChoiceCanRefineBeyondFirstTenDisplayedRecords(t *testing.T) {
	db, cal, now := services(t)
	ctx := context.Background()
	var want domain.Event
	for i := range 13 {
		want = selectionEvent(t, ctx, cal, now, "Проверить заявку", domain.EventTask, i, "")
	}
	s := assistant.New(db, nlp.Hybrid{}, cal)
	r := selectionMessage(t, ctx, s, now, "many", "start", "Удали дело проверить заявку")
	if len(r.Candidates) != 10 || !strings.Contains(r.Reply, "10 из 13") {
		t.Fatal("missing choice page", r)
	}
	s = assistant.New(db, nlp.Hybrid{}, cal)
	r = selectionMessage(t, ctx, s, now, "many", "date", "ту, что "+want.Date.Format("02.01.2006"))
	if !r.Pending || !strings.HasPrefix(r.Reply, fmt.Sprintf("Удалить #%d ", want.ID)) {
		t.Fatal("ignored records beyond the displayed page", r)
	}
	selectionMessage(t, ctx, s, now, "many", "yes", "да")
	if count, _ := cal.Count(ctx); count != 12 {
		t.Fatal("wrong deletion count", count)
	}
}

func TestExactSharedTitleDoesNotHideOtherMatchesInBulkSelection(t *testing.T) {
	db, cal, now := services(t)
	ctx := context.Background()
	selectionEvent(t, ctx, cal, now, "Отчёт", domain.EventTask, 1, "")
	selectionEvent(t, ctx, cal, now, "Налоговый отчёт", domain.EventTask, 1, "")
	s := assistant.New(db, nlp.Hybrid{}, cal)
	r := selectionMessage(t, ctx, s, now, "all-names", "one", "Удали дело отчет")
	if !r.Pending || len(r.Candidates) != 2 {
		t.Fatal("ambiguous partial name became an exact match", r)
	}
	selectionMessage(t, ctx, s, now, "all-names", "cancel", "отмена")
	r = selectionMessage(t, ctx, s, now, "all-names", "all", "Удали все дела по отчету завтра")
	if !r.Pending || len(r.Events) != 2 {
		t.Fatal("an exact title hid another bulk match", r)
	}
	selectionMessage(t, ctx, s, now, "all-names", "cancel-again", "отмена")
	r = selectionMessage(t, ctx, s, now, "all-names", "quoted", "Удали дело «Отчёт» завтра")
	if !r.Pending || len(r.Candidates) != 0 || !strings.HasPrefix(r.Reply, "Удалить #1 ") {
		t.Fatal("quoted exact name became a partial match", r)
	}
}

func TestNewQuestionDoesNotChooseOrMutatePendingCandidates(t *testing.T) {
	for _, directive := range []string{"Удали дело проверить заявку", "Перенеси дело проверить заявку на послезавтра", "Заверши дело проверить заявку"} {
		t.Run(directive, func(t *testing.T) {
			db, cal, now := services(t)
			ctx := context.Background()
			selectionEvent(t, ctx, cal, now, "Проверить заявку", domain.EventTask, 1, "")
			selectionEvent(t, ctx, cal, now, "Проверить заявку", domain.EventTask, 2, "")
			s := assistant.New(db, nlp.Hybrid{}, cal)
			r := selectionMessage(t, ctx, s, now, "new-question", "start", directive)
			if len(r.Candidates) != 2 || !r.Pending {
				t.Fatal("expected choice", r)
			}
			r = selectionMessage(t, ctx, s, now, "new-question", "question", "Что у меня завтра?")
			if r.Action != domain.ActionList || r.Pending || len(r.Events) != 1 || r.Events[0].Status != domain.StatusOpen {
				t.Fatal("question became a mutation", r)
			}
			selectionMessage(t, ctx, s, now, "new-question", "late-yes", "да")
			if count, _ := cal.Count(ctx); count != 2 {
				t.Fatal("an obsolete confirmation was executed")
			}
		})
	}
}
