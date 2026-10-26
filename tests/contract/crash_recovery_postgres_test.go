package contract

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/notifications"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/search"
	"github.com/NIDJEL/assistant-ai/internal/storage/postgres"
)

const crashOperation = `{"intent":"multi_action","confidence":"high","arguments":{"steps":[{"key":"a","source":"Купить продукты","operation":{"intent":"create_task","confidence":"high","arguments":{"title":"Купить продукты"}}},{"key":"b","source":"Забрать заказ","operation":{"intent":"create_task","confidence":"high","arguments":{"title":"Забрать заказ"}}}]}}`

type crashSender struct{ db *postgres.Database }

func (s crashSender) Send(ctx context.Context, _ notifications.Subscription, p notifications.Payload) notifications.Result {
	if _, err := s.db.DB.ExecContext(ctx, "INSERT INTO crash_receipts(id) VALUES($1)", p.NotificationID); err != nil {
		os.Exit(91)
	}
	os.Exit(39) // Gateway accepted the payload, worker dies before its local ACK.
	return notifications.Result{}
}

func TestQualityCrashHelper(t *testing.T) {
	mode := os.Getenv("GENA_CRASH_HELPER")
	if mode == "" {
		return
	}
	db, err := postgres.Open(context.Background(), os.Getenv("GENA_CRASH_DSN"))
	if err != nil {
		os.Exit(90)
	}
	ctx := repository.WithUser(context.Background(), os.Getenv("GENA_CRASH_USER"))
	zone, _ := time.LoadLocation("Europe/Moscow")
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, zone)
	if mode == "before_commit" {
		_ = db.Write(ctx, func(tx repository.Tx) error {
			_, err := tx.Create(ctx, domain.Event{Kind: domain.EventTask, Title: "Незавершённая запись", Duration: time.Hour, Status: domain.StatusOpen, Priority: domain.PriorityNormal}, now)
			if err != nil {
				os.Exit(92)
			}
			if tx.SaveState(ctx, "crash", domain.State{}, now) != nil {
				os.Exit(93)
			}
			os.Exit(37)
			return nil
		})
	} else if mode == "after_commit" {
		engine, _ := search.New()
		cal := &calendar.Service{Store: db, Search: engine, Location: time.UTC}
		s := assistant.New(db, &multiScopedOperations{raw: crashOperation}, cal)
		if _, err = s.Respond(ctx, "Купить продукты; Забрать заказ", "crash", "committed", now); err != nil {
			os.Exit(94)
		}
		os.Exit(38)
	} else if mode == "gateway_ack" {
		_, _ = db.DeliverOne(ctx, now, crashSender{db: db})
	}
	os.Exit(95)
}

func runCrashHelper(t *testing.T, db *postgres.Database, user, mode string, want int) {
	t.Helper()
	var schema string
	if err := db.DB.QueryRow("SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-test.run=^TestQualityCrashHelper$")
	cmd.Env = append(os.Environ(), "GENA_CRASH_HELPER="+mode, "GENA_CRASH_DSN="+u.String(), "GENA_CRASH_USER="+user)
	output, err := cmd.CombinedOutput()
	var exited *exec.ExitError
	if !errors.As(err, &exited) || exited.ExitCode() != want {
		t.Fatalf("helper did not reach crash point %s: %v %s", mode, err, strings.TrimSpace(string(output)))
	}
}

func TestPostgresProcessDeathRollsBackAndCommittedRequestReplays(t *testing.T) {
	db, auth, _, now := pgFixture(t)
	u, err := auth.Register(t.Context(), "Восстановление", "crash-owner@example.test", "test password 12345", "test password 12345")
	if err != nil {
		t.Fatal(err)
	}
	ctx := repository.WithUser(t.Context(), u.ID)
	runCrashHelper(t, db, u.ID, "before_commit", 37)
	if err = db.Read(ctx, func(tx repository.Tx) error {
		events, err := tx.Events(ctx, domain.Filter{})
		if err == nil && len(events) != 0 {
			t.Fatal("uncommitted event survived death")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	runCrashHelper(t, db, u.ID, "after_commit", 38)
	engine, _ := search.New()
	cal := &calendar.Service{Store: db, Search: engine, Location: now.Location()}
	// Invalid provider output proves this retry is served from durable cache.
	s := assistant.New(db, &multiScopedOperations{raw: `invalid`}, cal)
	r, err := s.Respond(ctx, "Купить продукты; Забрать заказ", "crash", "committed", now)
	if err != nil || len(r.Events) != 2 {
		t.Fatal("committed response lost", r, err)
	}
	if err = db.Read(ctx, func(tx repository.Tx) error {
		events, err := tx.Events(ctx, domain.Filter{})
		if err == nil && len(events) != 2 {
			t.Fatal("request replay duplicated effects", len(events))
		}
		history, err := tx.History(ctx, "crash", 20)
		if err == nil && len(history) != 1 {
			t.Fatal("history duplicated", len(history))
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresWorkerDeathAfterGatewayAcceptanceRetainsStableDeliveryID(t *testing.T) {
	db, auth, _, now := pgFixture(t)
	u, err := auth.Register(t.Context(), "Доставка", "crash-push@example.test", "test password 12345", "test password 12345")
	if err != nil {
		t.Fatal(err)
	}
	ctx := repository.WithUser(t.Context(), u.ID)
	sub, err := db.Subscribe(ctx, pushInput(t, "crash-receipt"), "fixture", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB.Exec("CREATE TABLE crash_receipts(id text NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	if err = db.QueueTest(ctx, sub.ID, now); err != nil {
		t.Fatal(err)
	}
	runCrashHelper(t, db, u.ID, "gateway_ack", 39)
	var id, status string
	var attempts int
	if err = db.DB.QueryRow("SELECT id FROM crash_receipts").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err = db.DB.QueryRow("SELECT status,attempts FROM notification_deliveries").Scan(&status, &attempts); err != nil || status != "pending" || attempts != 0 {
		t.Fatal("worker lease/transaction stuck", status, attempts, err)
	}
	fake := &pushFake{}
	result, err := db.DeliverOne(ctx, now, fake)
	if err != nil || result == nil || result.Status != "success" || len(fake.payloads) != 1 || fake.payloads[0].NotificationID != id {
		t.Fatal("retry changed logical delivery identity", result, err)
	}
}
