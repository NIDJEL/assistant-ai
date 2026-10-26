package contract

import (
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

func TestExecutionStatusesReadActualOutboxesAndDeliveriesWithUserScope(t *testing.T) {
	db, auth, _, now := pgFixture(t)
	u, err := auth.Register(t.Context(), "Статусы", "status-owner@example.test", "test password 12345", "test password 12345")
	if err != nil {
		t.Fatal(err)
	}
	other, err := auth.Register(t.Context(), "Другой", "status-other@example.test", "test password 12345", "test password 12345")
	if err != nil {
		t.Fatal(err)
	}
	ctx := repository.WithUser(t.Context(), u.ID)
	if err = db.SaveCloudConnection(ctx, repository.CloudConnection{Provider: "google", Subject: "fixture", Status: "connected", CalendarID: "primary"}); err != nil {
		t.Fatal(err)
	}
	date := domain.Day(now)
	clock := &domain.TimeOfDay{Hour: now.Hour(), Minute: now.Minute()}
	var event *domain.Event
	err = db.Write(ctx, func(tx repository.Tx) error {
		var e error
		event, e = tx.Create(ctx, domain.Event{Kind: domain.EventReminder, Title: "Напоминание", Date: date, StartTime: clock, Duration: time.Hour, Status: domain.StatusOpen, Priority: domain.PriorityNormal}, now)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	statuses, err := db.ExecutionStatuses(ctx, []int64{event.ID}, now)
	if err != nil {
		t.Fatal(err)
	}
	find := func(provider, state string) {
		t.Helper()
		for _, s := range statuses {
			if s.Provider == provider && s.State == state {
				return
			}
		}
		t.Fatal("status missing", provider, state, statuses)
	}
	find("google", "pending")
	find("web_push", "disabled")
	foreign, err := db.ExecutionStatuses(repository.WithUser(t.Context(), other.ID), []int64{event.ID}, now)
	if err != nil || len(foreign) != 0 {
		t.Fatal("scope leak", foreign, err)
	}
	if _, err = db.ExecutionStatuses(t.Context(), []int64{event.ID}, now); err == nil {
		t.Fatal("unauthenticated status read")
	}
	device, err := db.Subscribe(ctx, pushInput(t, "top10-status"), "fixture", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ProcessDue(ctx, now, now.Location()); err != nil {
		t.Fatal(err)
	}
	statuses, err = db.ExecutionStatuses(ctx, []int64{event.ID}, now)
	if err != nil {
		t.Fatal(err)
	}
	find("web_push", "pending")
	if _, err = db.DB.Exec(`UPDATE notification_deliveries SET status='success',attempts=1 WHERE user_id=$1 AND subscription_id=$2`, u.ID, device.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB.Exec(`DELETE FROM cloud_outbox WHERE user_id=$1`, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB.Exec(`INSERT INTO cloud_event_links(user_id,provider,event_id,remote_id) VALUES($1,'google',$2,'fixture-remote')`, u.ID, event.ID); err != nil {
		t.Fatal(err)
	}
	statuses, err = db.ExecutionStatuses(ctx, []int64{event.ID}, now)
	if err != nil {
		t.Fatal(err)
	}
	find("google", "synced")
	find("web_push", "accepted")
	var attempts int
	if err = db.DB.QueryRow(`SELECT sum(attempts) FROM notification_deliveries WHERE user_id=$1`, u.ID).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatal("status polling mutated delivery", attempts, err)
	}
}

func TestFiniteRecurrenceQueuesFinalNotificationOnceThenStops(t *testing.T) {
	db, auth, _, now := pgFixture(t)
	u, err := auth.Register(t.Context(), "Конец серии", "finite-owner@example.test", "test password 12345", "test password 12345")
	if err != nil {
		t.Fatal(err)
	}
	ctx := repository.WithUser(t.Context(), u.ID)
	if _, err = db.Subscribe(ctx, pushInput(t, "finite-series"), "fixture", now); err != nil {
		t.Fatal(err)
	}
	var event *domain.Event
	err = db.Write(ctx, func(tx repository.Tx) error {
		var e error
		event, e = tx.Create(ctx, domain.Event{Kind: domain.EventReminder, Title: "Последний приём", Date: domain.Day(now), StartTime: &domain.TimeOfDay{Hour: now.Hour(), Minute: now.Minute()}, Duration: time.Hour, Status: domain.StatusOpen, Priority: domain.PriorityNormal, EventDetails: domain.EventDetails{Recurrence: &domain.Recurrence{Frequency: "minute", Interval: 20, Count: 1, Timezone: now.Location().String(), Anchor: now.Add(-20 * time.Minute)}}}, now)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []int{1, 0} {
		n, e := db.ProcessDue(ctx, now.Add(time.Duration(i)*20*time.Minute), now.Location())
		if e != nil || n != want {
			t.Fatal(n, e)
		}
	}
	if err = db.Read(ctx, func(tx repository.Tx) error {
		current, e := tx.Get(ctx, event.ID)
		if e == nil && current.Status != domain.StatusDone {
			t.Fatal(current)
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	var count int
	var status string
	if err = db.DB.QueryRow(`SELECT count(*),min(d.status) FROM notification_deliveries d JOIN notification_occurrences o ON o.id=d.notification_id WHERE o.event_id=$1`, event.ID).Scan(&count, &status); err != nil || count != 1 || status != "pending" {
		t.Fatal("final delivery lost or repeated", count, status, err)
	}
	fake := &pushFake{}
	result, err := db.DeliverOne(ctx, now, fake)
	if err != nil || result == nil || result.Status != "success" || len(fake.payloads) != 1 {
		t.Fatal("final occurrence was queued but not delivered", result, err)
	}
}
