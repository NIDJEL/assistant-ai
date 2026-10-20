package contract

import (
	"context"
	"errors"
	"fmt"
	"github.com/NIDJEL/assistant-ai/internal/auth"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/search"
	"github.com/NIDJEL/assistant-ai/internal/social"
	"github.com/NIDJEL/assistant-ai/internal/storage/postgres"
	"github.com/google/uuid"
)

type schedulingInterpreter struct {
	op      nlu.Operation
	calls   int
	compact nlu.Context
}

type schedulingZoom struct {
	h     *schedulingHarness
	syncs int
	fail  bool
}

func (z *schedulingZoom) Ready(context.Context, string) error { return nil }
func (z *schedulingZoom) SyncEvent(ctx context.Context, id int64, google, zoom bool) (*domain.Event, error) {
	z.syncs++
	if z.fail {
		return nil, errors.New("provider unavailable")
	}
	var e *domain.Event
	err := z.h.db.Write(ctx, func(tx repository.Tx) error {
		var err error
		e, err = tx.Get(ctx, id)
		if err != nil {
			return err
		}
		e.MeetingURL = "https://zoom.us/j/123456789"
		e, err = tx.Update(ctx, *e, z.h.now)
		return err
	})
	return e, err
}

func TestPostgresSchedulingExistingOwnerZoomAndMultipleChoices(t *testing.T) {
	h := schedulingFixture(t)
	day := domain.Day(h.now.AddDate(0, 0, 1))
	first := h.shared(h.a, h.bid, day, 14)
	second := h.shared(h.a, h.bid, day, 17)
	z := &schedulingZoom{h: h}
	h.service.Integrations = z
	r := h.say(h.a, "meeting_zoom", `{"friends":["Матвей Лапин"],"date":{"type":"relative_day","offset":1}}`, "Добавь Zoom к встрече с Матвеем завтра")
	if !r.Pending || r.Scheduling == nil || len(r.Scheduling.Meetings) != 2 || h.count() != 2 || z.syncs != 0 {
		t.Fatal(r)
	}
	id := r.Scheduling.Meetings[1].ID
	r = h.say(h.a, "choose_time", `{"option":2}`, "Вторую")
	if r.Pending || len(r.Events) != 1 || r.Events[0].ID != id || len(r.ZoomMeetings) != 1 || r.ZoomMeetings[0].JoinURL == "" || z.syncs != 1 || h.count() != 2 {
		t.Fatal(r)
	}
	r = h.say(h.a, "meeting_zoom", `{"friends":["Матвей"],"date":{"type":"relative_day","offset":1},"query":"Встреча"}`, "Покажи Zoom на завтра")
	if r.Scheduling == nil {
		t.Fatal(r)
	}
	// Selecting the same linked meeting does not invoke another Zoom create/sync.
	r = h.say(h.a, "choose_time", `{"option":2}`, "Вторую")
	if r.Events[0].ID != id || z.syncs != 1 || h.count() != 2 {
		t.Fatal(r)
	}
	_ = first
	_ = second
}

func TestPostgresSchedulingZoomFailureDoesNotClaimSuccess(t *testing.T) {
	h := schedulingFixture(t)
	h.shared(h.a, h.bid, domain.Day(h.now.AddDate(0, 0, 1)), 17)
	z := &schedulingZoom{h: h, fail: true}
	h.service.Integrations = z
	r := h.say(h.a, "meeting_zoom", `{"friends":["Матвей"],"date":{"type":"relative_day","offset":1}}`, "Добавь Zoom завтра")
	if !r.Pending || r.Scheduling == nil || r.Scheduling.Kind != "existing" || z.syncs != 0 {
		t.Fatal(r)
	}
	r = h.say(h.a, "choose_time", `{"confirm":true}`, "Да, добавить")
	if !strings.Contains(r.Reply, "ещё не подтверждена") || len(r.ZoomMeetings) != 1 || r.ZoomMeetings[0].JoinURL != "" || h.count() != 1 {
		t.Fatal(r)
	}
}

