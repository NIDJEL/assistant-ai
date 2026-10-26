package api

import (
	"context"
	"net/http"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/google/uuid"
)

func (a *API) chatNotifications(w http.ResponseWriter, r *http.Request) {
	store, ok := a.Calendar.Store.(interface {
		ChatNotifications(context.Context) ([]domain.Exchange, error)
		ReadChatNotifications(context.Context, []string, time.Time) error
	})
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
	write(w, 200, map[string]any{"exchanges": list})
}
