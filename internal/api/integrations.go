package api

import (
	"context"
	"net/http"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

func (a *API) integrations(w http.ResponseWriter, r *http.Request) {
	connections := []repository.CalendarConnection{}
	if store, ok := a.Calendar.Store.(repository.CalendarStore); ok {
		var err error
		connections, err = store.Connections(r.Context())
		if err != nil {
			a.fail(w, r, err)
			return
		}
	}
	configured := a.CalendarSync != nil && a.Auth != nil && a.Auth.Providers["yandex"] != nil
	google, zoom := false, false
	if a.Cloud != nil {
		list, err := a.Cloud.Store.CloudConnections(r.Context())
		if err != nil {
			a.fail(w, r, err)
			return
		}
		connections = append(connections, list...)
		google, zoom = a.Cloud.Providers["google"] != nil, a.Cloud.Providers["zoom"] != nil
	}
	write(w, 200, map[string]any{"connections": connections, "yandex_configured": configured, "google_configured": google, "zoom_configured": zoom})
}
func (a *API) syncYandexCalendar(w http.ResponseWriter, r *http.Request) {
	if a.CalendarSync == nil {
		a.fail(w, r, &domain.AppError{Code: "calendar_unavailable", Message: "Подключение календаря не настроено", Status: 503})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	if err := a.CalendarSync.Sync(ctx); err != nil {
		a.fail(w, r, err)
		return
	}
	a.integrations(w, r)
}
func (a *API) disconnectYandexCalendar(w http.ResponseWriter, r *http.Request) {
	if a.CalendarSync == nil {
		a.fail(w, r, &domain.AppError{Code: "calendar_unavailable", Message: "Подключение календаря не настроено", Status: 503})
		return
	}
	if err := a.CalendarSync.Disconnect(r.Context()); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(204)
}
