package api

import (
	"net/http"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/notifications"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/go-chi/chi/v5"
)

func (a *API) pushStore(w http.ResponseWriter, r *http.Request) notifications.Store {
	if a.Auth == nil || repository.UserID(r.Context()) == "" {
		a.fail(w, r, &domain.AppError{Code: "unauthorized", Message: "Войдите, чтобы продолжить работу", Status: 401})
		return nil
	}
	store, ok := a.Calendar.Store.(notifications.Store)
	if !ok {
		a.fail(w, r, &domain.AppError{Code: "push_unavailable", Message: "Уведомления не настроены на сервере", Status: 503})
		return nil
	}
	w.Header().Set("Cache-Control", "no-store")
	return store
}
func (a *API) pushConfig(w http.ResponseWriter, r *http.Request) {
	if a.pushStore(w, r) == nil {
		return
	}
	write(w, 200, map[string]any{"enabled": a.Config.WebPush.Enabled(), "public_key": a.Config.WebPush.PublicKey})
}
func (a *API) pushSubscriptions(w http.ResponseWriter, r *http.Request) {
	store := a.pushStore(w, r)
	if store == nil {
		return
	}
	if r.Method == http.MethodGet {
		devices, err := store.Subscriptions(r.Context())
		if err != nil {
			a.fail(w, r, err)
			return
		}
		write(w, 200, map[string]any{"subscriptions": devices})
		return
	}
	if !a.Config.WebPush.Enabled() {
		a.fail(w, r, &domain.AppError{Code: "push_unavailable", Message: "Уведомления не настроены на сервере", Status: 503})
		return
	}
	var input notifications.Subscribe
	if err := decode(w, r, &input); err != nil {
		a.fail(w, r, domain.Validation("Некорректные настройки уведомлений"))
		return
	}
	device, err := store.Subscribe(r.Context(), input, r.UserAgent(), a.Clock.Now())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 201, map[string]any{"subscription": device})
}
func (a *API) deletePushSubscription(w http.ResponseWriter, r *http.Request) {
	store := a.pushStore(w, r)
	if store == nil {
		return
	}
	if err := store.Unsubscribe(r.Context(), chi.URLParam(r, "id"), a.Clock.Now()); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(204)
}
func (a *API) testPush(w http.ResponseWriter, r *http.Request) {
	store := a.pushStore(w, r)
	if store == nil {
		return
	}
	if !a.Config.WebPush.Enabled() {
		a.fail(w, r, &domain.AppError{Code: "push_unavailable", Message: "Уведомления не настроены на сервере", Status: 503})
		return
	}
	var input struct {
		SubscriptionID string `json:"subscription_id"`
	}
	if err := decode(w, r, &input); err != nil {
		a.fail(w, r, domain.Validation("Выберите устройство для проверки"))
		return
	}
	if err := store.QueueTest(r.Context(), input.SubscriptionID, a.Clock.Now()); err != nil {
		if app, ok := err.(*domain.AppError); ok && app.Status == 429 {
			w.Header().Set("Retry-After", "60")
		}
		a.fail(w, r, err)
		return
	}
	write(w, 202, map[string]any{"queued": true})
}
