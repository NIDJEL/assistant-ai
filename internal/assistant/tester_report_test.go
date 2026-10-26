package assistant

import (
	"context"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
	"strings"
	"testing"
	"time"
)

type reportedTargetRepair struct {
	*scriptedOperations
	issue nlu.ValidationIssue
}

func (p *reportedTargetRepair) RepairValidation(_ context.Context, _ string, _ nlu.Context, _ nlu.Operation, issue nlu.ValidationIssue) (nlu.Operation, error) {
	p.issue = issue
	return nlu.Decode([]byte(`{"intent":"update","arguments":{"target":{"type":"query","value":"дизайнер","filter":{"kind":"meeting"}},"changes":{"shift_minutes":60}},"confidence":"high","clarification":null}`))
}

func TestReportedNamedMeetingCanRepairAnUnavailableContextReference(t *testing.T) {
	s, p, now := simpleService(t)
	seedPlanEvent(t, s, "Встреча с дизайнером", "2026-10-29", "11:00", domain.EventMeeting, now)
	operationFixture(p, "update", `{"target":{"type":"last"},"changes":{"shift_minutes":60}}`)
	repair := &reportedTargetRepair{scriptedOperations: p}
	s.Interpreter = repair
	r := say(t, s, "Встречу с дизайнером сдвинь на час позже", now)
	if repair.issue.Code != "named_target_required" || r.Pending || len(r.Events) != 1 || r.Events[0].StartTime.String() != "12:00" {
		t.Fatal(repair.issue, r)
	}
}

func TestReportedNamedTopicVetoesWrongContextMutation(t *testing.T) {
	for _, action := range []string{"complete", "delete", "update"} {
		t.Run(action, func(t *testing.T) {
			s, p, now := simpleService(t)
			seedPlanEvent(t, s, "Отправить отчет", "2026-10-30", "", domain.EventTask, now)
			seedPlanEvent(t, s, "Согласовать договор аренды с юристом", "2026-10-29", "", domain.EventTask, now)
			operationFixture(p, "search", `{"query":"договор"}`)
			say(t, s, "Найди договор", now)
			operationFixture(p, "update", `{"target":{"type":"last"},"changes":{"date":{"type":"absolute","date":"2026-10-30"}}}`)
			say(t, s, "Перенеси его на пятницу", now.Add(time.Second))
			args := `{"target":{"type":"last"}`
			if action == "update" {
				args += `,"changes":{"status":"done"}`
			}
			operationFixture(p, action, args+`}`)
			r := say(t, s, "Я отправил отчёт, можно вычеркнуть", now.Add(2*time.Second))
			if r.Action != domain.ActionClarify || !strings.Contains(r.Reply, "другая запись") {
				t.Fatal(r)
			}
			for _, e := range readAll(t, s) {
				if e.Status != domain.StatusOpen || e.DeletedAt != nil {
					t.Fatal("unrelated record mutated", e)
				}
			}
		})
	}
}

func TestReportedEmptyTaskReadFindsOtherKindsButHonorsStrictFilter(t *testing.T) {
	for _, intent := range []string{"list", "search"} {
		for _, strict := range []bool{false, true} {
			s, p, now := simpleService(t)
			seedPlanEvent(t, s, "Созвон по презентации", "2026-10-17", "15:00", domain.EventMeeting, now)
			args := `{"kind":"task","date":{"type":"relative_day","offset":1}`
			if intent == "search" {
				args += `,"query":"презентация"`
			}
			if strict {
				args += `,"strict_kind":true`
			}
			operationFixture(p, intent, args+`}`)
			r := say(t, s, "Найди задачу про презентацию завтра", now)
			want := 1
			if strict {
				want = 0
			}
			if len(r.Events) != want {
				t.Fatal(intent, strict, r)
			}
			if !strict && !strings.Contains(r.Reply, "другие") {
				t.Fatal(r.Reply)
			}
		}
	}
}

