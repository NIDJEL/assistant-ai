package notifications

import (
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/golang-jwt/jwt/v5"
)

func validInput(t *testing.T) Subscribe {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s := Subscribe{Endpoint: "https://push.example.test/delivery", Platform: "Windows"}
	s.Keys.P256DH = base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
	s.Keys.Auth = base64.RawURLEncoding.EncodeToString(make([]byte, 16))
	return s
}
func TestSubscriptionValidation(t *testing.T) {
	s := validInput(t)
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Subscribe)
	}{
		{"http", func(s *Subscribe) { s.Endpoint = "http://example.test/push" }},
		{"credentials", func(s *Subscribe) { s.Endpoint = "https://a:b@example.test/push" }},
		{"port", func(s *Subscribe) { s.Endpoint = "https://example.test:5432/push" }},
		{"fragment", func(s *Subscribe) { s.Endpoint += "#token" }},
		{"length", func(s *Subscribe) { s.Endpoint += strings.Repeat("a", 4096) }},
		{"curve", func(s *Subscribe) { s.Keys.P256DH = base64.RawURLEncoding.EncodeToString(make([]byte, 65)) }},
		{"auth", func(s *Subscribe) { s.Keys.Auth = "secret" }},
		{"platform", func(s *Subscribe) { s.Platform = "arbitrary" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := s
			tc.mutate(&bad)
			if bad.Validate() == nil {
				t.Fatal("accepted invalid input")
			}
		})
	}
}
func TestVAPIDConfig(t *testing.T) {
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	valid := Config{PublicKey: public, PrivateKey: private, Subject: "mailto:push@example.test"}
	if !valid.Enabled() || valid.Validate() != nil || (Config{}).Enabled() || (Config{}).Validate() != nil {
		t.Fatal("bad config detection")
	}
	for _, c := range []Config{{PublicKey: public}, {PublicKey: public, PrivateKey: "bad", Subject: valid.Subject}, {PublicKey: public, PrivateKey: private, Subject: "http://localhost"}, {PublicKey: public, PrivateKey: private, Subject: "mailto:"}, {PublicKey: "bad", PrivateKey: private, Subject: valid.Subject}} {
		if c.Validate() == nil {
			t.Fatal("invalid config accepted")
		}
	}
	valid.Subject = "https://example.test/contact"
	if valid.Validate() != nil {
		t.Fatal("HTTPS subject rejected")
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }
func TestWebPushSenderEncryptedRequest(t *testing.T) {
	private, public, _ := webpush.GenerateVAPIDKeys()
	input := validInput(t)
	s := WebSender{Config: Config{public, private, "mailto:push@example.test"}, Client: transportFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.Header.Get("Content-Encoding") != "aes128gcm" || !strings.Contains(r.Header.Get("Authorization"), public) || strings.Contains(string(raw), "Снять чайник") || len(r.Header.Get("Topic")) > 32 || r.Header.Get("TTL") == "" {
			t.Fatal("invalid encrypted request")
		}
		header := r.Header.Get("Authorization")
		tokenValue := strings.Split(strings.TrimPrefix(header, "vapid t="), ", k=")[0]
		// The actual signed subject must be exactly the configured URI, especially on Safari.
		publicBytes, _ := base64.RawURLEncoding.DecodeString(public)
		x, y := elliptic.Unmarshal(elliptic.P256(), publicBytes)
		token, err := jwt.Parse(tokenValue, func(*jwt.Token) (any, error) { return &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, nil }, jwt.WithValidMethods([]string{"ES256"}))
		if err != nil || !token.Valid || token.Claims.(jwt.MapClaims)["sub"] != "mailto:push@example.test" {
			t.Fatal("invalid VAPID subject or signature", err)
		}
		return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
	})}
	p := Payload{NotificationID: "id", Body: "Снять чайник", ExpiresAt: time.Now().Add(time.Minute)}
	result := s.Send(t.Context(), Subscription{Endpoint: input.Endpoint, P256DH: input.Keys.P256DH, Auth: input.Keys.Auth}, p)
	if result.Status != 201 || result.Err != nil {
		t.Fatal(result)
	}
}
func TestWebPushSenderRetryAfterAndSanitizesErrors(t *testing.T) {
	private, public, _ := webpush.GenerateVAPIDKeys()
	input := validInput(t)
	s := WebSender{Config: Config{public, private, "mailto:push@example.test"}, Client: transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{"Retry-After": []string{"120"}}}, nil
	})}
	sub := Subscription{Endpoint: input.Endpoint, P256DH: input.Keys.P256DH, Auth: input.Keys.Auth}
	p := Payload{ExpiresAt: time.Now().Add(time.Minute)}
	if result := s.Send(t.Context(), sub, p); result.Status != 429 || result.RetryAfter != 2*time.Minute {
		t.Fatal(result)
	}
	s.Client = transportFunc(func(r *http.Request) (*http.Response, error) { return nil, errors.New("sensitive endpoint key") })
	if result := s.Send(t.Context(), sub, p); result.Err == nil || strings.Contains(result.Err.Error(), "sensitive") {
		t.Fatal(result)
	}
}
func TestDeliveryRetryPolicy(t *testing.T) {
	for _, tc := range []struct {
		result   Result
		attempts int
		status   string
		delay    time.Duration
	}{
		{Result{Status: 201}, 1, "success", 0}, {Result{Status: 404}, 1, "dead", 0}, {Result{Status: 410}, 1, "dead", 0},
		{Result{Status: 500}, 1, "retry", 30 * time.Second}, {Result{Status: 503}, 3, "retry", 2 * time.Minute}, {Result{Status: 429, RetryAfter: 5 * time.Minute}, 2, "retry", 5 * time.Minute},
		{Result{Err: errors.New("network")}, 1, "retry", 30 * time.Second}, {Result{Status: 503}, 6, "failed", 0}, {Result{Status: 401}, 1, "failed", 0},
	} {
		status, delay := DeliveryStatus(tc.result, tc.attempts)
		if status != tc.status || delay != tc.delay {
			t.Fatal(tc, status, delay)
		}
	}
}
func TestSenderBlocksPrivateNetworks(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "::1", "10.0.0.1", "192.168.1.1", "169.254.169.254", "fd00::1", "0.0.0.0", "224.0.0.1"} {
		if publicIP(net.ParseIP(ip)) {
			t.Fatal("private IP accepted", ip)
		}
	}
	if !publicIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("public IP rejected")
	}
	s := NewSender(Config{})
	transport := s.Client.(*http.Client).Transport.(*http.Transport)
	if _, err := transport.DialContext(t.Context(), "tcp", "127.0.0.1:443"); err == nil {
		t.Fatal("loopback dial accepted")
	}
}
func TestNextDueDSTNoDriftAndMissedOccurrence(t *testing.T) {
	zone, _ := time.LoadLocation("Europe/Berlin")
	clock, _ := domain.ParseTime("09:00")
	start := time.Date(2026, 3, 28, 9, 0, 0, 0, zone)
	e := domain.Event{Kind: domain.EventReminder, Title: "Вода", Status: domain.StatusOpen, EventDetails: domain.EventDetails{Recurrence: &domain.Recurrence{Frequency: "day", Interval: 1, Timezone: zone.String(), LocalTime: clock, Anchor: start.UTC()}}}
	e.SetOccurrence(start)
	updated, snapshot, changed, err := NextDue(e, start.Add(2*time.Minute), zone)
	if err != nil || snapshot == nil || !changed || updated.OccurrenceAt.In(zone).Hour() != 9 || updated.OccurrenceAt.Sub(start) != 23*time.Hour || !updated.Recurrence.Anchor.Equal(start) {
		t.Fatal(updated, snapshot, err)
	}
	late := start.AddDate(0, 2, 0)
	updated, snapshot, _, err = NextDue(e, late.Add(time.Hour), zone)
	if err != nil || snapshot != nil || !updated.OccurrenceAt.After(late) {
		t.Fatal("backlog flood", updated, snapshot, err)
	}
	_, snapshot, changed, err = NextDue(updated, late.Add(time.Hour), zone)
	if err != nil || snapshot != nil || changed {
		t.Fatal("advanced twice")
	}
}

