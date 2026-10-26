package assistant

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
)

func groupFixture(p *scriptedOperations, steps ...nlu.ActionStep) {
	for i := 0; i < len(steps)-1; i++ {
		steps[i].AtomicGroup = "g1"
	}
	multiFixture(p, steps...)
}

func TestAtomicGroupPreviewsAndCommitsTogetherWithIndependentProgress(t *testing.T) {
	s, p, now := simpleService(t)
	groupFixture(p, actionStep("a", "Отчёт", "create_task", `{"title":"Отчёт"}`), actionStep("b", "Хлеб", "create_task", `{"title":"Хлеб"}`), actionStep("c", "Молоко", "create_task", `{"title":"Молоко"}`))
	r := say(t, s, "Отчёт; Хлеб; Молоко", now)
	if !r.Pending || len(readAll(t, s)) != 1 || !r.Operations[0].CanConfirm || r.Operations[2].Status != "done" {
		t.Fatal(r)
	}
	r = say(t, s, "да", now.Add(time.Second))
	if r.Pending || len(readAll(t, s)) != 3 || r.Operations[0].Status != "done" || r.Operations[1].Status != "done" {
		t.Fatal(r)
	}
}

func TestAtomicGroupInvalidSiblingRollsBackAndCanBeRepaired(t *testing.T) {
	s, p, now := simpleService(t)
	groupFixture(p, actionStep("a", "Отчёт", "create_task", `{"title":"Отчёт"}`), actionStep("b", "Звонок", "create_reminder", `{"title":"Позвонить","date":{"type":"relative_day","offset":1},"time":"25:00"}`), actionStep("c", "Хлеб", "create_task", `{"title":"Хлеб"}`))
	r := say(t, s, "Отчёт; Звонок; Хлеб", now)
	if !r.Pending || len(readAll(t, s)) != 1 || r.Operations[0].Status == "done" {
		t.Fatal(r)
	}
	resumeFixture(p, "b", "create_reminder", `{"title":"Позвонить","date":{"type":"relative_day","offset":1},"time":"20:00"}`)
	r = say(t, s, "Исправь звонок на завтра 20:00", now.Add(time.Second))
	if !r.Pending || len(readAll(t, s)) != 1 || !r.Operations[0].CanConfirm {
		t.Fatal(r)
	}
	r = say(t, s, "да", now.Add(2*time.Second))
	if r.Pending || len(readAll(t, s)) != 3 {
		t.Fatal(r)
	}
}

func TestAtomicSnapshotChangeRequiresNewConfirmation(t *testing.T) {
	s, p, now := simpleService(t)
	groupFixture(p, actionStep("a", "Отчёт", "create_task", `{"title":"Отчёт"}`), actionStep("b", "Хлеб", "create_task", `{"title":"Хлеб"}`), actionStep("c", "Молоко", "create_task", `{"title":"Молоко"}`))
	say(t, s, "Отчёт; Хлеб; Молоко", now)
	seedPlanEvent(t, s, "Изменение другим клиентом", "", "", domain.EventTask, now)
	r := say(t, s, "да", now.Add(time.Second))
	if !r.Pending || len(readAll(t, s)) != 2 {
		t.Fatal(r)
	}
	r = say(t, s, "да", now.Add(2*time.Second))
	if r.Pending || len(readAll(t, s)) != 4 {
		t.Fatal(r)
	}
}

func TestAtomicDeleteIsOnlyPreviewedUntilGroupConfirmation(t *testing.T) {
	s, p, now := simpleService(t)
	seedPlanEvent(t, s, "Отчёт", "", "", domain.EventTask, now)
	groupFixture(p, actionStep("a", "Удали отчёт", "delete", `{"target":{"type":"query","value":"Отчёт"}}`), actionStep("b", "Хлеб", "create_task", `{"title":"Хлеб"}`), actionStep("c", "Молоко", "create_task", `{"title":"Молоко"}`))
	r := say(t, s, "Удали отчёт; Хлеб; Молоко", now)
	if !r.Pending || len(readAll(t, s)) != 2 {
		t.Fatal(r)
	}
	r = say(t, s, "да", now.Add(time.Second))
	if r.Pending || len(readAll(t, s)) != 2 {
		t.Fatal(r)
	}
	for _, e := range readAll(t, s) {
		if e.Title == "Отчёт" {
			t.Fatal("group deletion not applied")
		}
	}
}