func TestReportedDaytimeDefaultsAndQualifiedNight(t *testing.T) {
	for _, tc := range []struct{ text, clock string }{{"В четверг в 11 у меня встреча с дизайнером", "11:00"}, {"Завтра встреча в десять", "10:00"}, {"Завтра встреча в 12", "12:00"}, {"Завтра встреча в 11 вечера", "23:00"}} {
		s, p, now := simpleService(t)
		operationFixture(p, "create_meeting", `{"title":"Обсуждение","date":{"type":"relative_day","offset":1},"time":"`+tc.clock+`"}`)
		r := say(t, s, tc.text, now)
		if r.Pending || len(r.Events) != 1 || r.Events[0].StartTime.String() != tc.clock {
			t.Fatal(tc, r)
		}
	}
}

func TestReportedPastReminderRequiresNewDate(t *testing.T) {
	s, p, now := simpleService(t)
	operationFixture(p, "create_reminder", `{"title":"Сдать анализы","date":{"type":"relative_day","offset":-1},"time":"09:00"}`)
	r := say(t, s, "Напомни вчера в 9 утра сдать анализы", now)
	if !r.Pending || len(readAll(t, s)) != 0 || !strings.Contains(r.Reply, "уже прошло") {
		t.Fatal(r)
	}
	operationFixture(p, "continue", `{"changes":{"date":{"type":"relative_day","offset":1}}}`)
	r = say(t, s, "Завтра", now.Add(time.Second))
	if r.Pending || len(r.Events) != 1 || r.Events[0].StartTime.String() != "09:00" {
		t.Fatal(r)
	}
}

func TestReportedQuestionsAndReopenReply(t *testing.T) {
	s, p, now := simpleService(t)
	operationFixture(p, "create_reminder", `{"title":"Оплатить интернет","date":{"type":"relative_day","offset":2}}`)
	r := say(t, s, "Напомни послезавтра оплатить интернет", now)
	if !strings.Contains(r.Reply, "Оплатить интернет") || !strings.Contains(r.Reply, "18.10") {
		t.Fatal(r)
	}
	operationFixture(p, "create_task", `{"title":"Отправить отчет"}`)
	r = say(t, s, "Добавь задачу отправить отчет", now.Add(time.Second))
	if !strings.Contains(r.Reply, "Предыдущий запрос") {
		t.Fatal(r)
	}
	operationFixture(p, "complete", `{"target":{"type":"last"}}`)
	say(t, s, "Отметь её выполненной", now.Add(2*time.Second))
	operationFixture(p, "update", `{"target":{"type":"last"},"changes":{"status":"open"}}`)
	r = say(t, s, "Верни её в работу", now.Add(3*time.Second))
	if !strings.Contains(r.Reply, "Вернул в работу") || r.Events[0].Status != domain.StatusOpen {
		t.Fatal(r)
	}
}

func TestReportedWeeklySpeechIncludesAllTenRecords(t *testing.T) {
	s, p, now := simpleService(t)
	for _, name := range []string{"Первое", "Второе", "Третье", "Четвертое", "Пятое", "Шестое", "Седьмое", "Восьмое", "Ужин с родителями", "Купить билеты"} {
		seedPlanEvent(t, s, name, "2026-10-17", "", domain.EventTask, now)
	}
	operationFixture(p, "list", `{"period":{"type":"calendar_period","unit":"week","offset":0}}`)
	r := say(t, s, "Что запланировано на эту неделю?", now)
	if len(r.Events) != 10 || !strings.Contains(r.Reply, "Ужин с родителями") || !strings.Contains(r.Reply, "Купить билеты") {
		t.Fatal(r)
	}
}

func TestReportedTopicQuestionDoesNotInheritInventedToday(t *testing.T) {
	for _, today := range []bool{false, true} {
		s, p, now := simpleService(t)
		seedPlanEvent(t, s, "Ужин с родителями", "2026-10-31", "19:00", domain.EventMeeting, now)
		operationFixture(p, "search", `{"kind":"meeting","query":"родители","date":{"type":"relative_day","offset":0}}`)
		text := "Когда я вижусь с мамой и папой?"
		if today {
			text = "Когда я сегодня вижусь с мамой и папой?"
		}
		r := say(t, s, text, now)
		want := 1
		if today {
			want = 0
		}
		if len(r.Events) != want {
			t.Fatal(today, r)
		}
	}
}
