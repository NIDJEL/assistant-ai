package api

import (
	"bytes"
	"encoding/json"
	"github.com/NIDJEL/assistant-ai/internal/clock"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestZoomWebhookAuthenticationAndValidation(t *testing.T) {
	now := time.Unix(1800000000, 0)
	a := &API{Config: config.Config{ZoomWebhookSecret: "test-webhook-secret"}, Clock: clock.Func(func() time.Time { return now })}
	body := []byte(`{"event":"endpoint.url_validation","payload":{"plainToken":"challenge"}}`)
	for _, tc := range []struct {
		name      string
		age       int64
		signature string
		mutate    bool
		want      int
	}{
		{"valid", 0, "", false, 200}, {"forged", 0, "v0=invalid", false, 401}, {"old", -301, "", false, 401}, {"future", 301, "", false, 401}, {"altered", 0, "", true, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stamp := strconv.FormatInt(now.Unix()+tc.age, 10)
			sig := "v0=" + zoomHMAC(a.Config.ZoomWebhookSecret, "v0:"+stamp+":"+string(body))
			if tc.signature != "" {
				sig = tc.signature
			}
			payload := body
			if tc.mutate {
				payload = append(append([]byte{}, body...), byte(' '))
			}
			r := httptest.NewRequest("POST", "/api/webhooks/zoom", bytes.NewReader(payload))
			r.Header.Set("x-zm-request-timestamp", stamp)
			r.Header.Set("x-zm-signature", sig)
			w := httptest.NewRecorder()
			a.zoomWebhook(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d want %d", w.Code, tc.want)
			}
			if w.Code == 200 {
				var result map[string]string
				json.Unmarshal(w.Body.Bytes(), &result)
				if result["encryptedToken"] != zoomHMAC(a.Config.ZoomWebhookSecret, "challenge") || result["plainToken"] != "challenge" {
					t.Fatal("invalid CRC")
				}
			}
		})
	}
	a.Config.ZoomWebhookSecret = ""
	w := httptest.NewRecorder()
	a.zoomWebhook(w, httptest.NewRequest("POST", "/api/webhooks/zoom", bytes.NewReader(body)))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
}