func TestAtomicGroupChecksFinalStateAfterLaterDeletion(t *testing.T) {
	s, p, now := simpleService(t)
	seedPlanEvent(t, s, "Альфа", "2026-10-18", "10:00", domain.EventMeeting, now)
	seedPlanEvent(t, s, "Бета", "2026-10-18", "11:00", domain.EventMeeting, now)
	groupFixture(p, actionStep("a", "Перенеси Альфа", "update", `{"target":{"type":"query","value":"Альфа"},"changes":{"time":"11:00"}}`), actionStep("b", "Удали Бета", "delete", `{"target":{"type":"query","value":"Бета"}}`), actionStep("c", "Хлеб", "create_task", `{"title":"Хлеб"}`))
	r := say(t, s, "Перенеси Альфа; Удали Бета; Хлеб", now)
	if !r.Pending || !r.Operations[0].CanConfirm || len(readAll(t, s)) != 3 {
		t.Fatal(r)
	}
	r = say(t, s, "да", now.Add(time.Second))
	if r.Pending || len(readAll(t, s)) != 2 {
		t.Fatal(r)
	}
	for _, e := range readAll(t, s) {
		if e.Title == "Альфа" && e.StartTime.Hour != 11 || e.Title == "Бета" {
			t.Fatal(e)
		}
	}
}

func TestAtomicFinalConflictRollsBackGroupButKeepsIndependentTask(t *testing.T) {
	s, p, now := simpleService(t)
	groupFixture(p, actionStep("a", "Альфа", "create_meeting", `{"title":"Альфа","date":{"type":"relative_day","offset":1},"time":"11:00"}`), actionStep("b", "Бета", "create_meeting", `{"title":"Бета","date":{"type":"relative_day","offset":1},"time":"11:00"}`), actionStep("c", "Хлеб", "create_task", `{"title":"Хлеб"}`))
	r := say(t, s, "Альфа; Бета; Хлеб", now)
	if !r.Pending || len(readAll(t, s)) != 1 || r.Operations[0].CanConfirm || r.Operations[2].Status != "done" {
		t.Fatal(r)
	}
}

func TestAtomicRecurrencePreviewKeepsAcceptedTimeAcrossConfirmation(t *testing.T) {
	s, p, now := simpleService(t)
	groupFixture(p, actionStep("a", "Витамины", "create_reminder", `{"title":"Витамины","recurrence":{"frequency":"minute","interval":20,"count":3}}`), actionStep("b", "Хлеб", "create_task", `{"title":"Хлеб"}`), actionStep("c", "Молоко", "create_task", `{"title":"Молоко"}`))
	r := say(t, s, "Витамины; Хлеб; Молоко", now)
	if !r.Pending || !r.Operations[0].CanConfirm {
		t.Fatal(r)
	}
	r = say(t, s, "да", now.Add(2*time.Minute))
	if r.Pending {
		t.Fatal(r)
	}
	for _, e := range readAll(t, s) {
		if e.Title == "Витамины" {
			at, _, _ := e.Interval(now.Location())
			if !at.Equal(now.Add(20 * time.Minute)) {
				t.Fatal("preview schedule drifted", at)
			}
		}
	}
}

func TestActionButtonsFocusCancelAndRejectStaleToken(t *testing.T) {
	s, p, now := simpleService(t)
	multiFixture(p, actionStep("a", "Первая", "create_task", `{"title":""}`), actionStep("b", "Вторая", "create_task", `{"title":""}`))
	r := say(t, s, "Первая; Вторая", now)
	old := r.Operations[0].ControlToken
	ctx := WithChatChoice(t.Context(), &domain.ChatChoice{Token: old, Key: "b", Action: "action_focus"})
	r, err := s.Respond(ctx, "Уточнить второе", "dialogue", "focus", now.Add(time.Second))
	if err != nil || !r.Operations[1].Active {
		t.Fatal(r, err)
	}
	ctx = WithChatChoice(t.Context(), &domain.ChatChoice{Token: old, Key: "a", Action: "action_cancel"})
	stale, err := s.Respond(ctx, "Отменить первое", "dialogue", "stale", now.Add(2*time.Second))
	if err != nil || !stale.Pending {
		t.Fatal(stale, err)
	}
	operationFixture(p, "continue", `{"changes":{"title":"Второй отчёт"}}`)
	r = say(t, s, "Второй отчёт", now.Add(3*time.Second))
	if !r.Pending || len(readAll(t, s)) != 1 || r.Operations[1].Status != "done" {
		t.Fatal(r)
	}
}

func TestSimpleWeeklyRecurrenceAndRuleSurviveClarification(t *testing.T) {
	s, p, now := simpleService(t)
	operationFixture(p, "create_reminder", `{"title":"Проверить отчёт","recurrence":{"frequency":"week","weekdays":["monday"],"count":3}}`)
	r := say(t, s, "Каждый понедельник проверять отчёт три раза", now)
	if !r.Pending || len(readAll(t, s)) != 0 {
		t.Fatal(r)
	}
	operationFixture(p, "continue", `{"changes":{"time":"09:00"}}`)
	r = say(t, s, "09:00", now.Add(time.Second))
	if r.Pending || len(r.Events) != 1 || r.Events[0].Recurrence == nil || r.Events[0].Recurrence.Count != 3 || r.Events[0].Date.Weekday() != time.Monday {
		t.Fatal(r)
	}
}

