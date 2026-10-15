package contract

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/api"
	"github.com/NIDJEL/assistant-ai/internal/auth"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/clock"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/notifications"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/storage/postgres"
	webpush "github.com/SherClockHolmes/webpush-go"
)

func pushFixture(t *testing.T) (*postgres.Database, *auth.Service, http.Handler, time.Time) {
	t.Helper()
	db, authService, _, now := pgFixture(t)
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	cal := &calendar.Service{Store: db, Location: now.Location()}
	app := &api.API{Auth: authService, Calendar: cal, Clock: clock.Func(func() time.Time { return now }), Config: config.Config{Root: root(), PublicURL: "https://gena.test", WebPush: notifications.Config{PublicKey: public, PrivateKey: private, Subject: "mailto:push@example.test"}}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	return db, authService, app.Router(), now
}
func pushInput(t *testing.T, endpoint string) notifications.Subscribe {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s := notifications.Subscribe{Endpoint: "https://push.example.test/" + endpoint, Platform: "Windows"}
	s.Keys.P256DH = base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
	s.Keys.Auth = base64.RawURLEncoding.EncodeToString(make([]byte, 16))
	return s
}
func TestPostgresPushSubscriptionAPIAuthOwnershipAndDuplicates(t *testing.T) {
	db, _, router, now := pushFixture(t)
	uid, cookie := registerHTTP(t, router, "push-owner@example.test")
	_, other := registerHTTP(t, router, "push-other@example.test")
	for _, path := range []string{"/api/notifications/push/config", "/api/notifications/push/subscriptions"} {
		call(t, router, nil, "GET", path, nil, 401)
	}
	call(t, router, nil, "POST", "/api/notifications/push/subscriptions", pushInput(t, "unauth"), 401)
	cfg, w := call(t, router, cookie, "GET", "/api/notifications/push/config", nil, 200)
	if cfg["enabled"] != true || cfg["public_key"] == "" || strings.Contains(w.Body.String(), "private") {
		t.Fatal("unsafe config response")
	}
	input := pushInput(t, "owner")
	created, w := call(t, router, cookie, "POST", "/api/notifications/push/subscriptions", input, 201)
	id := created["subscription"].(map[string]any)["id"].(string)
	if strings.Contains(w.Body.String(), "endpoint") || strings.Contains(w.Body.String(), "p256dh") || strings.Contains(w.Body.String(), input.Keys.Auth) {
		t.Fatal("keys leaked")
	}
	duplicate, _ := call(t, router, cookie, "POST", "/api/notifications/push/subscriptions", input, 201)
	if duplicate["subscription"].(map[string]any)["id"] != id {
		t.Fatal("duplicate endpoint inserted")
	}
	call(t, router, other, "POST", "/api/notifications/push/subscriptions", input, 409)
	call(t, router, other, "DELETE", "/api/notifications/push/subscriptions/"+id, nil, 404)
	listed, _ := call(t, router, other, "GET", "/api/notifications/push/subscriptions", nil, 200)
	if len(listed["subscriptions"].([]any)) != 0 {
		t.Fatal("other user's devices exposed")
	}
	for _, bad := range []map[string]any{{"endpoint": "http://bad", "keys": input.Keys, "platform": "Windows"}, {"endpoint": input.Endpoint, "keys": map[string]string{"auth": "x", "p256dh": "x"}, "platform": "Windows"}, {"endpoint": input.Endpoint, "keys": input.Keys, "platform": "Windows", "extra": "secret"}} {
		call(t, router, cookie, "POST", "/api/notifications/push/subscriptions", bad, 422)
	}
	call(t, router, cookie, "POST", "/api/notifications/push/test", map[string]string{"subscription_id": id}, 202)
	call(t, router, cookie, "POST", "/api/notifications/push/test", map[string]string{"subscription_id": id}, 429)
	call(t, router, other, "POST", "/api/notifications/push/test", map[string]string{"subscription_id": id}, 404)
	call(t, router, cookie, "DELETE", "/api/notifications/push/subscriptions/"+id, nil, 204)
	devices, err := db.Subscriptions(repository.WithUser(t.Context(), uid))
	if err != nil || len(devices) != 1 || devices[0].DisabledAt == nil {
		t.Fatal("unsubscribe lost", err)
	}
	if _, err = db.Subscribe(repository.WithUser(t.Context(), uid), input, "agent", now); err != nil {
		t.Fatal(err)
	}
	var status string
	db.DB.QueryRow("SELECT status FROM notification_deliveries LIMIT 1").Scan(&status)
	if status != "cancelled" {
		t.Fatal("queued test not cancelled", status)
	}
}

type pushFake struct {
	mu       sync.Mutex
	payloads []notifications.Payload
	devices  []string
	status   int
}

func (s *pushFake) Send(ctx context.Context, sub notifications.Subscription, p notifications.Payload) notifications.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payloads = append(s.payloads, p)
	s.devices = append(s.devices, sub.ID)
	status := s.status
	if status == 0 {
		status = 201
	}
	return notifications.Result{Status: status}
}
func createPushReminder(t *testing.T, db *postgres.Database, uid string, now time.Time, recurrence *domain.Recurrence) *domain.Event {
	t.Helper()
	cal := &calendar.Service{Store: db, Location: now.Location()}
	day := domain.Day(now)
	clock := &domain.TimeOfDay{Hour: now.Hour(), Minute: now.Minute()}
	e, err := cal.Create(repository.WithUser(t.Context(), uid), domain.Event{Kind: domain.EventReminder, Title: "Снять чайник с плиты", Date: day, StartTime: clock, EventDetails: domain.EventDetails{Recurrence: recurrence}}, now.Add(-time.Minute), false)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func TestPostgresPushWorkerConcurrencyMultiDeviceAndReadOnlyPolling(t *testing.T) {
	db, _, router, now := pushFixture(t)
	uid, _ := registerHTTP(t, router, "worker@example.test")
	other, _ := registerHTTP(t, router, "isolation@example.test")
	ctx := repository.WithUser(t.Context(), uid)
	for _, endpoint := range []string{"phone", "desktop"} {
		if _, err := db.Subscribe(ctx, pushInput(t, endpoint), "agent", now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Subscribe(repository.WithUser(t.Context(), other), pushInput(t, "other"), "agent", now); err != nil {
		t.Fatal(err)
	}
	clock := &domain.TimeOfDay{Hour: now.Hour(), Minute: now.Minute()}
	e := createPushReminder(t, db, uid, now, &domain.Recurrence{Frequency: "day", Interval: 1, LocalTime: clock, Timezone: now.Location().String()})
	cal := &calendar.Service{Store: db, Location: now.Location()}
	if events, err := cal.Due(ctx, now); err != nil || len(events) != 0 {
		t.Fatal("poll delivered without worker", events, err)
	}
	stored, _ := cal.Get(ctx, e.ID)
	if !stored.OccurrenceAt.Equal(*e.OccurrenceAt) {
		t.Fatal("poll advanced")
	}
	sender := &pushFake{}
	worker := &notifications.Worker{Store: db, Sender: sender, Location: now.Location()}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := worker.Tick(t.Context(), now); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if len(sender.payloads) != 2 || sender.devices[0] == sender.devices[1] {
		t.Fatal("multi-device duplicates", len(sender.payloads))
	}
	for _, p := range sender.payloads {
		if p.UserID != uid || p.EventID != e.ID || p.Title != "Гена" || p.Body != e.Title {
			t.Fatal("wrong recipient or payload")
		}
	}
	stored, _ = cal.Get(ctx, e.ID)
	if !stored.OccurrenceAt.Equal(e.OccurrenceAt.AddDate(0, 0, 1)) || !stored.Recurrence.Anchor.Equal(e.Recurrence.Anchor) {
		t.Fatal("recurrence drift")
	}
	for range 4 {
		events, err := cal.Due(ctx, now)
		if err != nil || len(events) != 1 {
			t.Fatal(events, err)
		}
	}
	storedAgain, _ := cal.Get(ctx, e.ID)
	if !storedAgain.OccurrenceAt.Equal(*stored.OccurrenceAt) {
		t.Fatal("poll advanced twice")
	}
	var count int
	db.DB.QueryRow("SELECT count(*) FROM notification_occurrences").Scan(&count)
	if count != 1 {
		t.Fatal("duplicate occurrence", count)
	}
	db.DB.QueryRow("SELECT count(*) FROM notification_deliveries WHERE status='success'").Scan(&count)
	if count != 2 {
		t.Fatal("bad delivery count", count)
	}
}

func TestPostgresOneShotReminderArchivesWithoutCancellingNotifications(t *testing.T) {
	db, _, router, now := pushFixture(t)
	uid, _ := registerHTTP(t, router, "one-shot@example.test")
	ctx := repository.WithUser(t.Context(), uid)
	cal := &calendar.Service{Store: db, Location: now.Location()}
	for _, endpoint := range []string{"phone", "desktop"} {
		if _, err := db.Subscribe(ctx, pushInput(t, endpoint), "agent", now); err != nil {
			t.Fatal(err)
		}
	}
	e := createPushReminder(t, db, uid, now, nil)
	sender := &pushFake{status: 503}
	worker := notifications.Worker{Store: db, Sender: sender, Location: now.Location()}
	if err := worker.Tick(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	stored, err := cal.Get(ctx, e.ID)
	if err != nil || stored.Status != domain.StatusDone {
		t.Fatal("fired reminder still active", stored, err)
	}
	if events, err := cal.Due(ctx, now); err != nil || len(events) != 1 || events[0].ID != e.ID {
		t.Fatal("archive hid the in-app notification", events, err)
	}
	sender.status = 201
	if err := worker.Tick(t.Context(), now.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	var successes, occurrences int
	db.DB.QueryRow("SELECT count(*) FROM notification_deliveries WHERE status='success'").Scan(&successes)
	db.DB.QueryRow("SELECT count(*) FROM notification_occurrences").Scan(&occurrences)
	if successes != 2 || occurrences != 1 || len(sender.payloads) != 4 {
		t.Fatal("archive cancelled delivery, retry or deduplication", successes, occurrences, len(sender.payloads))
	}
}

func TestPostgresPreviouslyFiredReminderArchivesWithoutResending(t *testing.T) {
	db, _, router, now := pushFixture(t)
	uid, _ := registerHTTP(t, router, "previously-fired@example.test")
	ctx := repository.WithUser(t.Context(), uid)
	e := createPushReminder(t, db, uid, now.Add(-10*time.Minute), nil)
	start, _, _ := e.Interval(now.Location())
	e.LastNotificationAt = &start
	if err := db.Write(ctx, func(tx repository.Tx) error {
		_, err := tx.Update(ctx, *e, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	count, err := db.ProcessDue(t.Context(), now, now.Location())
	stored, getErr := (&calendar.Service{Store: db, Location: now.Location()}).Get(ctx, e.ID)
	if err != nil || getErr != nil || count != 0 || stored.Status != domain.StatusDone {
		t.Fatal("previously fired reminder remains active or replayed", count, stored, err, getErr)
	}
}
func reopenedPushDB(t *testing.T, db *postgres.Database) *postgres.Database {
	t.Helper()
	var schema string
	if err := db.DB.QueryRow("SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	newDB, err := postgres.Open(t.Context(), u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { newDB.Close() })
	return newDB
}
func TestPostgresPushRetryRestartAndDeadEndpoint(t *testing.T) {
	db, _, router, now := pushFixture(t)
	uid, _ := registerHTTP(t, router, "retry@example.test")
	ctx := repository.WithUser(t.Context(), uid)
	sub, err := db.Subscribe(ctx, pushInput(t, "retry"), "agent", now)
	if err != nil {
		t.Fatal(err)
	}
	createPushReminder(t, db, uid, now, nil)
	sender := &pushFake{status: 503}
	worker := &notifications.Worker{Store: db, Sender: sender, Location: now.Location()}
	if err = worker.Tick(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	var status string
	var attempts int
	var next time.Time
	if err = db.DB.QueryRow("SELECT status,attempts,next_attempt_at FROM notification_deliveries").Scan(&status, &attempts, &next); err != nil || status != "retry" || attempts != 1 || !next.Equal(now.Add(30*time.Second)) {
		t.Fatal(status, attempts, next, err)
	}
	reopened := reopenedPushDB(t, db)
	worker.Store = reopened
	if err = worker.Tick(t.Context(), now.Add(29*time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(sender.payloads) != 1 {
		t.Fatal("retried early")
	}
	sender.status = 410
	if err = worker.Tick(t.Context(), next); err != nil {
		t.Fatal(err)
	}
	devices, _ := reopened.Subscriptions(ctx)
	if devices[0].DisabledAt == nil || devices[0].FailureCount != 2 {
		t.Fatal("dead endpoint kept active")
	}
	if err = worker.Tick(t.Context(), next.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if len(sender.payloads) != 2 {
		t.Fatal("dead endpoint retried")
	}
	if _, err = reopened.Subscribe(ctx, pushInput(t, "retry"), "agent", next); err != nil {
		t.Fatal(err)
	}
	if err = reopened.QueueTest(ctx, sub.ID, next); err != nil {
		t.Fatal(err)
	}
	sender.status = 500
	for i := 0; i < notifications.MaxAttempts; i++ {
		if err = reopened.DB.QueryRow("SELECT next_attempt_at FROM notification_deliveries WHERE status IN ('pending','retry') LIMIT 1").Scan(&next); err != nil {
			t.Fatal(err)
		}
		if _, err = reopened.DeliverOne(t.Context(), next, sender); err != nil {
			t.Fatal(err)
		}
	}
	if err = reopened.DB.QueryRow("SELECT status,attempts FROM notification_deliveries WHERE notification_id IN (SELECT id FROM notification_occurrences WHERE event_id IS NULL)").Scan(&status, &attempts); err != nil || status != "failed" || attempts != 6 {
		t.Fatal("unbounded retry", status, attempts, err)
	}
}
func TestPostgresPushDowntimeDSTAndPersistence(t *testing.T) {
	db, _, router, now := pushFixture(t)
	uid, _ := registerHTTP(t, router, "downtime@example.test")
	ctx := repository.WithUser(t.Context(), uid)
	if _, err := db.Subscribe(ctx, pushInput(t, "downtime"), "agent", now); err != nil {
		t.Fatal(err)
	}
	createPushReminder(t, db, uid, now.Add(-2*time.Minute), nil)
	old := createPushReminder(t, db, uid, now.Add(-30*24*time.Hour), &domain.Recurrence{Frequency: "day", Interval: 1, LocalTime: &domain.TimeOfDay{Hour: now.Hour(), Minute: now.Minute()}, Timezone: now.Location().String()})
	sender := &pushFake{}
	reopened := reopenedPushDB(t, db)
	worker := notifications.Worker{Store: reopened, Sender: sender, Location: now.Location()}
	if err := worker.Tick(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	if len(sender.payloads) != 1 {
		t.Fatal("backlog flood or missed reminder", len(sender.payloads))
	}
	stored, _ := (&calendar.Service{Store: reopened, Location: now.Location()}).Get(ctx, old.ID)
	if !stored.OccurrenceAt.After(now) || !stored.Recurrence.Anchor.Equal(old.Recurrence.Anchor) {
		t.Fatal("backlog didn't advance")
	}
	zone, _ := time.LoadLocation("Europe/Berlin")
	start := time.Date(2026, 3, 28, 9, 0, 0, 0, zone)
	e := createPushReminder(t, db, uid, start, &domain.Recurrence{Frequency: "day", Interval: 1, LocalTime: &domain.TimeOfDay{Hour: 9}, Timezone: zone.String()})
	if _, err := reopened.ProcessDue(t.Context(), start, zone); err != nil {
		t.Fatal(err)
	}
	stored, _ = (&calendar.Service{Store: reopened, Location: zone}).Get(ctx, e.ID)
	if stored.OccurrenceAt.In(zone).Hour() != 9 || stored.OccurrenceAt.Sub(*e.OccurrenceAt) != 23*time.Hour {
		t.Fatal("DST drift", stored.OccurrenceAt)
	}
}
func TestPostgresPushSkipLockedClaimAndCancelledDelivery(t *testing.T) {
	db, _, router, now := pushFixture(t)
	uid, _ := registerHTTP(t, router, "locked@example.test")
	ctx := repository.WithUser(t.Context(), uid)
	for _, endpoint := range []string{"locked", "unlocked"} {
		if _, err := db.Subscribe(ctx, pushInput(t, endpoint), "agent", now); err != nil {
			t.Fatal(err)
		}
	}
	e := createPushReminder(t, db, uid, now, nil)
	if _, err := db.ProcessDue(t.Context(), now, now.Location()); err != nil {
		t.Fatal(err)
	}
	tx, err := db.DB.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var locked string
	if err = tx.QueryRow("SELECT id FROM notification_deliveries ORDER BY id FOR UPDATE LIMIT 1").Scan(&locked); err != nil {
		t.Fatal(err)
	}
	sender := &pushFake{}
	result, err := db.DeliverOne(t.Context(), now, sender)
	if err != nil || result == nil || result.ID == locked {
		t.Fatal("SKIP LOCKED failed", result, err)
	}
	tx.Rollback()
	done := domain.StatusDone
	cal := &calendar.Service{Store: db, Location: now.Location()}
	if _, err = cal.Update(ctx, e.ID, calendar.Patch{Status: &done}, now); err != nil {
		t.Fatal(err)
	}
	result, err = db.DeliverOne(t.Context(), now, sender)
	if err != nil || result == nil || result.Status != "cancelled" || len(sender.payloads) != 1 {
		t.Fatal("completed reminder sent", result, err)
	}
}
func TestPostgresPushDisabledConfigAndUserIsolation(t *testing.T) {
	db, _, router, now := pgFixture(t)
	uid, cookie := registerHTTP(t, router, "disabled-push@example.test")
	call(t, router, cookie, "POST", "/api/notifications/push/subscriptions", pushInput(t, "disabled"), 503)
	ctx := repository.WithUser(context.Background(), uid)
	if _, err := db.Subscriptions(context.Background()); err == nil {
		t.Fatal("missing ownership accepted")
	}
	if _, err := db.Subscribe(ctx, pushInput(t, "disabled"), "agent", now); err != nil {
		t.Fatal(err)
	}
	createPushReminder(t, db, uid, now, nil)
	worker := notifications.Worker{Store: db, Location: now.Location()}
	if err := worker.Tick(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	events, err := db.DueNotifications(ctx, now)
	if err != nil || len(events) != 1 {
		t.Fatal("in-app fallback lost", events, err)
	}
	raw, _ := json.Marshal(events)
	if len(raw) == 0 {
		t.Fatal("missing snapshot")
	}
}
