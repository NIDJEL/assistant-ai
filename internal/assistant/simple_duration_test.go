package assistant

import (
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

func TestSimplePendingTimeAndDurationSurviveInvalidReply(t *testing.T) {
	for _, invalid := range []string{
		`{"changes":{"time":"16:15","duration_minutes":2}}`,
		`{"changes":{"time":"16:15","duration_minutes":1441}}`,
		`{"changes":{"time":"not-a-clock","duration_minutes":30}}`,
		`{"changes":{"time":"16:15","unsupported_field":30}}`,
	} {
		t.Run(invalid, func(t *testing.T) {
			s, p, now := simpleService(t)
			operationFixture(p, "create_meeting", `{"title":"Обсуждение макета","date":{"type":"relative_day","offset":2},"reminder_minutes":40}`)
			if r := say(t, s, "Запиши послезавтра обсуждение макета", now); !r.Pending || len(readAll(t, s)) != 0 {
				t.Fatal(r)
			}
			operationFixture(p, "continue", invalid)
			if r := say(t, s, "Неразобранный ответ о времени", now.Add(time.Second)); !r.Pending || r.Action != domain.ActionClarify || len(readAll(t, s)) != 0 {
				t.Fatal("invalid reply lost pending creation or created a record", r)
			}
			operationFixture(p, "continue", `{"changes":{"time":"16:15","duration_minutes":30}}`)
			r := say(t, s, "В четыре пятнадцать дня, на полчаса", now.Add(2*time.Second))
			if r.Action != domain.ActionCreate || r.Pending || len(r.Events) != 1 || len(readAll(t, s)) != 1 {
				t.Fatal(r)
			}
			e := r.Events[0]
			if e.Title != "Обсуждение макета" || e.Date.String() != domain.Day(now.AddDate(0, 0, 2)).String() || e.StartTime.String() != "16:15" || e.Duration != 30*time.Minute || e.ReminderMinutes == nil || *e.ReminderMinutes != 40 {
				t.Fatal("pending fields lost", e)
			}
		})
	}
}

func TestSimpleDurationUpdatePreservesOtherFieldsAndChecksConflicts(t *testing.T) {
	s, p, now := simpleService(t)
	meeting := seedPlanEvent(t, s, "Проверка макета", "2026-10-17", "10:00", domain.EventMeeting, now)
	operationFixture(p, "search", `{"kind":"meeting","query":"макет"}`)
	say(t, s, "Найди встречу по макету", now)
	operationFixture(p, "update", `{"target":{"type":"last"},"changes":{"duration_minutes":30}}`)
	r := say(t, s, "Сделай её на полчаса", now.Add(time.Second))
	if r.Action != domain.ActionUpdate || r.Pending || len(r.Events) != 1 || r.Events[0].ID != meeting.ID || r.Events[0].StartTime.String() != "10:00" || r.Events[0].Duration != 30*time.Minute {
		t.Fatal(r)
	}
	operationFixture(p, "update", `{"target":{"type":"last"},"changes":{"time":"11:00"}}`)
	r = say(t, s, "Нет, лучше в одиннадцать", now.Add(2*time.Second))
	if r.Action != domain.ActionUpdate || r.Pending || r.Events[0].Duration != 30*time.Minute {
		t.Fatal("time correction reset duration", r)
	}
	seedPlanEvent(t, s, "Другая встреча", "2026-10-17", "11:45", domain.EventMeeting, now)
	operationFixture(p, "update", `{"target":{"type":"last"},"changes":{"duration_minutes":60}}`)
	r = say(t, s, "Давай продлим до часа", now.Add(3*time.Second))
	if !r.Pending || len(r.Conflicts) == 0 && len(r.Candidates) == 0 {
		t.Fatal("duration extension bypassed conflict check", r)
	}
	if r := say(t, s, "да", now.Add(4*time.Second)); r.Action != domain.ActionUpdate || r.Pending || r.Events[0].Duration != time.Hour || r.Events[0].StartTime.String() != "11:00" {
		t.Fatal("confirmation lost changed duration", r)
	}
}

func TestSimpleBatchDurationRequiresPreviewAndConfirmation(t *testing.T) {
	s, p, now := simpleService(t)
	seedPlanEvent(t, s, "Обсуждение макета", "2026-10-17", "10:00", domain.EventMeeting, now)
	seedPlanEvent(t, s, "Обсуждение плана", "2026-10-17", "12:00", domain.EventMeeting, now)
	operationFixture(p, "batch_update", `{"filter":{"kind":"meeting","date":{"type":"relative_day","offset":1}},"changes":{"duration_minutes":30}}`)
	r := say(t, s, "Сделай завтрашние встречи по полчаса", now)
	if !r.Pending {
		t.Fatal("batch had no preview", r)
	}
	for _, e := range readAll(t, s) {
		if e.Duration != time.Hour {
			t.Fatal("batch changed a record before confirmation", e)
		}
	}
	if r := say(t, s, "да", now.Add(time.Second)); r.Action != domain.ActionUpdate || r.Pending {
		t.Fatal(r)
	}
	for _, e := range readAll(t, s) {
		if e.Duration != 30*time.Minute {
			t.Fatal("batch duration not persisted", e)
		}
	}
}

func TestSimpleInvalidDurationDisarmsOlderConflictConfirmation(t *testing.T) {
	s, p, now := simpleService(t)
	meeting := seedPlanEvent(t, s, "Проверка макета", "2026-10-17", "10:00", domain.EventMeeting, now)
	seedPlanEvent(t, s, "Другая встреча", "2026-10-17", "11:45", domain.EventMeeting, now)
	operationFixture(p, "search", `{"kind":"meeting","query":"макет"}`)
	say(t, s, "Найди встречу по макету", now)
	operationFixture(p, "update", `{"target":{"type":"last"},"changes":{"time":"11:00"}}`)
	if r := say(t, s, "Перенеси на одиннадцать", now.Add(time.Second)); !r.Pending {
		t.Fatal("expected conflict confirmation", r)
	}
	operationFixture(p, "continue", `{"changes":{"duration_minutes":2}}`)
	if r := say(t, s, "Лучше на две минуты", now.Add(2*time.Second)); r.Pending || r.Action != domain.ActionClarify {
		t.Fatal("invalid correction kept an older preview armed", r)
	}
	operationFixture(p, "confirm", `{}`)
	if r := say(t, s, "да", now.Add(3*time.Second)); r.Action != domain.ActionClarify || r.Pending {
		t.Fatal("confirmed an invalidated preview", r)
	}
	for _, e := range readAll(t, s) {
		if e.ID == meeting.ID && e.StartTime.String() != "10:00" {
			t.Fatal("stale move executed", e)
		}
	}
}
