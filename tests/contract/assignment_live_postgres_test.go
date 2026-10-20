package contract

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/llm"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
)

type reviewLiveInterpreter struct {
	*llm.Simple
	t *testing.T
}

func (p reviewLiveInterpreter) InterpretOperation(ctx context.Context, text string, d nlu.Context) (nlu.Operation, error) {
	op, err := p.Simple.InterpretOperation(ctx, text, d)
	p.t.Logf("model operation: %s %s (error: %v)", op.Intent, op.Arguments, err)
	return op, err
}

// Opt-in evaluation of real inference and execution against disposable schemas.
// No production account, calendar, microphone or database is used.
func TestAssignmentLiveFreeSpeech(t *testing.T) {
	if os.Getenv("LIVE_SIMPLE_LLM") != "1" {
		t.Skip("set LIVE_SIMPLE_LLM=1 and a disposable TEST_DATABASE_URL for real model evaluation")
	}
	t.Setenv("APP_ROOT", root())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	provider, err := llm.NewGigaChat(cfg.GigaChat)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	type step struct {
		text, action, kind, title, date, clock string
		pending                                bool
		reminder                               int
		personal, shared                       bool
	}
	cases := []struct {
		name          string
		steps         []step
		count         int
		importHistory bool
	}{
		{"polite_reminder", []step{{text: "Слушай, напомни мне, пожалуйста, завтра в десять утра позвонить Ивану", action: "create", kind: "reminder", title: "Иван", date: "2026-10-21", clock: "10:00"}}, 1, false},
		{"implicit_task", []step{{text: "В пятницу надо бы отправить отчёт", action: "create", kind: "task", title: "отч", date: "2026-10-23"}}, 1, false},
		{"end_of_week", []step{{text: "Мне бы до конца недели купить билеты", action: "create", kind: "task", title: "билет", date: "2026-10-25"}}, 1, false},
		{"unknown_person_meeting", []step{{text: "Давай на завтра в три часа дня запишем встречу с Иваном", action: "create", kind: "meeting", title: "Иван", date: "2026-10-21", clock: "15:00"}}, 1, false},
		{"designer_meeting", []step{{text: "В четверг в 11 у меня встреча с дизайнером", action: "create", kind: "meeting", title: "дизайнер", date: "2026-10-22", clock: "11:00"}}, 1, false},
		{"team_meeting", []step{{text: "Добавь встречу с командой завтра в 15:00", action: "create", kind: "meeting", title: "команд", date: "2026-10-21", clock: "15:00"}}, 1, false},
		{"missing_time", []step{
			{text: "Завтра встреча с Иваном", action: "clarify", pending: true},
			{text: "Давай часов в пять вечера", action: "create", kind: "meeting", title: "Иван", date: "2026-10-21", clock: "17:00"},
		}, 1, false},
		{"relative_delay", []step{{text: "Чтобы не забыть, напомни через пару часов оплатить интернет", action: "create", kind: "reminder", title: "интернет", date: "2026-10-20", clock: "14:00"}}, 1, false},
		{"colloquial_list", []step{
			{text: "Завтра надо купить краски", action: "create", kind: "task", title: "краск", date: "2026-10-21"},
			{text: "Чё у меня на завтра намечается?", action: "list", kind: "task", title: "краск", date: "2026-10-21"},
		}, 1, false},
		{"context_correction", []step{
			{text: "Запиши встречу с Иваном завтра в 15:00", action: "create", kind: "meeting", title: "Иван", date: "2026-10-21", clock: "15:00"},
			{text: "Не, давай всё-таки в шесть вечера", action: "update", kind: "meeting", title: "Иван", date: "2026-10-21", clock: "18:00"},
			{text: "И предупреди об этом за час", action: "update", kind: "meeting", title: "Иван", date: "2026-10-21", clock: "18:00", reminder: 60},
		}, 1, false},
		{"negated_alternative", []step{{text: "Встречу с Иваном запиши не в пятницу, а в субботу в 12:00", action: "create", kind: "meeting", title: "Иван", date: "2026-10-24", clock: "12:00"}}, 1, false},
		{"explicit_invitation", []step{{text: "Пригласи Ивана на новую встречу завтра в 15:00", action: "clarify", pending: true}}, 0, false},
		{"negative_command", []step{{text: "Не создавай встречу с Иваном завтра в три часа дня"}}, 0, false},
		{"historical_contract", []step{{text: "Я когда-то ставил задачу по договору, найди её", action: "search", kind: "task", title: "договор"}}, 1000, true},
		{"historical_synonym", []step{{text: "Найди старые задачи про контракт", action: "search", kind: "task", title: "договор"}}, 1000, true},
		{"presentation_typo", []step{{text: "Найди задачи про презинтацию", action: "search", kind: "task", title: "презентац"}}, 1000, true},
		{"semantic_paraphrase", []step{{text: "Найди задачи, где надо подготовить слайды для выступления", action: "search", kind: "task", title: "презентац"}}, 1000, true},
		{"additional_spoken_clock", []step{{text: "На завтра, пожалуйста, запиши обсуждение макета в шестнадцать тридцать", action: "create", kind: "meeting", title: "макет", date: "2026-10-21", clock: "16:30"}}, 1, false},
		{"additional_half_past", []step{{text: "Напомни послезавтра в половине девятого утра забрать паспорт", action: "create", kind: "reminder", title: "паспорт", date: "2026-10-22", clock: "08:30"}}, 1, false},
		{"additional_delay", []step{{text: "Сделай напоминалку через двадцать пять минут проверить духовку", action: "create", kind: "reminder", title: "духовк", date: "2026-10-20", clock: "12:25"}}, 1, false},
		{"additional_month_end", []step{{text: "Надо подать заявку к концу следующего месяца", action: "create", kind: "task", title: "заявк", date: "2026-11-30"}}, 1, false},
		{"additional_existing_friend", []step{{text: "Давай завтра в три встречусь с Матвеем", action: "create", kind: "meeting", title: "Матве", date: "2026-10-21", clock: "15:00", shared: true}}, 1, false},
		{"additional_without_invitations", []step{{text: "Запиши встречу с Матвеем завтра в 15, но никого не приглашай", action: "create", kind: "meeting", title: "Матве", date: "2026-10-21", clock: "15:00", personal: true}}, 1, false},
		{"additional_ordinal", []step{
			{text: "Запиши встречу с Иваном завтра в 15:00", action: "create", kind: "meeting", title: "Иван", date: "2026-10-21", clock: "15:00"},
			{text: "Ещё одну встречу с Иваном завтра в 17:00", action: "create", kind: "meeting", title: "Иван", date: "2026-10-21", clock: "17:00"},
			{text: "Покажи встречи с Иваном на завтра", action: "search", kind: "meeting", title: "Иван", date: "2026-10-21"},
			{text: "Вторую сдвинь на полчаса позже", action: "update", kind: "meeting", title: "Иван", date: "2026-10-21", clock: "17:30"},
		}, 2, false},
		{"additional_cancel_delete", []step{
			{text: "Завтра надо заказать бумагу", action: "create", kind: "task", title: "бумаг", date: "2026-10-21"},
			{text: "Удали эту задачу", action: "clarify", pending: true},
			{text: "Передумал, отмена", action: "cancel"},
		}, 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := schedulingFixture(t)
			h.now = time.Date(2026, 10, 20, 12, 0, 0, 0, h.now.Location())
			h.service = assistant.New(h.db, reviewLiveInterpreter{Simple: &llm.Simple{Provider: provider}, t: t}, h.service.Calendar)
			if tc.importHistory {
				h.service.Calendar.DemoPath = filepath.Join(root(), "data", "voice_assistant_events_1000.csv")
				r, err := h.service.Calendar.ImportDemo(h.a, h.now)
				if err != nil || r.Imported != 1000 {
					t.Fatal(r, err)
				}
			}
			for _, s := range tc.steps {
				r, err := h.service.Respond(h.a, s.text, h.sid, "live-"+s.text, h.now)
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("%s -> %s: %s", s.text, r.Action, r.Reply)
				if s.action != "" && r.Action.String() != s.action || r.Pending != s.pending {
					t.Fatalf("unexpected response: %+v", r)
				}
				if s.kind != "" {
					if len(r.Events) == 0 {
						t.Fatal("no matching events", r)
					}
					for _, e := range r.Events {
						if s.personal && (e.Collaborative || !strings.Contains(r.Reply, "Приглашения не отправлены")) || s.shared && !e.Collaborative {
							t.Fatalf("wrong invitation mode: %+v", r)
						}
						if s.reminder > 0 && (e.ReminderMinutes == nil || *e.ReminderMinutes != s.reminder) {
							t.Fatalf("reminder lead lost: %+v", e)
						}
						if e.Kind.String() != s.kind || s.title != "" && !strings.Contains(strings.ToLower(e.Title), strings.ToLower(s.title)) || s.date != "" && (e.Date == nil || e.Date.String() != s.date) || s.clock != "" && (e.StartTime == nil || e.StartTime.String() != s.clock) {
							t.Fatalf("unexpected event: %+v", e)
						}
					}
				}
			}
			if count := personalRecordCount(t, h); count != tc.count {
				t.Fatal("unexpected number of stored records", count, tc.count)
			}
		})
	}
}
