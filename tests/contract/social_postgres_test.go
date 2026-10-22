package contract

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/search"
	"github.com/NIDJEL/assistant-ai/internal/social"
	"github.com/google/uuid"
)

func TestPostgresFriendsInvitationsAvailabilityRescheduleAndPrivacy(t *testing.T) {
	db, _, router, now := pgFixture(t)
	aid, a := registerHTTP(t, router, "planning-a@example.test")
	bid, b := registerHTTP(t, router, "planning-b@example.test")
	cid, c := registerHTTP(t, router, "planning-c@example.test")
	op := func(cookie *http.Cookie, action string, fields map[string]any, status int) map[string]any {
		t.Helper()
		if fields == nil {
			fields = map[string]any{}
		}
		fields["action"] = action
		if fields["request_id"] == nil {
			fields["request_id"] = uuid.NewString()
		}
		result, _ := call(t, router, cookie, "POST", "/api/social/operations", fields, status)
		return result
	}
	people := func(cookie *http.Cookie) map[string]any {
		r, _ := call(t, router, cookie, "GET", "/api/social/people", nil, 200)
		return r
	}
	ap := people(a)
	bp := people(b)
	cp := people(c)
	code := func(p map[string]any) string { return p["profile"].(map[string]any)["code"].(string) }
	if code(ap) == code(bp) || len(code(ap)) != 9 {
		t.Fatal("invalid/nonunique code")
	}
	op(a, "request", map[string]any{"code": code(ap)}, 422)
	request := map[string]any{"code": code(ap), "request_id": uuid.NewString()}
	req := op(b, "request", request, 200)
	op(b, "request", request, 200)
	op(a, "request", map[string]any{"code": code(bp)}, 409)
	op(c, "accept_request", map[string]any{"id": req["id"]}, 404)
	op(b, "accept_request", map[string]any{"id": req["id"]}, 404)
	op(a, "accept_request", map[string]any{"id": req["id"]}, 200)
	if len(people(a)["friends"].([]any)) != 1 || len(people(b)["friends"].([]any)) != 1 {
		t.Fatal("friendship not mutual")
	}
	op(a, "tags", map[string]any{"user_id": bid, "tags": []string{"Коллега", "Лучший друг"}}, 200)
	bf := people(b)["friends"].([]any)[0].(map[string]any)
	if len(bf["tags"].([]any)) != 0 || bf["email"] != nil {
		t.Fatal("private tag/email leaked", bf)
	}
	op(a, "settings", map[string]any{"share_email": true}, 200)
	bf = people(b)["friends"].([]any)[0].(map[string]any)
	if bf["email"] != "planning-a@example.test" {
		t.Fatal("email opt-in failed")
	}
	req2 := op(c, "request", map[string]any{"code": code(ap)}, 200)
	op(a, "accept_request", map[string]any{"id": req2["id"]}, 200)
	day := now.AddDate(0, 0, 1).Format("2006-01-02")
	private, _ := call(t, router, b, "POST", "/api/events", map[string]any{"kind": "meeting", "title": "PRIVATE MEDICAL TITLE", "description": "PRIVATE BODY", "date": day, "time": "15:00", "duration_minutes": 60}, 201)
	slot := social.Slot{Start: time.Date(now.Year(), now.Month(), now.Day()+1, 15, 0, 0, 0, now.Location()), End: time.Date(now.Year(), now.Month(), now.Day()+1, 15, 30, 0, 0, now.Location())}
	check, _ := call(t, router, a, "POST", "/api/social/availability", map[string]any{"friend_ids": []string{bid, cid}, "slot": slot}, 200)
	raw, _ := json.Marshal(check)
	if strings.Contains(string(raw), "PRIVATE") || strings.Contains(string(raw), fmt.Sprint(private["id"])) && strings.Contains(string(raw), "event_id") {
		t.Fatal("availability leaked private event", string(raw))
	}
	if len(check["slots"].([]any)) == 0 {
		t.Fatal("no alternatives")
	}
	event := map[string]any{"kind": "meeting", "title": "План команды", "date": day, "time": "15:00", "duration_minutes": 30}
	op(a, "create_meeting", map[string]any{"event": event, "friend_ids": []string{bid, cid}}, 409)
	event["time"] = "16:00"
	creation := map[string]any{"event": event, "friend_ids": []string{bid, cid}, "request_id": uuid.NewString()}
	created := op(a, "create_meeting", creation, 200)
	op(a, "create_meeting", creation, 200)
	eid := int64(created["event_id"].(float64))
	path := fmt.Sprintf("/api/events/%d", eid)
	call(t, router, b, "GET", path, nil, 404)
	op(a, "respond", map[string]any{"event_id": eid, "version": 1, "status": "accepted"}, 422)
	op(b, "respond", map[string]any{"event_id": eid, "version": 1, "status": "accepted", "user_id": cid}, 403)
	op(b, "respond", map[string]any{"event_id": eid, "version": 1, "status": "accepted"}, 200)
	var cStatus string
	if err := db.DB.QueryRow(`SELECT status FROM meeting_participants WHERE event_id=$1 AND user_id=$2`, eid, cid).Scan(&cStatus); err != nil || cStatus != "pending" {
		t.Fatal("response spoofed", cStatus, err)
	}
	call(t, router, b, "GET", path, nil, 200)
	call(t, router, b, "PATCH", path, map[string]any{"title": "stolen"}, 403)
	call(t, router, b, "DELETE", path, nil, 403)
	moved := social.Slot{Start: slot.Start.Add(2 * time.Hour), End: slot.End.Add(2 * time.Hour)}
	proposal := op(b, "propose", map[string]any{"event_id": eid, "version": 1, "slots": []social.Slot{moved}}, 200)
	op(b, "resolve_proposal", map[string]any{"event_id": eid, "version": 1, "id": proposal["id"], "status": "accepted"}, 404)
	// Recheck at confirmation, after a new conflict appeared since the proposal.
	conflict, _ := call(t, router, c, "POST", "/api/events", map[string]any{"kind": "meeting", "title": "PRIVATE OTHER", "date": day, "time": "17:00", "duration_minutes": 30}, 201)
	resolution := map[string]any{"event_id": eid, "version": 1, "id": proposal["id"], "status": "accepted", "request_id": uuid.NewString()}
	op(a, "resolve_proposal", resolution, 409)
	call(t, router, c, "DELETE", fmt.Sprintf("/api/events/%.0f", conflict["id"]), nil, 204)
	op(a, "resolve_proposal", resolution, 200)
	call(t, router, b, "GET", path, nil, 404) // old acceptance does not survive a move.
	op(b, "respond", map[string]any{"event_id": eid, "version": 1, "status": "accepted"}, 409)
	response := map[string]any{"event_id": eid, "version": 2, "status": "accepted", "request_id": uuid.NewString()}
	op(b, "respond", response, 200)
	op(b, "respond", response, 200)
	op(c, "respond", map[string]any{"event_id": eid, "version": 2, "status": "declined"}, 200)
	got, _ := call(t, router, b, "GET", path, nil, 200)
	if got["time"] != "17:00" {
		t.Fatal("canonical time not updated", got)
	}
	participants := got["participants"].([]any)
	if len(participants) != 2 {
		t.Fatal("calendar participants missing", got)
	}
	for _, p := range participants {
		if p.(map[string]any)["email"] != nil {
			t.Fatal("calendar participant email leaked", p)
		}
	}
	var count int
	if err := db.DB.QueryRow(`SELECT count(*) FROM events WHERE title='План команды'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate canonical event", count, err)
	}
	if err := db.DB.QueryRow(`SELECT count(*) FROM invitation_responses WHERE event_id=$1 AND user_id=$2 AND version=2`, eid, bid).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate RSVP history", count, err)
	}
	notifications, _ := call(t, router, b, "GET", "/api/chat/notifications", nil, 200)
	if len(notifications["exchanges"].([]any)) < 2 {
		t.Fatal("notifications not persisted")
	}
	op(a, "remove_friend", map[string]any{"user_id": bid}, 200)
	call(t, router, b, "GET", path, nil, 200)
	op(a, "invite", map[string]any{"event_id": eid, "version": 2, "friend_ids": []string{bid}}, 403)
	op(a, "cancel_meeting", map[string]any{"event_id": eid, "version": 2}, 200)
	call(t, router, b, "GET", path, nil, 404)
	shared, _ := call(t, router, b, "GET", "/api/social/meetings", nil, 200)
	if len(shared["meetings"].([]any)) != 1 || shared["meetings"].([]any)[0].(map[string]any)["cancelled"] != true {
		t.Fatal("cancel history lost")
	}
	// Guess rate limits survive failed requests and are isolated by account.
	for i := 0; i < 19; i++ {
		op(c, "request", map[string]any{"code": "ZZZZ-ZZZZ"}, 404)
	}
	op(c, "request", map[string]any{"code": "ZZZZ-ZZZZ"}, 429)
	_ = aid
	_ = cp
}

func TestPostgresSocialConcurrentRSVPAndFriendRequest(t *testing.T) {
	db, auth, _, now := pgFixture(t)
	u, _ := auth.Register(t.Context(), "A", "race-a@example.test", "test password 12345", "test password 12345")
	v, _ := auth.Register(t.Context(), "B", "race-b@example.test", "test password 12345", "test password 12345")
	a, b := repository.WithUser(t.Context(), u.ID), repository.WithUser(t.Context(), v.ID)
	loc := now.Location()
	var code string
	if err := db.DB.QueryRow(`SELECT friend_code FROM users WHERE id=$1`, v.ID).Scan(&code); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	success := 0
	var mu sync.Mutex
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := db.Write(a, func(tx repository.Tx) error {
				s, e := social.In(a, tx, loc)
				if e != nil {
					return e
				}
				_, e = s.Execute(social.WithTx(a, tx), social.Operation{Action: "request", Code: code, RequestID: uuid.NewString()}, now)
				return e
			})
			if err == nil {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if success != 1 {
		t.Fatal("concurrent duplicate requests", success)
	}
	var requestID string
	db.DB.QueryRow(`SELECT id FROM friend_requests WHERE sender_id=$1`, u.ID).Scan(&requestID)
	execute := func(ctx context.Context, op social.Operation) social.Result {
		t.Helper()
		op.RequestID = uuid.NewString()
		var result social.Result
		err := db.Write(ctx, func(tx repository.Tx) error {
			s, e := social.In(ctx, tx, loc)
			if e != nil {
				return e
			}
			result, e = s.Execute(social.WithTx(ctx, tx), op, now)
			return e
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	execute(b, social.Operation{Action: "accept_request", ID: requestID})
	e := domain.Event{Kind: domain.EventMeeting, Title: "Race", Date: domain.Day(now.AddDate(0, 0, 1)), StartTime: &domain.TimeOfDay{Hour: 15}, Duration: 30 * time.Minute, Status: domain.StatusOpen, Priority: domain.PriorityNormal}
	result := execute(a, social.Operation{Action: "create_meeting", Event: &e, FriendIDs: []string{v.ID}})
	success = 0
	rid := uuid.NewString()
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := db.Write(b, func(tx repository.Tx) error {
				s, _ := social.In(b, tx, loc)
				_, err := s.Execute(social.WithTx(b, tx), social.Operation{Action: "respond", EventID: result.EventID, Status: "accepted", Version: 1, RequestID: rid}, now)
				return err
			})
			if err != nil {
				t.Error(err)
			} else {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	var count int
	db.DB.QueryRow(`SELECT count(*) FROM invitation_responses WHERE event_id=$1`, result.EventID).Scan(&count)
	if success != 8 || count != 1 {
		t.Fatal(success, count)
	}
}

type socialInterpreter struct{ op nlu.Operation }

func (s socialInterpreter) Interpret(context.Context, string, time.Time) (domain.Command, error) {
	return domain.Command{}, fmt.Errorf("legacy must not run")
}
func (s socialInterpreter) InterpretOperation(context.Context, string, nlu.Context) (nlu.Operation, error) {
	return s.op, nil
}
func TestPostgresSimpleLLMSocialAtomicityAndContinuation(t *testing.T) {
	db, auth, _, now := pgFixture(t)
	u, _ := auth.Register(t.Context(), "Никита", "nlu-a@example.test", "test password 12345", "test password 12345")
	v, _ := auth.Register(t.Context(), "Алиса", "nlu-b@example.test", "test password 12345", "test password 12345")
	a, b := repository.WithUser(t.Context(), u.ID), repository.WithUser(t.Context(), v.ID)
	var code string
	db.DB.QueryRow(`SELECT friend_code FROM users WHERE id=$1`, v.ID).Scan(&code)
	engine, _ := search.New()
	cal := &calendar.Service{Store: db, Location: now.Location(), Search: engine}
	s := assistant.New(db, socialInterpreter{}, cal)
	set := func(intent string, args any) {
		t.Helper()
		raw, _ := json.Marshal(args)
		op, e := nlu.Decode([]byte(fmt.Sprintf(`{"intent":%q,"arguments":%s,"confidence":"high","clarification":null}`, intent, raw)))
		if e != nil {
			t.Fatal(e)
		}
		s.Interpreter = socialInterpreter{op}
	}
	set("friend_request", map[string]any{"code": code})
	sid, rid := uuid.NewString(), uuid.NewString()
	r, err := s.Respond(a, "Добавь друга", sid, rid, now)
	if err != nil || !strings.Contains(r.Reply, "отправлена") {
		t.Fatal(r, err)
	}
	s.Respond(a, "Добавь друга", sid, rid, now)
	var requestID string
	db.DB.QueryRow(`SELECT id FROM friend_requests WHERE sender_id=$1`, u.ID).Scan(&requestID)
	err = db.Write(b, func(tx repository.Tx) error {
		session, _ := social.In(b, tx, now.Location())
		_, e := session.Execute(social.WithTx(b, tx), social.Operation{Action: "accept_request", ID: requestID, RequestID: uuid.NewString()}, now)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	set("create_meeting", map[string]any{"title": "Встреча с Алисой", "friends": []string{"Алиса"}})
	r, err = s.Respond(a, "Назначь встречу с Алисой", sid, uuid.NewString(), now)
	if err != nil || !r.Pending {
		t.Fatal("missing date must clarify", r, err)
	}
	set("create_meeting", map[string]any{"title": "Встреча с Алисой", "friends": []string{"Алиса"}, "date": map[string]any{"type": "relative_day", "offset": 1}, "time": "15:00"})
	rid = uuid.NewString()
	r, err = s.Respond(a, "Завтра в 15", sid, rid, now)
	if err != nil || r.Social == nil || r.Social.EventID == 0 {
		t.Fatal(r, err)
	}
	s.Respond(a, "Завтра в 15", sid, rid, now)
	var count int
	db.DB.QueryRow(`SELECT count(*) FROM collaborative_meetings`).Scan(&count)
	if count != 1 {
		t.Fatal("chat retry duplicated meeting", count)
	}
	set("respond_invitation", map[string]any{"friends": []string{"Никита"}, "status": "accepted"})
	r, err = s.Respond(b, "Прими приглашение от Никиты", uuid.NewString(), uuid.NewString(), now)
	if err != nil || !strings.Contains(r.Reply, "подтвердили") {
		t.Fatal(r, err)
	}
}
