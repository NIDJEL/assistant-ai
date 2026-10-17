package assistant

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/search"
	"github.com/NIDJEL/assistant-ai/internal/storage/sqlite"
)

type scriptedInterpreter struct {
	turns    map[string]nlp.ContextualTurn
	calls    int
	contexts []nlp.DialogueContext
	commands map[string]domain.Command
}

func (p *scriptedInterpreter) Interpret(ctx context.Context, text string, now time.Time) (domain.Command, error) {
	p.calls++
	if command, ok := p.commands[text]; ok {
		return command, nil
	}
	return nlp.Parser{}.Interpret(ctx, text, now)
}
func (p *scriptedInterpreter) InterpretTurn(_ context.Context, text string, d nlp.DialogueContext, _ time.Time) (nlp.ContextualTurn, error) {
	p.calls++
	p.contexts = append(p.contexts, d)
	if t, ok := p.turns[text]; ok {
		return t, nil
	}
	return nlp.ContextualTurn{Status: "unknown", Type: "chat"}, nil
}

func dialogueService(t *testing.T) (*Service, *scriptedInterpreter, time.Time) {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "dialogue.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	engine, _ := search.New()
	loc, _ := time.LoadLocation("Europe/Moscow")
	p := &scriptedInterpreter{turns: map[string]nlp.ContextualTurn{}}
	s := New(db, p, &calendar.Service{Store: db, Search: engine, Location: loc})
	return s, p, time.Date(2026, 10, 16, 12, 30, 0, 0, loc)
}
func say(t *testing.T, s *Service, text string, now time.Time) domain.Response {
	t.Helper()
	r, err := s.Respond(t.Context(), text, "dialogue", text+now.String(), now)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func slots(raw string) nlp.ContextualTurn {
	return nlp.ContextualTurn{Status: "ok", Type: "slot_reply", Slots: &nlp.SlotUpdate{TemporalExpression: pointer(raw)}}
}
func reference(raw, relation string) nlp.ContextualTurn {
	return nlp.ContextualTurn{Status: "ok", Type: "reference", Reference: &nlp.DialogueReference{Relation: relation, Action: pointer("update")}, Slots: &nlp.SlotUpdate{TemporalExpression: pointer(raw)}}
}

func TestPendingReminderFastPathPreservesTitleAndExactInstant(t *testing.T) {
	s, p, now := dialogueService(t)
	first := say(t, s, "Напомни вытащить пиццу из духовки", now)
	if !first.Pending || first.Reply != "Когда напомнить?" {
		t.Fatal(first)
	}
	r := say(t, s, "Ну через 10 минут", now)
	if p.calls != 1 || r.Action != domain.ActionCreate || len(r.Events) != 1 || r.Events[0].Title != "Вытащить пиццу из духовки" || r.Events[0].OccurrenceAt == nil || !r.Events[0].OccurrenceAt.Equal(now.Add(10*time.Minute)) || r.Reply != "Готово. Через 10 минут напомню вытащить пиццу из духовки." {
		t.Fatal(r, p.calls)
	}
}

func TestRequiredTemporalConversationsNeverCallModelForSlot(t *testing.T) {
	for _, tc := range []struct {
		first, reply, title string
		minutes             int
	}{
		{"Напомни закрыть окно", "А давай-ка через 20 минут", "Закрыть окно", 20},
		{"Напомни покормить кота", "Ну, пожалуй, через 15 минут", "Покормить кота", 15},
		{"Напомни проверить почту", "Хорошо, минут через десять", "Проверить почту", 10},
	} {
		t.Run(tc.reply, func(t *testing.T) {
			s, p, now := dialogueService(t)
			if first := say(t, s, tc.first, now); !first.Pending {
				t.Fatal(first)
			}
			before := p.calls
			r := say(t, s, tc.reply, now)
			if r.Action != domain.ActionCreate || r.Pending || len(r.Events) != 1 || r.Events[0].Title != tc.title || r.Events[0].OccurrenceAt == nil || !r.Events[0].OccurrenceAt.Equal(now.Add(time.Duration(tc.minutes)*time.Minute)) || p.calls != before {
				t.Fatal(r, p.calls, before)
			}
		})
	}
}

func TestContextualModelCannotSelectPartOfAmbiguousTemporalReply(t *testing.T) {
	for _, reply := range []string{"Через 11 минут или через 22 минуты", "В 14 или в 17", "Через 11 минут, но не надо"} {
		t.Run(reply, func(t *testing.T) {
			s, p, now := dialogueService(t)
			say(t, s, "Напомни забрать керамику", now)
			raw := "через 11 минут"
			if strings.HasPrefix(reply, "В ") {
				raw = "в 14"
			}
			p.turns[reply] = slots(raw)
			r := say(t, s, reply, now)
			if r.Action != domain.ActionClarify || !r.Pending || len(r.Events) > 0 {
				t.Fatal(r)
			}
		})
	}
}

func TestNegativeDeleteRepliesClearPersistedPendingWithoutMutation(t *testing.T) {
	for _, message := range []string{"нет", "не надо", "не удаляй", "нет, не удаляй", "отмена", "отмени", "передумал", "не нужно", "оставь", "пусть останется", "не трогай", "Пожалуй, не надо", "Ну нет, пожалуйста, не трогай"} {
		t.Run(message, func(t *testing.T) {
			s, p, now := dialogueService(t)
			created := say(t, s, "Добавь задачу откалибровать датчик", now)
			r := say(t, s, "Удали задачу откалибровать датчик", now)
			if !r.Pending {
				t.Fatal(r)
			}
			before := p.calls
			r = say(t, s, message, now)
			if r.Action != domain.ActionCancel || r.Pending || p.calls != before {
				t.Fatal(r, p.calls, before)
			}
			if err := s.Store.Read(t.Context(), func(tx repository.Tx) error {
				state, err := tx.State(t.Context(), "dialogue")
				if err != nil {
					return err
				}
				if state.Pending != nil {
					t.Fatal("pending persisted")
				}
				e, err := tx.Get(t.Context(), created.Events[0].ID)
				if e == nil || !reflect.DeepEqual(*e, created.Events[0]) {
					t.Fatal("cancellation changed event", e)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPositiveConfirmationNeedsActualConfirmationPending(t *testing.T) {
	for _, message := range []string{"да", "ага", "подтверждаю", "давай", "удаляй", "точно", "верно", "Да, удаляй"} {
		t.Run(message, func(t *testing.T) {
			s, p, now := dialogueService(t)
			if r := say(t, s, message, now); r.Action != domain.ActionHelp || r.Pending || p.calls != 0 {
				t.Fatal(r, p.calls)
			}
			say(t, s, "Напомни проветрить мастерскую", now)
			before := p.calls
			if r := say(t, s, message, now.Add(time.Second)); r.Action != domain.ActionHelp || !r.Pending || p.calls != before {
				t.Fatal(r, p.calls)
			}
			say(t, s, "Добавь задачу откалибровать датчик", now)
			say(t, s, "Удали задачу откалибровать датчик", now)
			before = p.calls
			if r := say(t, s, message, now.Add(2*time.Second)); r.Action != domain.ActionDelete || r.Pending || p.calls != before {
				t.Fatal(r, p.calls)
			}
		})
	}
}

func TestFailedSearchPreservesOlderFilter(t *testing.T) {
	s, p, now := dialogueService(t)
	say(t, s, "Найди встречи с Платоном", now)
	_, original, err := s.History(t.Context(), "dialogue")
	if err != nil {
		t.Fatal(err)
	}
	p.commands = map[string]domain.Command{"Найди встречи с Мирославой": {Action: domain.ActionHelp, Error: "Не удалось понять поиск."}}
	say(t, s, "Найди встречи с Мирославой", now)
	err = s.Store.Read(t.Context(), func(tx repository.Tx) error {
		state, err := tx.State(t.Context(), "dialogue")
		if err != nil {
			return err
		}
		if !freshResult(state, now) || state.Dialogue.Query != original.Dialogue.Query {
			t.Fatal("successful filter lost after safe failure", state)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	before := p.calls
	if r := say(t, s, "А только за прошлый месяц?", now); r.Action != domain.ActionClarify || r.Pending || p.calls != before+1 {
		t.Fatal(r, p.calls)
	}
}

func TestContextualRepliesAndNewCommand(t *testing.T) {
	for _, tc := range []struct{ first, reply, raw, title, clock string }{
		{"Напомни позвонить Ивану", "Ну давай через полчаса", "через полчаса", "Позвонить Ивану", "13:00"},
		{"Добавь встречу с Алисой завтра", "Часов в десять", "часов в десять", "Встреча с Алисой", "10:00"},
	} {
		t.Run(tc.reply, func(t *testing.T) {
			s, p, now := dialogueService(t)
			p.turns[tc.reply] = slots(tc.raw)
			say(t, s, tc.first, now)
			r := say(t, s, tc.reply, now)
			if r.Action != domain.ActionCreate || r.Events[0].Title != tc.title || r.Events[0].StartTime.String() != tc.clock {
				t.Fatal(r)
			}
			if len(p.contexts) > 0 && (p.contexts[0].Pending == nil || p.contexts[0].Pending.Title != tc.title) {
				t.Fatal(p.contexts)
			}
		})
	}
	s, _, now := dialogueService(t)
	say(t, s, "Напомни позвонить Ивану", now)
	r := say(t, s, "Добавь встречу с Никитой завтра в 15", now)
	if r.Action != domain.ActionCreate || r.Events[0].Kind != domain.EventMeeting || strings.Contains(r.Events[0].Title, "Иван") {
		t.Fatal(r)
	}
}

func TestFreshReferencesCorrectionsAndChain(t *testing.T) {
	s, p, now := dialogueService(t)
	p.turns["А перенеси её на 11"] = reference("на 11", "last")
	p.turns["Нет, лучше послезавтра"] = nlp.ContextualTurn{Status: "ok", Type: "reference", Reference: &nlp.DialogueReference{Relation: "last", Action: pointer("update")}, Slots: &nlp.SlotUpdate{TemporalExpression: pointer("послезавтра")}}
	p.turns["На 15"] = nlp.ContextualTurn{Status: "ok", Type: "reference", Reference: &nlp.DialogueReference{Relation: "last", Action: pointer("update")}, Slots: &nlp.SlotUpdate{TemporalExpression: pointer("На 15")}}
	r := say(t, s, "Добавь встречу с Алисой завтра в 10", now)
	id := r.Events[0].ID
	for _, text := range []string{"А перенеси её на 11", "Нет, лучше послезавтра", "На 15"} {
		r = say(t, s, text, now)
		if r.Action != domain.ActionUpdate || r.Events[0].ID != id {
			t.Fatal(text, r)
		}
	}
	if r.Events[0].Date.String() != "2026-10-18" || r.Events[0].StartTime.String() != "15:00" {
		t.Fatal(r)
	}
}

func TestOrdinalReferenceAndSearchRefinement(t *testing.T) {
	s, p, now := dialogueService(t)
	say(t, s, "Добавь встречу с Алисой завтра в 10", now)
	b := say(t, s, "Добавь созвон с командой завтра в 15", now)
	say(t, s, "Покажи встречи на завтра", now)
	p.turns["Вторую перенеси на 16"] = reference("на 16", "second")
	r := say(t, s, "Вторую перенеси на 16", now)
	if r.Action != domain.ActionUpdate || r.Events[0].ID != b.Events[0].ID {
		t.Fatal(r)
	}
	say(t, s, "Найди встречи с Алисой", now)
	p.turns["А только за прошлую неделю?"] = nlp.ContextualTurn{Status: "ok", Type: "refinement", Slots: &nlp.SlotUpdate{PeriodExpression: pointer("за прошлую неделю")}}
	var observed DialogueObservation
	s.ObserveTurn = func(o DialogueObservation) { observed = o }
	r = say(t, s, "А только за прошлую неделю?", now)
	if r.Action != domain.ActionSearch || !strings.Contains(observed.Command.Query, "Алис") || observed.Command.PeriodStart.String() != "2026-10-05" {
		t.Fatal(r, observed)
	}
}

func TestContextTTLAndDeletionSafety(t *testing.T) {
	for _, elapsed := range []time.Duration{0, 3 * time.Hour} {
		t.Run(elapsed.String(), func(t *testing.T) {
			s, p, now := dialogueService(t)
			created := say(t, s, "Добавь задачу проверить архив", now)
			p.turns["Удали её"] = nlp.ContextualTurn{Status: "ok", Type: "reference", Reference: &nlp.DialogueReference{Relation: "last", Action: pointer("delete")}}
			r := say(t, s, "Удали её", now.Add(elapsed))
			if r.Action != domain.ActionClarify || (elapsed == 0) != r.Pending {
				t.Fatal(r)
			}
			if elapsed == 0 {
				r = say(t, s, "нет", now)
				if r.Action != domain.ActionCancel {
					t.Fatal(r)
				}
			}
			s.Store.Read(t.Context(), func(tx repository.Tx) error {
				e, err := tx.Get(t.Context(), created.Events[0].ID)
				if e == nil {
					t.Fatal("deleted without confirmation")
				}
				return err
			})
		})
	}
}

func TestChatDoesNotFillPendingOrMutate(t *testing.T) {
	s, p, now := dialogueService(t)
	say(t, s, "Напомни проверить чай", now)
	for _, text := range []string{"Привет", "Что ты умеешь?", "Спасибо", "Как тебя зовут?"} {
		r := say(t, s, text, now)
		if r.Action != domain.ActionHelp || !r.Pending || len(r.Events) != 0 {
			t.Fatal(r)
		}
	}
	if p.calls != 1 {
		t.Fatal(p.calls)
	}
}

func TestStateRoundTripAndContextPrivacy(t *testing.T) {
	s, _, now := dialogueService(t)
	say(t, s, "Добавь встречу с Алисой завтра в 10", now)
	err := s.Store.Read(t.Context(), func(tx repository.Tx) error {
		state, err := tx.State(t.Context(), "dialogue")
		if err != nil {
			return err
		}
		raw, err := json.Marshal(state)
		if err != nil {
			return err
		}
		var restored domain.State
		if err = json.Unmarshal(raw, &restored); err != nil {
			return err
		}
		if len(restored.LastIDs) != 1 || restored.Dialogue.LastAction != domain.ActionCreate {
			t.Fatal(restored)
		}
		d, err := buildDialogueContext(t.Context(), tx, state)
		if err != nil {
			return err
		}
		b, _ := json.Marshal(d)
		for _, token := range []string{"#1", "target_id", "last_ids", "participants", "access_token", "external_calendar_id", "source_key"} {
			if strings.Contains(string(b), token) {
				t.Fatal("privacy leak", token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestUnrelatedTurnsExpireReferencesWithoutExtendingTTL(t *testing.T) {
	s, p, now := dialogueService(t)
	say(t, s, "Добавь встречу с Алисой завтра в 10", now)
	for i := 0; i < 4; i++ {
		say(t, s, "Спасибо", now.Add(time.Duration(i)*time.Second))
	}
	p.turns["Перенеси её на 11"] = reference("на 11", "last")
	r := say(t, s, "Перенеси её на 11", now.Add(5*time.Second))
	if r.Action != domain.ActionClarify || r.Pending || len(r.Events) != 0 {
		t.Fatal(r)
	}
}

func TestSemanticConfirmationCannotInventUserConsent(t *testing.T) {
	s, p, now := dialogueService(t)
	created := say(t, s, "Добавь задачу проверить архив", now)
	say(t, s, "Удали задачу проверить архив", now)
	yes := true
	p.turns["Ну, может быть"] = nlp.ContextualTurn{Status: "ok", Type: "confirmation", Confirmation: &yes}
	r := say(t, s, "Ну, может быть", now)
	if r.Action != domain.ActionClarify || !r.Pending {
		t.Fatal(r)
	}
	if err := s.Store.Read(t.Context(), func(tx repository.Tx) error {
		e, err := tx.Get(t.Context(), created.Events[0].ID)
		if e == nil {
			t.Fatal("model invented confirmation")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestReferenceUpdateKeepsConflictProtection(t *testing.T) {
	s, p, now := dialogueService(t)
	say(t, s, "Добавь встречу с Алисой завтра в 10", now)
	b := say(t, s, "Добавь встречу с Борисом завтра в 15", now)
	p.turns["Перенеси её на 10"] = reference("на 10", "last")
	r := say(t, s, "Перенеси её на 10", now)
	if r.Action != domain.ActionClarify || !r.Pending {
		t.Fatal(r)
	}
	r = say(t, s, "да", now)
	if r.Action != domain.ActionUpdate || r.Events[0].ID != b.Events[0].ID || r.Events[0].StartTime.String() != "10:00" {
		t.Fatal(r)
	}
}

type blockingDialogueInterpreter struct {
	*scriptedInterpreter
	started chan struct{}
	release chan struct{}
}

func (p blockingDialogueInterpreter) InterpretTurn(ctx context.Context, text string, d nlp.DialogueContext, now time.Time) (nlp.ContextualTurn, error) {
	close(p.started)
	select {
	case <-p.release:
	case <-ctx.Done():
		return nlp.ContextualTurn{}, ctx.Err()
	}
	return p.scriptedInterpreter.InterpretTurn(ctx, text, d, now)
}

func TestContextualInferenceCannotApplyToChangedSession(t *testing.T) {
	s, p, now := dialogueService(t)
	first := say(t, s, "Добавь встречу с Алисой завтра в 10", now)
	p.turns["Перенеси её на 11"] = reference("на 11", "last")
	started, release := make(chan struct{}), make(chan struct{})
	s.Interpreter = blockingDialogueInterpreter{p, started, release}
	type result struct {
		r   domain.Response
		err error
	}
	done := make(chan result, 1)
	go func() {
		r, err := s.Respond(t.Context(), "Перенеси её на 11", "dialogue", "concurrent-reference", now)
		done <- result{r, err}
	}()
	<-started
	other := New(s.Store, nlp.Parser{}, s.Calendar)
	say(t, other, "Добавь задачу проверить окно", now)
	close(release)
	r := <-done
	if r.err != nil || r.r.Action != domain.ActionClarify || !strings.Contains(r.r.Reply, "Контекст диалога изменился") {
		t.Fatal(r)
	}
	if err := s.Store.Read(t.Context(), func(tx repository.Tx) error {
		e, err := tx.Get(t.Context(), first.Events[0].ID)
		if e == nil || e.StartTime.String() != "10:00" {
			t.Fatal("stale inference changed original target")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