func TestNextDueCompletesOnlyOneShotReminders(t *testing.T) {
	start := time.Date(2026, 10, 14, 23, 26, 0, 0, time.UTC)
	for _, kind := range []domain.EventType{domain.EventReminder, domain.EventTask, domain.EventMeeting} {
		t.Run(kind.String(), func(t *testing.T) {
			enabled := true
			e := domain.Event{Kind: kind, Status: domain.StatusOpen, Date: domain.Day(start), StartTime: &domain.TimeOfDay{Hour: start.Hour(), Minute: start.Minute()}, EventDetails: domain.EventDetails{ReminderEnabled: &enabled, OccurrenceAt: &start}}
			before, snapshot, changed, err := NextDue(e, start.Add(-time.Second), time.UTC)
			if err != nil || changed || snapshot != nil || before.Status != domain.StatusOpen {
				t.Fatal("changed before notification", before, snapshot, err)
			}
			updated, snapshot, changed, err := NextDue(e, start, time.UTC)
			want := domain.StatusOpen
			if kind == domain.EventReminder {
				want = domain.StatusDone
			}
			if err != nil || !changed || snapshot == nil || updated.Status != want || snapshot.Status != domain.StatusOpen {
				t.Fatal("incorrect notification lifecycle", updated, snapshot, err)
			}
		})
	}
	stamp := start.UTC()
	e := domain.Event{Kind: domain.EventReminder, Status: domain.StatusOpen, Date: domain.Day(start), StartTime: &domain.TimeOfDay{Hour: start.Hour(), Minute: start.Minute()}, EventDetails: domain.EventDetails{LastNotificationAt: &stamp, OccurrenceAt: &start}}
	updated, snapshot, changed, err := NextDue(e, start.Add(10*time.Minute), time.UTC)
	if err != nil || !changed || snapshot != nil || updated.Status != domain.StatusDone {
		t.Fatal("previously fired reminder was not archived without replay", updated, snapshot, err)
	}
}

type workerStore struct {
	Store
	ticks int
}

func (s *workerStore) ProcessDue(ctx context.Context, _ time.Time, _ *time.Location) (int, error) {
	s.ticks++
	return 0, nil
}
func TestWorkerGracefulCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	s := &workerStore{}
	w := Worker{Store: s}
	done := make(chan struct{})
	go func() { defer close(done); w.Run(ctx) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker leaked")
	}
}