func TestSimpleRecurrenceRenameKeepsAnchorAndCanStopRepeating(t *testing.T) {
	s, p, now := simpleService(t)
	operationFixture(p, "create_reminder", `{"title":"Разминка","recurrence":{"frequency":"hour","interval":2,"count":3}}`)
	r := say(t, s, "Каждые два часа напомни размяться три раза", now)
	if r.Pending || len(r.Events) != 1 {
		t.Fatal(r)
	}
	anchor := r.Events[0].Recurrence.Anchor
	operationFixture(p, "update", `{"target":{"type":"last"},"changes":{"title":"Встать"}}`)
	r = say(t, s, "Назови Встать", now.Add(time.Minute))
	if r.Pending || !r.Events[0].Recurrence.Anchor.Equal(anchor) {
		t.Fatal(r)
	}
	operationFixture(p, "update", `{"target":{"type":"last"},"changes":{"clear_recurrence":true}}`)
	r = say(t, s, "Отмени повторения", now.Add(2*time.Minute))
	if r.Pending || r.Events[0].Recurrence != nil {
		t.Fatal(r)
	}
}

func TestStateJSONIncludesBoundedDiagnostics(t *testing.T) {
	s, p, now := simpleService(t)
	operationFixture(p, "create_task", `{"title":"Отчёт"}`)
	r := say(t, s, "Отчёт", now)
	raw, _ := json.Marshal(r)
	if r.Diagnostics == nil || r.Diagnostics.StateBytes == 0 || len(raw) > 1<<20 {
		t.Fatal(r)
	}
}

func TestOnlyNextRecurrenceCanBeDetachedWithoutChangingSeriesAnchor(t *testing.T) {
	s, p, now := simpleService(t)
	operationFixture(p, "create_reminder", `{"title":"Витамины","time":"09:00","recurrence":{"frequency":"day","count":3}}`)
	r := say(t, s, "Витамины ежедневно в 09:00 три раза", now)
	if r.Pending || len(r.Events) != 1 {
		t.Fatal(r)
	}
	original := r.Events[0]
	anchor := original.Recurrence.Anchor
	operationFixture(p, "update", `{"target":{"type":"last"},"changes":{"scope":"occurrence","time":"10:00"}}`)
	r = say(t, s, "Только ближайший приём перенеси на 10:00", now.Add(time.Second))
	if r.Pending || len(r.Events) != 1 || r.Events[0].ID == original.ID || r.Events[0].Recurrence != nil || r.Events[0].StartTime.Hour != 10 {
		t.Fatal(r)
	}
	events := readAll(t, s)
	if len(events) != 2 {
		t.Fatal(events)
	}
	for _, e := range events {
		if e.ID == original.ID {
			if e.Recurrence == nil || !e.Recurrence.Anchor.Equal(anchor) || e.Date.String() == original.Date.String() || e.StartTime.Hour != 9 {
				t.Fatal(e)
			}
		}
	}
}

func TestRecurringClockInventedByModelIsWithheld(t *testing.T) {
	s, p, now := simpleService(t)
	operationFixture(p, "create_reminder", `{"title":"Делать зарядку","time":"09:00","recurrence":{"frequency":"week","weekdays":["monday"],"count":3}}`)
	r := say(t, s, "Напоминай делать зарядку по понедельникам три раза", now)
	if !r.Pending || len(readAll(t, s)) != 0 {
		t.Fatal(r)
	}
	operationFixture(p, "continue", `{"changes":{"time":"09:00"}}`)
	r = say(t, s, "В 09:00", now.Add(time.Second))
	if r.Pending || len(readAll(t, s)) != 1 || r.Events[0].Recurrence.Count != 3 {
		t.Fatal(r)
	}
}

func TestClockGuardKeepsDateWordsSeparateFromPartOfDay(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"через два дня в 8", "08:00"}, {"в половине восьмого", "07:30"}, {"полвосьмого", "07:30"}, {"завтра в 8 утра", ""}, {"завтра в 08:15", ""}, {"завтра вечером в восемь", ""}, {"таймер на 5 минут", ""},
	} {
		t.Run(tc.text, func(t *testing.T) {
			v := ambiguousHours(tc.text)
			if tc.want == "" && len(v) != 0 || tc.want != "" && (len(v) != 2 || v[0] != tc.want) {
				t.Fatal(v)
			}
		})
	}
}
