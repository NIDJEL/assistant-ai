package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type preferenceStore interface {
	Preferences(context.Context, string) (json.RawMessage, error)
	SavePreferences(context.Context, string, json.RawMessage, time.Time) error
}

func (a *API) preferences(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "profile"))
	if err != nil {
		a.fail(w, r, domain.Validation("Некорректный UUID профиля"))
		return
	}
	var value json.RawMessage
	if r.Method == http.MethodPut {
		if err = decode(w, r, &value); err != nil {
			a.fail(w, r, err)
			return
		}
		// Preferences are a bounded, versionable object, never executable data.
		var object map[string]json.RawMessage
		if err = json.Unmarshal(value, &object); err != nil || object == nil {
			a.fail(w, r, domain.Validation("Настройки должны быть объектом"))
			return
		}
		err = a.Calendar.Store.Write(r.Context(), func(tx repository.Tx) error {
			return tx.(preferenceStore).SavePreferences(r.Context(), id.String(), value, a.Clock.Now())
		})
	} else {
		err = a.Calendar.Store.Read(r.Context(), func(tx repository.Tx) error {
			var e error
			value, e = tx.(preferenceStore).Preferences(r.Context(), id.String())
			return e
		})
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, value)
}
