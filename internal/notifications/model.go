package notifications

import (
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"net/url"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

type Subscription struct {
	ID            string     `json:"id"`
	UserID        string     `json:"-"`
	Endpoint      string     `json:"-"`
	P256DH        string     `json:"-"`
	Auth          string     `json:"-"`
	UserAgent     string     `json:"-"`
	Platform      string     `json:"platform"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	LastSuccessAt *time.Time `json:"last_success_at"`
	LastFailureAt *time.Time `json:"last_failure_at"`
	FailureCount  int        `json:"failure_count"`
	DisabledAt    *time.Time `json:"disabled_at"`
}

type Subscribe struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256DH string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	Platform string `json:"platform"`
}

func (s Subscribe) Validate() error {
	u, err := url.Parse(s.Endpoint)
	if err != nil || len(s.Endpoint) > 4096 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || (u.Port() != "" && u.Port() != "443") {
		return domain.Validation("Некорректный адрес уведомлений")
	}
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(s.Keys.P256DH, "="))
	if err != nil {
		return domain.Validation("Некорректные настройки уведомлений")
	}
	if _, err = ecdh.P256().NewPublicKey(key); err != nil {
		return domain.Validation("Некорректные настройки уведомлений")
	}
	auth, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(s.Keys.Auth, "="))
	if err != nil || len(auth) != 16 {
		return domain.Validation("Некорректные настройки уведомлений")
	}
	switch s.Platform {
	case "iPhone", "iPad", "Android", "Mac", "Windows", "Linux", "Этот браузер":
	default:
		return domain.Validation("Некорректное устройство")
	}
	return nil
}

type Payload struct {
	NotificationID string    `json:"notification_id"`
	UserID         string    `json:"user_id"`
	EventID        int64     `json:"event_id,omitempty"`
	Title          string    `json:"title"`
	Body           string    `json:"body"`
	URL            string    `json:"url"`
	Kind           string    `json:"kind"`
	Timestamp      time.Time `json:"timestamp"`
	ExpiresAt      time.Time `json:"expires_at"`
}

type Result struct {
	Status     int
	RetryAfter time.Duration
	Err        error
	Expired    bool
}
type Sender interface {
	Send(context.Context, Subscription, Payload) Result
}
type DeliveryResult struct {
	ID             string
	NotificationID string
	Status         string
	Attempts       int
	HTTPStatus     int
	SubscriptionID string
}

type Store interface {
	Subscriptions(context.Context) ([]Subscription, error)
	Subscribe(context.Context, Subscribe, string, time.Time) (*Subscription, error)
	Unsubscribe(context.Context, string, time.Time) error
	QueueTest(context.Context, string, time.Time) error
	ProcessDue(context.Context, time.Time, *time.Location) (int, error)
	DeliverOne(context.Context, time.Time, Sender) (*DeliveryResult, error)
}

const Grace = 5 * time.Minute
const DeliveryLifetime = 30 * time.Minute
const MaxAttempts = 6

// NextDue preserves the existing recurrence engine, anchor and DST policy.
// Only one recent occurrence is signalled; older backlog is advanced silently.
func NextDue(e domain.Event, now time.Time, loc *time.Location) (updated domain.Event, snapshot *domain.Event, changed bool, err error) {
	updated = e
	start, _, ok := e.Interval(loc)
	if !ok {
		return
	}
	age := now.Sub(start.Add(-time.Duration(e.ReminderLead()) * time.Minute))
	if age < 0 {
		return
	}
	if e.LastNotificationAt == nil || !e.LastNotificationAt.Equal(start) {
		stamp := start.UTC()
		updated.LastNotificationAt = &stamp
		updated.LastNotificationLead = e.ReminderLead()
		changed = true
		if e.HasReminder() && age <= Grace {
			copy := e
			snapshot = &copy
		}
	}
	if e.Recurrence != nil {
		var next time.Time
		next, err = e.Recurrence.Next(now.Add(time.Duration(e.ReminderLead()) * time.Minute))
		if err != nil {
			return
		}
		updated.SetOccurrence(next)
		changed = true
	} else if e.Kind == domain.EventReminder && e.HasReminder() && e.Status == domain.StatusOpen {
		// A one-shot reminder is fulfilled when its notification is queued. Keep
		// the occurrence stamp so persisted deliveries and in-app alerts survive
		// this automatic completion; user edits clear it in calendar.Update.
		updated.Status = domain.StatusDone
		changed = true
	}
	return
}
