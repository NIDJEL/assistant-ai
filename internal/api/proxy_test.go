package api

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NIDJEL/assistant-ai/internal/config"
)

func TestAuthClientIPTrustBoundary(t *testing.T) {
	_, trusted, _ := net.ParseCIDR("172.30.81.1/32")
	a := API{Config: config.Config{TrustedProxyCIDRs: []*net.IPNet{trusted}}}
	for _, test := range []struct{ name, peer, real, want string }{
		{"trusted proxy", "172.30.81.1:1234", "198.51.100.10", "198.51.100.10"},
		{"untrusted client spoof", "198.51.100.20:1234", "198.51.100.10", "198.51.100.20"},
		{"invalid header", "172.30.81.1:1234", "garbage", "172.30.81.1"},
		{"multiple addresses rejected", "172.30.81.1:1234", "198.51.100.10, 198.51.100.20", "172.30.81.1"},
		{"missing header", "172.30.81.1:1234", "", "172.30.81.1"},
		{"ipv6 client", "172.30.81.1:1234", "2001:db8::1", "2001:db8::1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/auth/login", nil)
			r.RemoteAddr = test.peer
			r.Header.Set("X-Real-IP", test.real)
			if got := a.authClientIP(r); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
	a.Config.TrustedProxyCIDRs = nil
	r := httptest.NewRequest("POST", "/api/auth/login", nil)
	r.RemoteAddr = "172.30.81.1:1234"
	r.Header.Set("X-Real-IP", "198.51.100.10")
	if got := a.authClientIP(r); got != "172.30.81.1" {
		t.Fatalf("unconfigured proxy trusted: %q", got)
	}
}

func TestAuthLimiterSeparatesClientsBehindProxy(t *testing.T) {
	_, trusted, _ := net.ParseCIDR("172.30.81.1/32")
	a := API{Config: config.Config{AuthRateLimit: 1, TrustedProxyCIDRs: []*net.IPNet{trusted}}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	h := a.authLimiter()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	for _, test := range []struct {
		ip     string
		status int
	}{
		{"198.51.100.10", 204}, {"198.51.100.10", 429}, {"198.51.100.20", 204},
	} {
		r := httptest.NewRequest("POST", "/api/auth/login", nil)
		r.RemoteAddr = "172.30.81.1:1234"
		r.Header.Set("X-Real-IP", test.ip)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatalf("client %s: status %d, want %d", test.ip, w.Code, test.status)
		}
	}
}
