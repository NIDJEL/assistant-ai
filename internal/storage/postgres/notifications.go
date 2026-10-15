package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/notifications"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/google/uuid"
)

const subscriptionColumns = "id,user_id,endpoint,p256dh,auth,user_agent,platform,created_at,updated_at,last_success_at,last_failure_at,failure_count,disabled_at"

func scanSubscription(row scanner) (*notifications.Subscription, error) {
	var s notifications.Subscription
	err := row.Scan(&s.ID, &s.UserID, &s.Endpoint, &s.P256DH, &s.Auth, &s.UserAgent, &s.Platform, &s.CreatedAt, &s.UpdatedAt, &s.LastSuccessAt, &s.LastFailureAt, &s.FailureCount, &s.DisabledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &s, err
}
func (d *Database) Subscriptions(ctx context.Context) ([]notifications.Subscription, error) {
	uid, err := calendarUser(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := d.DB.QueryContext(ctx, "SELECT "+subscriptionColumns+" FROM push_subscriptions WHERE user_id=$1 ORDER BY created_at", uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []notifications.Subscription{}
	for rows.Next() {
		s, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *s)
	}
	return result, rows.Err()
}
func (d *Database) Subscribe(ctx context.Context, input notifications.Subscribe, agent string, now time.Time) (result *notifications.Subscription, err error) {
	if err = input.Validate(); err != nil {
		return
	}
	if len(agent) > 512 {
		agent = agent[:512]
	}
	err = d.Write(ctx, func(tx repository.Tx) error {
		r := tx.(*Repo)
		var count int
		if err := r.q.QueryRowContext(ctx, "SELECT count(*) FROM push_subscriptions WHERE user_id=$1 AND disabled_at IS NULL AND endpoint<>$2", r.userID, input.Endpoint).Scan(&count); err != nil {
			return err
		}
		if count >= 20 {
			return &domain.AppError{Code: "device_limit", Message: "Отключите одно из устройств перед добавлением нового", Status: 409}
		}
		var err error
		result, err = scanSubscription(r.q.QueryRowContext(ctx, `INSERT INTO push_subscriptions(id,user_id,endpoint,p256dh,auth,user_agent,platform,created_at,updated_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$8) ON CONFLICT(endpoint) DO UPDATE SET p256dh=excluded.p256dh,auth=excluded.auth,
		 user_agent=excluded.user_agent,platform=excluded.platform,updated_at=excluded.updated_at,disabled_at=NULL,failure_count=0
		 WHERE push_subscriptions.user_id=excluded.user_id RETURNING `+subscriptionColumns, uuid.NewString(), r.userID, input.Endpoint, input.Keys.P256DH, input.Keys.Auth, agent, input.Platform, now))
		if err == nil && result == nil {
			return &domain.AppError{Code: "subscription_conflict", Message: "Это устройство привязано к другому аккаунту. Сначала отключите уведомления в нём.", Status: 409}
		}
		return err
	})
	return
}
func (d *Database) Unsubscribe(ctx context.Context, id string, now time.Time) error {
	uid, err := calendarUser(ctx)
	if err != nil {
		return err
	}
	if _, err = uuid.Parse(id); err != nil {
		return domain.Validation("Некорректное устройство")
	}
	return d.Write(ctx, func(tx repository.Tx) error {
		r := tx.(*Repo)
		result, err := r.q.ExecContext(ctx, "UPDATE push_subscriptions SET disabled_at=$3,updated_at=$3 WHERE id=$1 AND user_id=$2", id, uid, now)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return &domain.AppError{Code: "not_found", Message: "Устройство не найдено", Status: 404}
		}
		_, err = r.q.ExecContext(ctx, "UPDATE notification_deliveries SET status='cancelled',updated_at=$2 WHERE subscription_id=$1 AND status IN ('pending','retry')", id, now)
		return err
	})
}

func enqueueNotification(ctx context.Context, r *Repo, event *domain.Event, now time.Time, subscriptionID string) error {
	nid := uuid.NewString()
	payload := notifications.Payload{NotificationID: nid, UserID: r.userID, Title: "Гена", Body: "Уведомления работают", URL: "/settings", Kind: "test", Timestamp: now, ExpiresAt: now.Add(notifications.DeliveryLifetime)}
	var eventID any
	var snapshot any
	occurrence := now
	if event != nil {
		eventID = event.ID
		occurrence, _, _ = event.Interval(time.UTC) // Dated legacy records already have OccurrenceAt set below.
		payload.EventID, payload.Body, payload.Kind = event.ID, event.Title, event.Kind.String()
		payload.URL = fmt.Sprintf("/calendar?date=%s&event=%d", event.Date.String(), event.ID)
		payload.Timestamp = occurrence.Add(-time.Duration(event.ReminderLead()) * time.Minute)
		payload.ExpiresAt = payload.Timestamp.Add(notifications.DeliveryLifetime)
		raw, err := json.Marshal(event)
		if err != nil {
			return err
		}
		snapshot = string(raw)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var inserted string
	err = r.q.QueryRowContext(ctx, `INSERT INTO notification_occurrences(id,user_id,event_id,occurrence_at,created_at,expires_at,payload,event_snapshot)
	 VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(event_id,occurrence_at) DO NOTHING RETURNING id`, nid, r.userID, eventID, occurrence, now, payload.ExpiresAt, string(raw), snapshot).Scan(&inserted)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = r.q.ExecContext(ctx, `INSERT INTO notification_deliveries(id,notification_id,subscription_id,user_id,next_attempt_at,updated_at)
	 SELECT gen_random_uuid(),$1,id,user_id,$2,$2 FROM push_subscriptions WHERE user_id=$3 AND disabled_at IS NULL AND ($4='' OR id::text=$4)
	 ON CONFLICT(notification_id,subscription_id) DO NOTHING`, nid, now, r.userID, subscriptionID)
	return err
}
func (d *Database) QueueTest(ctx context.Context, subscriptionID string, now time.Time) error {
	if _, err := uuid.Parse(subscriptionID); err != nil {
		return domain.Validation("Некорректное устройство")
	}
	return d.Write(ctx, func(tx repository.Tx) error {
		r := tx.(*Repo)
		var exists bool
		if err := r.q.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM push_subscriptions WHERE id=$1 AND user_id=$2 AND disabled_at IS NULL)", subscriptionID, r.userID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return &domain.AppError{Code: "not_found", Message: "Сначала включите уведомления на этом устройстве", Status: 404}
		}
		var last time.Time
		err := r.q.QueryRowContext(ctx, `INSERT INTO push_test_limits(user_id,last_sent_at) VALUES($1,$2)
		 ON CONFLICT(user_id) DO UPDATE SET last_sent_at=excluded.last_sent_at WHERE push_test_limits.last_sent_at<=$2::timestamptz-interval '1 minute' RETURNING last_sent_at`, r.userID, now).Scan(&last)
		if errors.Is(err, sql.ErrNoRows) {
			return &domain.AppError{Code: "rate_limited", Message: "Повторить проверку можно через минуту", Status: 429}
		}
		if err != nil {
			return err
		}
		return enqueueNotification(ctx, r, nil, now, subscriptionID)
	})
}