func (p *schedulingInterpreter) Interpret(context.Context, string, time.Time) (domain.Command, error) {
	return domain.Command{}, fmt.Errorf("legacy called")
}
func (p *schedulingInterpreter) InterpretOperation(_ context.Context, _ string, c nlu.Context) (nlu.Operation, error) {
	p.calls++
	p.compact = c
	return p.op, nil
}

type schedulingHarness struct {
	t             *testing.T
	db            *postgres.Database
	auth          *auth.Service
	service       *assistant.Service
	parser        *schedulingInterpreter
	a, b          context.Context
	aid, bid, sid string
	now           time.Time
}

func schedulingFixture(t *testing.T) *schedulingHarness {
	t.Helper()
	db, auth, _, now := pgFixture(t)
	a, err := auth.Register(t.Context(), "Никита Круглов", "schedule-a@example.test", "password 12345", "password 12345")
	if err != nil {
		t.Fatal(err)
	}
	b, err := auth.Register(t.Context(), "Матвей Лапин", "schedule-b@example.test", "password 12345", "password 12345")
	if err != nil {
		t.Fatal(err)
	}
	p := &schedulingInterpreter{}
	engine, _ := search.New()
	cal := &calendar.Service{Store: db, Location: now.Location(), Search: engine}
	h := &schedulingHarness{t: t, db: db, auth: auth, service: assistant.New(db, p, cal), parser: p, a: repository.WithUser(t.Context(), a.ID), b: repository.WithUser(t.Context(), b.ID), aid: a.ID, bid: b.ID, sid: uuid.NewString(), now: now}
	var code string
	if err = db.DB.QueryRow(`SELECT friend_code FROM users WHERE id=$1`, b.ID).Scan(&code); err != nil {
		t.Fatal(err)
	}
	r := h.execute(h.a, social.Operation{Action: "request", Code: code})
	h.execute(h.b, social.Operation{Action: "accept_request", ID: r.ID})
	return h
}
func (h *schedulingHarness) execute(ctx context.Context, op social.Operation) social.Result {
	h.t.Helper()
	op.RequestID = uuid.NewString()
	var r social.Result
	err := h.db.Write(ctx, func(tx repository.Tx) error {
		session, e := social.In(ctx, tx, h.now.Location())
		if e != nil {
			return e
		}
		r, e = session.Execute(social.WithTx(ctx, tx), op, h.now)
		return e
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return r
}
func (h *schedulingHarness) say(ctx context.Context, intent, args, text string) domain.Response {
	h.t.Helper()
	op, e := nlu.Decode([]byte(fmt.Sprintf(`{"intent":%q,"arguments":%s,"confidence":"high","clarification":null}`, intent, args)))
	if e != nil {
		h.t.Fatal(e)
	}
	h.parser.op = op
	r, e := h.service.Respond(ctx, text, h.sid, uuid.NewString(), h.now)
	if e != nil {
		h.t.Fatal(e)
	}
	return r
}
func (h *schedulingHarness) personal(ctx context.Context, day *domain.Date, hour int, minutes int) int64 {
	h.t.Helper()
	var id int64
	err := h.db.Write(ctx, func(tx repository.Tx) error {
		e, err := tx.Create(ctx, domain.Event{Kind: domain.EventMeeting, Title: "PRIVATE EVENT", Date: day, StartTime: &domain.TimeOfDay{Hour: hour}, Duration: time.Duration(minutes) * time.Minute, Status: domain.StatusOpen, Priority: domain.PriorityNormal}, h.now)
		if e != nil {
			id = e.ID
		}
		return err
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return id
}
func (h *schedulingHarness) shared(ctx context.Context, other string, day *domain.Date, hour int) int64 {
	e := domain.Event{Kind: domain.EventMeeting, Title: "Встреча", Date: day, StartTime: &domain.TimeOfDay{Hour: hour}, Duration: time.Hour, Status: domain.StatusOpen, Priority: domain.PriorityNormal}
	return h.execute(ctx, social.Operation{Action: "create_meeting", Event: &e, FriendIDs: []string{other}}).EventID
}
func (h *schedulingHarness) count() int {
	h.t.Helper()
	var count int
	if err := h.db.DB.QueryRow(`SELECT count(*) FROM collaborative_meetings`).Scan(&count); err != nil {
		h.t.Fatal(err)
	}
	return count
}

func TestPostgresSchedulingExistingMeetingRolesAndDisplayTitle(t *testing.T) {
	h := schedulingFixture(t)
	day, _ := domain.ParseDate("2026-10-22")
	id := h.shared(h.b, h.aid, day, 17)
	h.execute(h.a, social.Operation{Action: "respond", EventID: id, Version: 1, Status: "accepted"})
	r := h.say(h.a, "meeting_zoom", `{"friends":["Лапин Матвей"],"date":{"type":"day_of_month","day":22}}`, "Добавь Zoom к встрече с Матвеем 22 числа")
	if r.Pending || len(r.Events) != 1 || r.Events[0].ID != id || r.Events[0].OrganizerID != h.bid || !strings.Contains(r.Reply, "только организатор") || h.count() != 1 {
		t.Fatal(r)
	}
	if r.Events[0].DisplayTitle != "Встреча с Матвеем Лапиным" {
		t.Fatal(r.Events[0])
	}
	var title string
	if err := h.db.DB.QueryRow(`SELECT title FROM events WHERE id=$1`, id).Scan(&title); err != nil || title != "Встреча" {
		t.Fatal(title, err)
	}
	if err := h.db.Read(h.a, func(tx repository.Tx) error {
		e, err := tx.Get(h.a, id)
		if err != nil {
			return err
		}
		if e.DisplayTitle != r.Events[0].DisplayTitle || e.OrganizerName != "Матвей Лапин" {
			t.Fatal(e)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Accessible invitations still resolve after the friendship was removed.
	h.execute(h.b, social.Operation{Action: "remove_friend", UserID: h.aid})
	r = h.say(h.a, "meeting_zoom", `{"friends":["Матвей Лапин"],"date":{"type":"day_of_month","day":22}}`, "Добавь Zoom к этой встрече")
	if r.Pending || len(r.Events) != 1 || r.Events[0].ID != id || h.count() != 1 {
		t.Fatal(r)
	}
}

func TestPostgresSchedulingConflictDurableOrdinalAndInvitation(t *testing.T) {
	h := schedulingFixture(t)
	h.personal(h.b, domain.Day(h.now.AddDate(0, 0, 1)), 16, 60)
	r := h.say(h.a, "create_meeting", `{"title":"Встреча","friends":["Лапин Матвей"],"date":{"type":"relative_day","offset":1},"time":"16:00","duration_minutes":30}`, "Назначь встречу с Матвеем завтра в 16")
	if !r.Pending || r.Scheduling == nil || len(r.Scheduling.Slots) < 2 || h.count() != 0 || strings.Contains(r.Reply, "PRIVATE") {
		t.Fatal(r)
	}
	first, second := r.Scheduling.Slots[0], r.Scheduling.Slots[1]
	if first.Start.Hour() != 17 || first.Start.Before(h.now) || second.End.Sub(second.Start) != 30*time.Minute {
		t.Fatal(r.Scheduling)
	}
	// Reconstruct the process: pending command and exact ordered offers live in PostgreSQL.
	h.service = assistant.New(h.db, h.parser, h.service.Calendar)
	r = h.say(h.a, "choose_time", `{"option":2}`, "Давай второй")
	if r.Pending || len(r.Events) != 1 || r.Events[0].StartTime.String() != second.Start.Format("15:04") || r.Events[0].Duration != 30*time.Minute || r.Events[0].Title != "Встреча с Матвеем Лапиным" || h.count() != 1 {
		t.Fatal(r)
	}
	var status string
	if err := h.db.DB.QueryRow(`SELECT status FROM meeting_participants WHERE event_id=$1 AND user_id=$2`, r.Events[0].ID, h.bid).Scan(&status); err != nil || status != "pending" {
		t.Fatal(status, err)
	}
	if !strings.Contains(r.Reply, "приглашения") {
		t.Fatal(r.Reply)
	}
}

func TestPostgresSchedulingButtonRechecksConflictsAndIsIdempotent(t *testing.T) {
	h := schedulingFixture(t)
	day := domain.Day(h.now.AddDate(0, 0, 1))
	h.personal(h.b, day, 16, 60)
	r := h.say(h.a, "create_meeting", `{"title":"Обсуждение макета","friends":["Матвей"],"date":{"type":"relative_day","offset":1},"time":"16:00"}`, "Обсудим макет с Матвеем завтра в 16")
	if r.Scheduling == nil || len(r.Scheduling.Slots) == 0 {
		t.Fatal(r)
	}
	slot := r.Scheduling.Slots[0]
	token := r.Scheduling.Token
	h.personal(h.b, day, slot.Start.Hour(), 60)
	click := func(token string, slot domain.PlanningSlot, rid string) domain.Response {
		h.t.Helper()
		calls := h.parser.calls
		ctx := assistant.WithChatChoice(h.a, &domain.ChatChoice{Token: token, Action: "slot", Index: 1, Slot: &slot})
		result, err := h.service.Respond(ctx, "Выбираю время", h.sid, rid, h.now)
		if err != nil {
			t.Fatal(err)
		}
		if h.parser.calls != calls {
			t.Fatal("button called LLM")
		}
		return result
	}
	r = click(token, slot, uuid.NewString())
	if !r.Pending || r.Scheduling == nil || h.count() != 0 || r.Scheduling.Slots[0].Start.Equal(slot.Start) {
		t.Fatal(r)
	}
	newToken, newSlot := r.Scheduling.Token, r.Scheduling.Slots[0]
	rid := uuid.NewString()
	r = click(newToken, newSlot, rid)
	if r.Pending || len(r.Events) != 1 || h.count() != 1 {
		t.Fatal(r)
	}
	id := r.Events[0].ID
	r = click(newToken, newSlot, rid)
	if len(r.Events) != 1 || r.Events[0].ID != id || h.count() != 1 {
		t.Fatal(r)
	}
	r = click(newToken, newSlot, uuid.NewString())
	if r.Action != domain.ActionClarify || h.count() != 1 {
		t.Fatal(r)
	}
}

func TestPostgresSchedulingMoreCorrectionCancelAndTagAmbiguity(t *testing.T) {
	h := schedulingFixture(t)
	h.execute(h.a, social.Operation{Action: "tags", UserID: h.bid, Tags: []string{"Коллега", "Нестандартный тег"}})
	h.personal(h.b, domain.Day(h.now), 16, 60)
	r := h.say(h.a, "create_meeting", `{"title":"Встреча","friend_tag":"Коллега","date":{"type":"relative_day","offset":0},"time":"16:00"}`, "Назначь коллеге встречу сегодня в 16")
	if !r.Pending || r.Scheduling == nil || r.Scheduling.Slots[0].Start.Hour() < 17 {
		t.Fatal(r)
	}
	last := r.Scheduling.Slots[len(r.Scheduling.Slots)-1].End
	r = h.say(h.a, "choose_time", `{"more":true}`, "Предложи ещё варианты")
	if r.Scheduling == nil || r.Scheduling.Slots[0].Start.Before(last) {
		t.Fatal(r)
	}
	r = h.say(h.a, "choose_time", `{"changes":{"date":{"type":"relative_day","offset":1},"time":"11:00"}}`, "Нет, лучше завтра в 11")
	if r.Pending || len(r.Events) != 1 || r.Events[0].StartTime.Hour != 11 || r.Events[0].Date.String() != domain.Day(h.now.AddDate(0, 0, 1)).String() {
		t.Fatal(r)
	}
	r = h.say(h.a, "create_meeting", `{"title":"","friends":["Неизвестный"]}`, "Назначь с неизвестным")
	if !r.Pending || !strings.Contains(r.Reply, "не найден") {
		t.Fatal(r)
	}
	r = h.say(h.a, "cancel", `{}`, "Отмена")
	if r.Pending || r.Action != domain.ActionCancel {
		t.Fatal(r)
	}
	// A shared tag must clarify rather than silently select or invite everybody.
	if _, err := h.db.DB.Exec(`UPDATE users SET display_name='Никита Круглов' WHERE id=$1`, h.bid); err != nil {
		t.Fatal(err)
	}
	if err := h.db.Read(h.a, func(tx repository.Tx) error {
		s, _ := social.In(h.a, tx, h.now.Location())
		ids, err := s.ResolveInvitees(h.a, []string{"Круглов Никита"}, "", false)
		if err != nil || len(ids) != 1 || ids[0] != h.bid {
			t.Fatal(ids, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresSchedulingMissingSubjectKeepsDateAndTime(t *testing.T) {
	h := schedulingFixture(t)
	r := h.say(h.a, "create_meeting", `{"title":"Встреча","date":{"type":"relative_day","offset":1},"time":"15:00"}`, "Создай встречу завтра в 15")
	if !r.Pending || r.Reply != "С кем встреча или по какому поводу?" {
		t.Fatal(r)
	}
	r = h.say(h.a, "continue", `{"changes":{"title":"Обсуждение дизайна приложения"}}`, "Обсудим дизайн приложения")
	if r.Pending || len(r.Events) != 1 || r.Events[0].Title != "Обсуждение дизайна приложения" || r.Events[0].StartTime.Hour != 15 {
		t.Fatal(r)
	}
}

func TestPostgresSchedulingAmbiguousTagAndRevokedFriendship(t *testing.T) {
	h := schedulingFixture(t)
	c, err := h.auth.Register(t.Context(), "Алиса Проверка", "schedule-c@example.test", "password 12345", "password 12345")
	if err != nil {
		t.Fatal(err)
	}
	ctx := repository.WithUser(t.Context(), c.ID)
	var code string
	if err = h.db.DB.QueryRow(`SELECT friend_code FROM users WHERE id=$1`, c.ID).Scan(&code); err != nil {
		t.Fatal(err)
	}
	rq := h.execute(h.a, social.Operation{Action: "request", Code: code})
	h.execute(ctx, social.Operation{Action: "accept_request", ID: rq.ID})
	for _, id := range []string{h.bid, c.ID} {
		h.execute(h.a, social.Operation{Action: "tags", UserID: id, Tags: []string{"Коллега"}})
	}
	r := h.say(h.a, "create_meeting", `{"title":"Встреча","friend_tag":"Коллега","date":{"type":"relative_day","offset":1},"time":"16:00"}`, "Создай встречу с коллегой")
	if !r.Pending || !strings.Contains(r.Reply, "Матвей Лапин") || !strings.Contains(r.Reply, "Алиса Проверка") || h.count() != 0 {
		t.Fatal(r)
	}
	h.personal(h.b, domain.Day(h.now.AddDate(0, 0, 1)), 16, 60)
	r = h.say(h.a, "create_meeting", `{"title":"Встреча","friends":["Матвей Лапин"],"date":{"type":"relative_day","offset":1},"time":"16:00"}`, "С Матвеем завтра в 16")
	if r.Scheduling == nil {
		t.Fatal(r)
	}
	h.execute(h.a, social.Operation{Action: "remove_friend", UserID: h.bid})
	r = h.say(h.a, "choose_time", `{"option":1}`, "Первый вариант")
	if !r.Pending || h.count() != 0 || !strings.Contains(r.Reply, "не найден") {
		t.Fatal(r)
	}
}
