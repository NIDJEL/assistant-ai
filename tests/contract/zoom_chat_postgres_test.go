package contract

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/NIDJEL/assistant-ai/internal/api"
	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/clock"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/llm"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/search"
	"github.com/google/uuid"
	"io"
	"log/slog"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestZoomChatSignedWebhookLifecycleAndPersistence(t *testing.T) {
	db, auth, _, now := pgFixture(t)
	engine, _ := search.New()
	cal := &calendar.Service{Store: db, Search: engine, Location: now.Location()}
	service := assistant.New(db, nlp.Hybrid{}, cal)
	app := &api.API{Config: config.Config{Root: root(), PublicURL: "https://rhythm.test", ZoomWebhookSecret: "test-webhook-secret"}, Auth: auth, Clock: clock.Func(func() time.Time { return now }), Calendar: cal, Assistant: service, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	router := app.Router()
	alice, aliceCookie := registerHTTP(t, router, "zoom-alice@example.test")
	bob, bobCookie := registerHTTP(t, router, "zoom-bob@example.test")
	ctx := repository.WithUser(t.Context(), alice)
	other := repository.WithUser(t.Context(), bob)
	if err := db.SaveCloudConnection(ctx, repository.CloudConnection{Provider: "zoom", Subject: "host-a", Status: "connected"}); err != nil {
		t.Fatal(err)
	}
	var meetings []domain.Event
	if err := db.Write(ctx, func(tx repository.Tx) error {
		for i := 1; i <= 2; i++ {
			day, _ := domain.ParseDate("2026-10-12")
			clock, _ := domain.ParseTime(fmt.Sprintf("1%d:00", i))
			e, err := tx.Create(ctx, domain.Event{Kind: domain.EventMeeting, Title: fmt.Sprintf("Zoom %d", i), Date: day, StartTime: clock, Duration: 30 * time.Minute, Status: domain.StatusOpen, Priority: domain.PriorityNormal, MeetingURL: fmt.Sprintf("https://zoom.us/j/100%d?pwd=opaque", i), EventDetails: domain.EventDetails{ConferenceProvider: "zoom"}}, now)
			if err != nil {
				return err
			}
			meetings = append(meetings, *e)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for i, e := range meetings {
		if err := db.SaveCloudLink(ctx, "zoom", "", repository.CloudLink{EventID: e.ID, RemoteID: fmt.Sprintf("100%d", i+1)}); err != nil {
			t.Fatal(err)
		}
	}
	response := domain.Response{Action: domain.ActionList, Events: meetings}
	if err := service.RefreshZoomCards(ctx, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.ZoomMeetings) != 2 || response.ZoomMeetings[0].Status != "scheduled" || response.ZoomMeetings[0].JoinURL == response.ZoomMeetings[1].JoinURL {
		t.Fatal("scheduled cards", response)
	}
	sid, rid := uuid.NewString(), uuid.NewString()
	if err := db.Write(ctx, func(tx repository.Tx) error {
		if err := tx.SaveState(ctx, sid, domain.State{}, now); err != nil {
			return err
		}
		return tx.SaveExchange(ctx, sid, rid, "Покажи встречи", response, now)
	}); err != nil {
		t.Fatal(err)
	}
	history, _, err := service.History(ctx, sid)
	if err != nil || len(history) != 1 || len(history[0].Response.ZoomMeetings) != 2 {
		t.Fatal("durable cards", history, err)
	}
	post := func(event, mid, run, host string) int {
		payload := map[string]any{"event": event, "event_ts": now.UnixMilli(), "payload": map[string]any{"object": map[string]any{"id": mid, "uuid": run, "host_id": host, "start_time": now.UTC().Format(time.RFC3339)}}}
		raw, _ := json.Marshal(payload)
		stamp := strconv.FormatInt(now.Unix(), 10)
		mac := hmac.New(sha256.New, []byte(app.Config.ZoomWebhookSecret))
		mac.Write([]byte("v0:" + stamp + ":" + string(raw)))
		r := httptest.NewRequest("POST", "/api/webhooks/zoom", bytes.NewReader(raw))
		r.Header.Set("x-zm-request-timestamp", stamp)
		r.Header.Set("x-zm-signature", "v0="+hex.EncodeToString(mac.Sum(nil)))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w.Code
	}
	for _, tc := range [][4]string{{"meeting.started", "9999", "unknown", "host-a"}, {"meeting.started", "1001", "forged-host", "host-b"}} {
		if code := post(tc[0], tc[1], tc[2], tc[3]); code != 204 {
			t.Fatal(code)
		}
	}
	list, _ := db.ChatNotifications(ctx)
	if len(list) != 0 {
		t.Fatal("unknown meeting created notification")
	}
	var wg sync.WaitGroup
	codes := make(chan int, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- post("meeting.started", "1001", "run-one", "host-a") }()
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != 204 {
			t.Fatal("concurrent delivery", code)
		}
	}
	list, err = db.ChatNotifications(ctx)
	if err != nil || len(list) != 1 || list[0].Read || list[0].Response.ZoomMeetings[0].Status != "started" {
		t.Fatal(list, err)
	}
	foreign, _ := db.ChatNotifications(other)
	if len(foreign) != 0 {
		t.Fatal("user leak")
	}
	call(t, router, nil, "GET", "/api/chat/notifications", nil, 401)
	data, _ := call(t, router, aliceCookie, "GET", "/api/chat/notifications", nil, 200)
	if len(data["exchanges"].([]any)) != 1 {
		t.Fatal(data)
	}
	id := list[0].Response.RequestID
	call(t, router, bobCookie, "POST", "/api/chat/notifications/read", map[string]any{"ids": []string{id}}, 204)
	list, _ = db.ChatNotifications(ctx)
	if list[0].Read {
		t.Fatal("cross-user read")
	}
	call(t, router, aliceCookie, "POST", "/api/chat/notifications/read", map[string]any{"ids": []string{id}}, 204)
	list, _ = db.ChatNotifications(ctx)
	if !list[0].Read {
		t.Fatal("read not persisted")
	}
	if code := post("meeting.ended", "1001", "run-one", "host-a"); code != 204 {
		t.Fatal(code)
	}
	if code := post("meeting.started", "1001", "run-one", "host-a"); code != 204 {
		t.Fatal(code)
	}
	list, _ = db.ChatNotifications(ctx)
	if len(list) != 1 {
		t.Fatal("duplicate after ended")
	}
	history, _, err = service.History(ctx, sid)
	if err != nil || history[0].Response.ZoomMeetings[0].Status != "ended" {
		t.Fatal("ended history", history, err)
	}
	if code := post("meeting.started", "1001", "run-two", "host-a"); code != 204 {
		t.Fatal(code)
	}
	list, _ = db.ChatNotifications(ctx)
	if len(list) != 2 {
		t.Fatal("recurring run missing")
	}
	if err = service.RefreshZoomCards(ctx, &list[0].Response); err != nil || list[0].Response.ZoomMeetings[0].Status != "ended" {
		t.Fatal("old run rewritten", err)
	}
	post("meeting.ended", "1002", "end-before-start", "host-a")
	post("meeting.started", "1002", "end-before-start", "host-a")
	list, _ = db.ChatNotifications(ctx)
	if len(list) != 2 {
		t.Fatal("out of order start announced after end")
	}
	if err = db.DisconnectCloud(ctx, "zoom"); err != nil {
		t.Fatal(err)
	}
	post("meeting.started", "1001", "disconnected", "host-a")
	list, _ = db.ChatNotifications(ctx)
	if len(list) != 2 {
		t.Fatal("disconnected account notified")
	}
	// New service instance loads the same persisted attachment and run state.
	service = assistant.New(db, nlp.Hybrid{}, cal)
	history, _, err = service.History(ctx, sid)
	if err != nil || len(history[0].Response.ZoomMeetings) != 2 {
		t.Fatal("restart persistence", err)
	}
}

type zoomReadOperation struct{}

func (zoomReadOperation) Interpret(context.Context, string, time.Time) (domain.Command, error) {
	return domain.Command{}, nil
}
func (zoomReadOperation) InterpretOperation(_ context.Context, _ string, _ nlu.Context) (nlu.Operation, error) {
	return nlu.Decode([]byte(`{"intent":"list","arguments":{"kind":"meeting","conference":"zoom","next":true},"confidence":"high","clarification":null}`))
}

func TestZoomChatSimpleReadChoosesNextConferenceForTextAndVoice(t *testing.T) {
	db, auth, _, now := pgFixture(t)
	u, err := auth.Register(t.Context(), "Zoom reader", "zoom-reader@example.test", "correct password 123", "correct password 123")
	if err != nil {
		t.Fatal(err)
	}
	ctx := repository.WithUser(t.Context(), u.ID)
	engine, _ := search.New()
	cal := &calendar.Service{Store: db, Search: engine, Location: now.Location()}
	service := assistant.New(db, zoomReadOperation{}, cal)
	var want int64
	if err = db.Write(ctx, func(tx repository.Tx) error {
		for i := 0; i < 3; i++ {
			clock, _ := domain.ParseTime(fmt.Sprintf("1%d:00", i))
			e := domain.Event{Kind: domain.EventMeeting, Title: "Команда", Date: domain.Day(now.Add(24 * time.Hour)), StartTime: clock, Duration: 30 * time.Minute, Status: domain.StatusOpen, Priority: domain.PriorityNormal}
			if i > 0 {
				e.ConferenceProvider = "zoom"
				e.MeetingURL = fmt.Sprintf("https://zoom.us/j/%d", 1000+i)
			}
			saved, err := tx.Create(ctx, e, now)
			if err != nil {
				return err
			}
			if i == 1 {
				want = saved.ID
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"text", "voice"} {
		sid := uuid.NewString()
		r, err := service.Respond(llm.WithSource(ctx, source), "Какая у меня следующая Zoom-встреча?", sid, uuid.NewString(), now)
		if err != nil || len(r.Events) != 1 || r.Events[0].ID != want || len(r.ZoomMeetings) != 1 || r.ZoomMeetings[0].JoinURL != "https://zoom.us/j/1001" || r.ZoomMeetings[0].Status != "scheduled" {
			t.Fatal(source, r, err)
		}
		history, _, err := service.History(ctx, sid)
		if err != nil || len(history) != 1 || len(history[0].Response.ZoomMeetings) != 1 {
			t.Fatal("simple history", err)
		}
	}
}
