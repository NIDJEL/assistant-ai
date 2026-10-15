package notifications

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

type Config struct{ PublicKey, PrivateKey, Subject string }

func (c Config) Enabled() bool { return c.PublicKey != "" && c.PrivateKey != "" && c.Subject != "" }
func (c Config) Validate() error {
	if c.PublicKey == "" && c.PrivateKey == "" && c.Subject == "" {
		return nil
	}
	bad := errors.New("WEB_PUSH requires a matching P-256 VAPID key pair and a mailto or HTTPS subject")
	private, err := base64.RawURLEncoding.DecodeString(c.PrivateKey)
	if err != nil {
		return bad
	}
	key, err := ecdh.P256().NewPrivateKey(private)
	if err != nil {
		return bad
	}
	public, err := base64.RawURLEncoding.DecodeString(c.PublicKey)
	if err != nil || !bytes.Equal(public, key.PublicKey().Bytes()) {
		return bad
	}
	u, err := url.Parse(c.Subject)
	if err != nil || u.User != nil || (u.Scheme != "mailto" && u.Scheme != "https") || (u.Scheme == "mailto" && !strings.Contains(u.Opaque, "@")) || (u.Scheme == "https" && u.Hostname() == "") {
		return bad
	}
	return nil
}

// Do not let authenticated subscription URLs turn the sender into an SSRF proxy.
// Resolve at dial time, verify every address, and pin the connection to that IP.
func publicIP(ip net.IP) bool {
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
}
func NewSender(c Config) *WebSender {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errors.New("invalid push host")
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(ips) == 0 {
			return nil, errors.New("push host unavailable")
		}
		for _, ip := range ips {
			if !publicIP(ip.IP) {
				return nil, errors.New("push host rejected")
			}
		}
		dialer := net.Dialer{Timeout: 5 * time.Second}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}
	return &WebSender{Config: c, Client: &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

type WebSender struct {
	Config Config
	Client webpush.HTTPClient
}

func (s *WebSender) Send(ctx context.Context, subscription Subscription, payload Payload) Result {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Result{Err: errors.New("push payload invalid")}
	}
	if len(raw) > 3000 {
		return Result{Status: 413}
	}
	topic := sha256.Sum256([]byte(payload.NotificationID))
	ttl := int(time.Until(payload.ExpiresAt).Seconds())
	if ttl <= 0 {
		return Result{Expired: true}
	}
	if ttl > 1800 {
		ttl = 1800
	}
	response, err := webpush.SendNotificationWithContext(ctx, raw, &webpush.Subscription{Endpoint: subscription.Endpoint, Keys: webpush.Keys{P256dh: subscription.P256DH, Auth: subscription.Auth}}, &webpush.Options{
		// webpush-go adds mailto: to a bare email itself; avoid a doubled scheme.
		HTTPClient: s.Client, Subscriber: strings.TrimPrefix(s.Config.Subject, "mailto:"), VAPIDPublicKey: s.Config.PublicKey, VAPIDPrivateKey: s.Config.PrivateKey,
		TTL: ttl, Urgency: webpush.UrgencyHigh, Topic: base64.RawURLEncoding.EncodeToString(topic[:24]),
	})
	if err != nil {
		return Result{Err: errors.New("push transport failed")}
	} // Never include endpoint/keys in logs.
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	result := Result{Status: response.StatusCode}
	if n, err := strconv.Atoi(response.Header.Get("Retry-After")); err == nil && n > 0 {
		result.RetryAfter = time.Duration(min(n, 900)) * time.Second
	} else if at, err := http.ParseTime(response.Header.Get("Retry-After")); err == nil {
		result.RetryAfter = min(max(time.Until(at), 0), 15*time.Minute)
	}
	return result
}

func DeliveryStatus(result Result, attempts int) (string, time.Duration) {
	if result.Expired {
		return "expired", 0
	}
	if result.Err == nil && result.Status >= 200 && result.Status < 300 {
		return "success", 0
	}
	if result.Status == 404 || result.Status == 410 {
		return "dead", 0
	}
	if attempts >= MaxAttempts {
		return "failed", 0
	}
	if result.Err != nil || result.Status == 408 || result.Status == 429 || result.Status >= 500 {
		backoff := min(30*time.Second*time.Duration(1<<max(attempts-1, 0)), 15*time.Minute)
		return "retry", max(backoff, result.RetryAfter)
	}
	return "failed", 0
}
