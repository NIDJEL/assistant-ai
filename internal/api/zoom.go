package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/google/uuid"
	"io"
	"net/http"
	"strconv"
	"time"
)

func zoomHMAC(secret, message string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(message))
	return hex.EncodeToString(m.Sum(nil))
}
func (a *API) zoomWebhook(w http.ResponseWriter, r *http.Request) {
	secret := a.Config.ZoomWebhookSecret
	if secret == "" {
		http.Error(w, "Webhook not configured", http.StatusServiceUnavailable)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 256<<10))
	if err != nil {
		http.Error(w, "Invalid body", 413)
		return
	}
	stamp := r.Header.Get("x-zm-request-timestamp")
	sec, err := strconv.ParseInt(stamp, 10, 64)
	now := a.Clock.Now()
	if err != nil || sec < now.Unix()-300 || sec > now.Unix()+300 || !hmac.Equal([]byte(r.Header.Get("x-zm-signature")), []byte("v0="+zoomHMAC(secret, "v0:"+stamp+":"+string(raw)))) {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}
	var body struct {
		Event   string `json:"event"`
		EventTS int64  `json:"event_ts"`
		Payload struct {
			PlainToken string `json:"plainToken"`
			Object     struct {
				ID        json.RawMessage `json:"id"`
				UUID      string          `json:"uuid"`
				HostID    string          `json:"host_id"`
				StartTime *time.Time      `json:"start_time"`
				EndTime   *time.Time      `json:"end_time"`
			} `json:"object"`
		} `json:"payload"`
	}
	if json.Unmarshal(raw, &body) != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}
	if body.Event == "endpoint.url_validation" {
		if body.Payload.PlainToken == "" || len(body.Payload.PlainToken) > 1024 {
			http.Error(w, "Invalid challenge", 400)
			return
		}
		write(w, 200, map[string]string{"plainToken": body.Payload.PlainToken, "encryptedToken": zoomHMAC(secret, body.Payload.PlainToken)})
		return
	}
	if body.Event != "meeting.started" && body.Event != "meeting.ended" {
		w.WriteHeader(204)
		return
	}
	object := body.Payload.Object
	var meetingID string
	if json.Unmarshal(object.ID, &meetingID) != nil {
		meetingID = string(object.ID)
	}
	mid, err := strconv.ParseInt(meetingID, 10, 64)
	if err != nil || mid <= 0 || object.UUID == "" || len(object.UUID) > 512 || object.HostID == "" || len(object.HostID) > 256 || body.EventTS <= 0 {
		http.Error(w, "Invalid meeting event", 400)
		return
	}
	occurred := time.UnixMilli(body.EventTS)
	// A retried webhook has fresh request headers and can carry an older event_ts.
	if body.Event == "meeting.started" && object.StartTime != nil {
		occurred = *object.StartTime
	}
	if body.Event == "meeting.ended" && object.EndTime != nil {
		occurred = *object.EndTime
	}
	store, ok := a.Calendar.Store.(repository.ZoomStore)
	if !ok {
		http.Error(w, "Storage unavailable", 503)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2500*time.Millisecond)
	defer cancel()
	if err = store.ReceiveZoomEvent(ctx, repository.ZoomEvent{Event: body.Event, MeetingID: strconv.FormatInt(mid, 10), RunID: object.UUID, HostID: object.HostID, OccurredAt: occurred, StartedAt: object.StartTime}, a.Calendar.Location, now); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(204)
}

func (a *API) chatNotifications(w http.ResponseWriter, r *http.Request) {
	store, ok := a.Calendar.Store.(repository.ZoomStore)
	if !ok {
		write(w, 200, map[string]any{"exchanges": []domain.Exchange{}})
		return
	}
	if r.Method == "POST" {
		var body struct {
			IDs []string `json:"ids"`
		}
		if err := decode(w, r, &body); err != nil {
			a.fail(w, r, err)
			return
		}
		if len(body.IDs) > 200 {
			a.fail(w, r, domain.Validation("Слишком много сообщений"))
			return
		}
		for _, id := range body.IDs {
			if _, err := uuid.Parse(id); err != nil {
				a.fail(w, r, domain.Validation("Некорректный ID сообщения"))
				return
			}
		}
		if err := store.ReadChatNotifications(r.Context(), body.IDs, a.Clock.Now()); err != nil {
			a.fail(w, r, err)
			return
		}
		w.WriteHeader(204)
		return
	}
	list, err := store.ChatNotifications(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	for i := range list {
		if err = a.Assistant.RefreshZoomCards(r.Context(), &list[i].Response); err != nil {
			a.fail(w, r, err)
			return
		}
	}
	write(w, 200, map[string]any{"exchanges": list})
}
