package assistant

import (
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

func TestSimpleTypedLastReferenceSelectsByStoredKind(t *testing.T) {
	for _, action := range []string{"delete", "update", "complete"} {
		t.Run(action, func(t *testing.T) {
			s, p, now := simpleService(t)
			task := seedPlanEvent(t, s, "Подготовиться к встрече с Пульсом", "2026-10-17", "10:15", domain.EventTask, now)
			meeting := seedPlanEvent(t, s, "Встреча с Иваном", "2026-10-17", "17:30", domain.EventMeeting, now)
			operationFixture(p, "list", `{"date":{"type":"relative_day","offset":1}}`)
			if r := say(t, s, "Покажи в итоге, что у меня завтра", now); len(r.Events) != 2 {
				t.Fatal(r)
			}
			kind := "meeting"
			target := meeting
			if action == "complete" {
				kind, target = "task", task
			}
			args := `{"target":{"type":"last","filter":{"kind":"` + kind + `"}}`
			if action == "update" {
				args += `,"changes":{"time":"18:15"}`
			}
			operationFixture(p, action, args+`}`)
			r := say(t, s, "Действие с этой записью названного типа", now.Add(time.Second))
			if action == "delete" {
				if !r.Pending || !strings.Contains(r.Reply, "Удалить") || !strings.Contains(r.Reply, meeting.Title) || len(readAll(t, s)) != 2 {
					t.Fatal("expected confirmation of only the meeting", r)
				}
				operationFixture(p, "cancel", `{}`)
				if r := say(t, s, "Передумал, отмена", now.Add(2*time.Second)); r.Pending || len(readAll(t, s)) != 2 {
					t.Fatal("cancellation deleted a record", r)
				}
				operationFixture(p, "delete", args+`}`)
				say(t, s, "Удали эту встречу", now.Add(3*time.Second))
				r = say(t, s, "да", now.Add(4*time.Second))
				remaining := readAll(t, s)
				if r.Action != domain.ActionDelete || len(remaining) != 1 || remaining[0].ID != task.ID {
					t.Fatal("wrong record removed", r, remaining)
				}
			} else if len(r.Events) != 1 || r.Events[0].ID != target.ID || r.Action.String() != action {
				t.Fatal("wrong typed reference", r)
			} else if action == "update" && r.Events[0].StartTime.String() != "18:15" {
				t.Fatal(r)
			}
		})
	}
}

func TestSimpleTypedLastReferenceDoesNotGuessOrEscapeDisplayedList(t *testing.T) {
	for _, scope := range []string{"ambiguous", "absent", "untyped", "expired"} {
		t.Run(scope, func(t *testing.T) {
			s, p, now := simpleService(t)
			seedPlanEvent(t, s, "Подготовиться к встрече с Пульсом", "2026-10-17", "10:15", domain.EventTask, now)
			if scope != "absent" {
				seedPlanEvent(t, s, "Встреча с Иваном", "2026-10-17", "17:30", domain.EventMeeting, now)
			}
			if scope == "ambiguous" {
				seedPlanEvent(t, s, "Проверка сметы", "2026-10-17", "19:30", domain.EventMeeting, now)
			}
			seedPlanEvent(t, s, "Встреча вне последнего списка", "2026-10-18", "12:00", domain.EventMeeting, now)
			operationFixture(p, "list", `{"date":{"type":"relative_day","offset":1}}`)
			say(t, s, "Планы на завтра", now)
			args := `{"target":{"type":"last","filter":{"kind":"meeting"}}}`
			if scope == "untyped" {
				args = `{"target":{"type":"last"}}`
			}
			at := now.Add(time.Second)
			if scope == "expired" {
				at = now.Add(referenceTTL + time.Second)
			}
			before := len(readAll(t, s))
			operationFixture(p, "delete", args)
			r := say(t, s, "Удали эту встречу", at)
			if r.Action != domain.ActionClarify || r.Pending || len(readAll(t, s)) != before {
				t.Fatal("ambiguous/missing/stale context selected a record", r)
			}
		})
	}
}
