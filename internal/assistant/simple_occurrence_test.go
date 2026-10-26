package assistant

import (
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

func TestNamedOccurrenceUsesItsDateInsteadOfTheMasterRowDate(t *testing.T) {
	for _, selector := range []string{
		`"target":{"type":"query","value":"Тест серия","filter":{"kind":"meeting","date":{"type":"relative_day","offset":2}}},"changes":{"scope":"occurrence","time":"11:00"}`,
		`"target":{"type":"query","value":"Тест серия"},"changes":{"scope":"occurrence","occurrence":{"type":"relative_day","offset":2},"time":"11:00"}`,
	} {
		t.Run(selector, func(t *testing.T) {
			s, p, now := simpleService(t)
			now = time.Date(2026, 10, 26, 15, 49, 0, 0, now.Location())
			operationFixture(p, "create_meeting", `{"title":"Тест серия","date":{"type":"relative_day","offset":1},"time":"10:00","duration_minutes":30,"recurrence":{"frequency":"day","count":4}}`)
			created := say(t, s, "Создай встречу «Тест серия» каждый день в 10:00 на 30 минут, начиная с завтра, всего четыре встречи.", now)
			if created.Pending || len(created.Events) != 1 {
				t.Fatal(created)
			}
			masterID := created.Events[0].ID
			operationFixture(p, "update", `{`+selector+`}`)
			moved := say(t, s, "Перенеси только встречу «Тест серия» послезавтра на 11:00. Остальные повторения оставь как есть.", now.Add(time.Second))
			if moved.Action != domain.ActionUpdate || moved.Pending || len(moved.Events) != 1 || moved.Events[0].Date.String() != "2026-10-28" || moved.Events[0].StartTime.String() != "11:00" {
				t.Fatal(moved)
			}
			child := moved.Events[0]
			if child.SeriesID == nil || *child.SeriesID != masterID || child.RecurrenceID == nil || child.RecurrenceID.In(now.Location()).Format("2006-01-02 15:04") != "2026-10-28 10:00" {
				t.Fatal(child)
			}
			// Last affected is now the detached child. An explicit title and date
			// must still resolve the same series, without creating an ambiguity.
			operationFixture(p, "update", `{"target":{"type":"query","value":"Тест серия","filter":{"kind":"meeting","date":{"type":"relative_day","offset":3}}},"changes":{"scope":"occurrence","status":"done"}}`)
			cancelled := say(t, s, "Отмени только встречу «Тест серия» через три дня. Остальные повторения сохрани.", now.Add(2*time.Second))
			if cancelled.Pending || len(cancelled.Events) != 1 || cancelled.Events[0].Date.String() != "2026-10-29" || cancelled.Events[0].StartTime.String() != "10:00" || cancelled.Events[0].Status != domain.StatusDone || !strings.HasPrefix(cancelled.Reply, "Отменено одно повторение") {
				t.Fatal(cancelled)
			}
			all := readAll(t, s)
			if len(all) != 3 {
				t.Fatal(all)
			}
			for _, e := range all {
				if e.ID != masterID {
					continue
				}
				if e.Date.String() != "2026-10-27" || e.StartTime.String() != "10:00" || e.Recurrence.Count != 4 || len(e.Recurrence.ExDates) != 2 {
					t.Fatal("unrelated occurrences changed", e)
				}
				for _, day := range []int{27, 30} {
					at := time.Date(2026, 10, day, 10, 0, 0, 0, now.Location())
					if !e.Recurrence.Contains(at) || e.Recurrence.Excluded(at) {
						t.Fatal("unrelated occurrence excluded", at, e)
					}
				}
			}
			operationFixture(p, "create_meeting", `{"title":"Тест пересечение","date":{"type":"relative_day","offset":2},"time":"11:00","duration_minutes":30}`)
			conflict := say(t, s, "Создай послезавтра в 11:00 встречу «Тест пересечение» на 30 минут.", now.Add(3*time.Second))
			if !conflict.Pending || len(conflict.Candidates) != 1 || conflict.Candidates[0].ID != child.ID || len(readAll(t, s)) != 3 {
				t.Fatal("moved occurrence conflict was not detected", conflict)
			}
		})
	}
}

func TestOccurrenceReferenceToDetachedChildKeepsTheSeries(t *testing.T) {
	s, p, now := simpleService(t)
	operationFixture(p, "create_meeting", `{"title":"Серия","date":{"type":"relative_day","offset":1},"time":"10:00","recurrence":{"frequency":"day","count":4}}`)
	created := say(t, s, "Создай Серию завтра в 10:00 каждый день четыре раза", now)
	operationFixture(p, "update", `{"target":{"type":"last"},"changes":{"scope":"occurrence","occurrence":{"type":"relative_day","offset":2},"time":"11:00"}}`)
	moved := say(t, s, "Перенеси только повторение послезавтра на 11:00", now.Add(time.Second))
	if len(moved.Events) != 1 {
		t.Fatal(moved)
	}
	operationFixture(p, "update", `{"target":{"type":"last"},"changes":{"scope":"occurrence","occurrence":{"type":"relative_day","offset":3},"status":"done"}}`)
	r := say(t, s, "В этой же серии отмени только повторение через три дня", now.Add(2*time.Second))
	if r.Pending || len(r.Events) != 1 || r.Events[0].SeriesID == nil || *r.Events[0].SeriesID != created.Events[0].ID || r.Events[0].Date.String() != domain.Day(now).AddDays(3).String() {
		t.Fatal(r)
	}
	// A short correction to that exact detached occurrence edits it in place.
	operationFixture(p, "update", `{"target":{"type":"last"},"changes":{"scope":"occurrence","status":"open"}}`)
	restored := say(t, s, "Верни это повторение", now.Add(3*time.Second))
	if restored.Pending || len(restored.Events) != 1 || restored.Events[0].ID != r.Events[0].ID || len(readAll(t, s)) != 3 {
		t.Fatal(restored)
	}
}

func TestOccurrenceConflictingSourceDatesDoNotMutate(t *testing.T) {
	s, p, now := simpleService(t)
	operationFixture(p, "create_meeting", `{"title":"Серия","date":{"type":"relative_day","offset":1},"time":"10:00","recurrence":{"frequency":"day","count":4}}`)
	say(t, s, "Создай Серию завтра в 10:00 каждый день четыре раза", now)
	operationFixture(p, "update", `{"target":{"type":"query","value":"Серия","filter":{"date":{"type":"relative_day","offset":2}}},"changes":{"scope":"occurrence","occurrence":{"type":"relative_day","offset":3},"time":"11:00"}}`)
	r := say(t, s, "Перенеси одно повторение", now.Add(time.Second))
	if r.Action != domain.ActionClarify || len(readAll(t, s)) != 1 || !strings.Contains(r.Reply, "разные даты") {
		t.Fatal(r)
	}
}

func TestSeparateNamedSeriesStillRequireAChoice(t *testing.T) {
	s, p, now := simpleService(t)
	for _, clock := range []string{"10:00", "12:00"} {
		operationFixture(p, "create_meeting", `{"title":"Серия","date":{"type":"relative_day","offset":1},"time":"`+clock+`","duration_minutes":30,"recurrence":{"frequency":"day","count":4}}`)
		say(t, s, "Создай Серию завтра в "+clock+" каждый день четыре раза", now)
	}
	operationFixture(p, "update", `{"target":{"type":"query","value":"Серия","filter":{"kind":"meeting","date":{"type":"relative_day","offset":2}}},"changes":{"scope":"occurrence","time":"11:00"}}`)
	r := say(t, s, "Перенеси только встречу Серия послезавтра на 11:00", now.Add(time.Second))
	if !r.Pending || len(r.Candidates) != 2 || len(readAll(t, s)) != 2 {
		t.Fatal(r)
	}
}