// The same database advisory lock as domain writes prevents lost recurrence
// updates during edits, and try-lock allows other workers to process other users.
const occurrenceSQL = `COALESCE((details::jsonb->>'occurrence_at')::timestamptz,
 (date||' '||time)::timestamp AT TIME ZONE $2 + COALESCE((details::jsonb->>'start_second')::int,0)*interval '1 second')`
const duePredicate = `deleted_at IS NULL AND status='open' AND date IS NOT NULL AND time IS NOT NULL
 AND (` + occurrenceSQL + `)-COALESCE((details::jsonb->>'reminder_minutes')::int,0)*interval '1 minute'<=$1
 AND (details::jsonb->>'recurrence' IS NOT NULL OR COALESCE((details::jsonb->>'reminder_enabled')::boolean,kind='reminder' OR details::jsonb->>'reminder_minutes' IS NOT NULL))
 AND (details::jsonb->>'recurrence' IS NOT NULL OR kind='reminder' OR (details::jsonb->>'last_notification_at')::timestamptz IS DISTINCT FROM (` + occurrenceSQL + `))`

func (d *Database) ProcessDue(ctx context.Context, now time.Time, loc *time.Location) (int, error) {
	rows, err := d.DB.QueryContext(ctx, `SELECT DISTINCT user_id::text FROM events WHERE `+duePredicate, now, loc.String())
	if err != nil {
		return 0, err
	}
	users := []string{}
	for rows.Next() {
		var uid string
		if err = rows.Scan(&uid); err != nil {
			rows.Close()
			return 0, err
		}
		users = append(users, uid)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, uid := range users {
		n, err := d.processUserDue(ctx, uid, now, loc)
		if err != nil {
			return count, err
		}
		count += n
	}
	return count, nil
}
func (d *Database) processUserDue(ctx context.Context, uid string, now time.Time, loc *time.Location) (int, error) {
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var locked bool
	if err = tx.QueryRowContext(ctx, "SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))", uid).Scan(&locked); err != nil || !locked {
		return 0, err
	}
	r := &Repo{q: tx, userID: uid}
	rows, err := tx.QueryContext(ctx, "SELECT "+columns+" FROM events WHERE user_id=$3 AND "+duePredicate, now, loc.String(), uid)
	if err != nil {
		return 0, err
	}
	events := []domain.Event{}
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			rows.Close()
			return 0, err
		}
		events = append(events, *e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, e := range events {
		updated, snapshot, changed, err := notifications.NextDue(e, now, loc)
		if err != nil {
			return 0, err
		}
		if snapshot != nil {
			start, _, _ := snapshot.Interval(loc)
			snapshot.OccurrenceAt = &start
			if err = enqueueNotification(ctx, r, snapshot, now, ""); err != nil {
				return 0, err
			}
			count++
		}
		if changed {
			if _, err = r.Update(ctx, updated, now); err != nil {
				return 0, err
			}
		}
	}
	return count, tx.Commit()
}
func (d *Database) DueNotifications(ctx context.Context, now time.Time) ([]domain.Event, error) {
	uid, err := calendarUser(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := d.DB.QueryContext(ctx, `SELECT n.event_snapshot FROM notification_occurrences n JOIN events e ON e.id=n.event_id
	 WHERE n.user_id=$1 AND e.user_id=$1 AND e.deleted_at IS NULL
	 AND (e.status='open' OR (e.status='done' AND e.kind='reminder' AND e.details::jsonb->>'recurrence' IS NULL
	 AND (e.details::jsonb->>'last_notification_at')::timestamptz=n.occurrence_at))
	 AND n.created_at BETWEEN $2::timestamptz-interval '5 minutes' AND $2 ORDER BY n.created_at LIMIT 100`, uid, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.Event{}
	for rows.Next() {
		var raw []byte
		var e domain.Event
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &e); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// Keep the row lock through the bounded HTTP call: SKIP LOCKED excludes another
// instance until the result commits. A crash rolls back for a later retry; the
// notification ID also deduplicates visible messages in the service worker.
func (d *Database) DeliverOne(ctx context.Context, now time.Time, sender notifications.Sender) (*notifications.DeliveryResult, error) {
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var result notifications.DeliveryResult
	var sid string
	var raw []byte
	var expires time.Time
	err = tx.QueryRowContext(ctx, `SELECT d.id,d.notification_id,d.subscription_id,d.attempts,n.payload,n.expires_at FROM notification_deliveries d
	 JOIN notification_occurrences n ON n.id=d.notification_id JOIN push_subscriptions s ON s.id=d.subscription_id
	 WHERE d.status IN ('pending','retry') AND d.next_attempt_at<=$1
	 ORDER BY d.next_attempt_at,d.id FOR UPDATE OF d,s SKIP LOCKED LIMIT 1`, now).Scan(&result.ID, &result.NotificationID, &sid, &result.Attempts, &raw, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sub, err := scanSubscription(tx.QueryRowContext(ctx, "SELECT "+subscriptionColumns+" FROM push_subscriptions WHERE id=$1", sid))
	if err != nil {
		return nil, err
	}
	result.SubscriptionID = sid
	var payload notifications.Payload
	if err = json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	result.Status = "cancelled"
	allowed := sub.DisabledAt == nil
	if allowed && payload.EventID != 0 {
		e, err := (&Repo{q: tx, userID: sub.UserID}).Get(ctx, payload.EventID)
		if err != nil {
			return nil, err
		}
		// Automatic completion retains the occurrence stamp. Manual completion
		// or edits clear it, which still cancels queued deliveries.
		autoCompleted := e != nil && e.Status == domain.StatusDone && e.Kind == domain.EventReminder && e.Recurrence == nil && e.LastNotificationAt != nil
		allowed = e != nil && (e.Status == domain.StatusOpen || autoCompleted) && e.HasReminder()
		if allowed {
			var snapshotRaw []byte
			var snapshot domain.Event
			if err = tx.QueryRowContext(ctx, "SELECT event_snapshot FROM notification_occurrences WHERE id=$1", result.NotificationID).Scan(&snapshotRaw); err != nil {
				return nil, err
			}
			if err = json.Unmarshal(snapshotRaw, &snapshot); err != nil {
				return nil, err
			}
			if autoCompleted && (snapshot.OccurrenceAt == nil || !e.LastNotificationAt.Equal(*snapshot.OccurrenceAt)) {
				allowed = false
			}
			allowed = allowed && e.ReminderLead() == snapshot.ReminderLead() && (e.Recurrence == nil) == (snapshot.Recurrence == nil)
			if allowed && e.Recurrence != nil {
				allowed = e.Recurrence.Anchor.Equal(snapshot.Recurrence.Anchor)
			}
			if allowed && e.Recurrence == nil {
				allowed = e.Date != nil && e.StartTime != nil && e.Date.String() == snapshot.Date.String() && e.StartTime.String() == snapshot.StartTime.String() && e.StartSecond == snapshot.StartSecond
				if e.OccurrenceAt != nil {
					allowed = allowed && e.OccurrenceAt.Equal(*snapshot.OccurrenceAt)
				}
			}
		}
	}
	next := now
	if allowed && !now.Before(expires) {
		result.Status = "expired"
	} else if allowed {
		attemptCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		sent := sender.Send(attemptCtx, *sub, payload)
		cancel()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		result.Attempts++
		result.HTTPStatus = sent.Status
		var delay time.Duration
		result.Status, delay = notifications.DeliveryStatus(sent, result.Attempts)
		next = now.Add(delay)
		if result.Status == "retry" && !next.Before(expires) {
			result.Status = "expired"
		}
		if result.Status == "success" {
			_, err = tx.ExecContext(ctx, "UPDATE push_subscriptions SET last_success_at=$2,failure_count=0 WHERE id=$1", sid, now)
		} else {
			_, err = tx.ExecContext(ctx, "UPDATE push_subscriptions SET last_failure_at=$2,failure_count=failure_count+1,disabled_at=CASE WHEN $3='dead' THEN $2 ELSE disabled_at END WHERE id=$1", sid, now, result.Status)
		}
		if err != nil {
			return nil, err
		}
	}
	_, err = tx.ExecContext(ctx, "UPDATE notification_deliveries SET status=$2,attempts=$3,next_attempt_at=$4,updated_at=$5,http_status=$6 WHERE id=$1", result.ID, result.Status, result.Attempts, next, now, result.HTTPStatus)
	if err != nil {
		return nil, err
	}
	return &result, tx.Commit()
}
