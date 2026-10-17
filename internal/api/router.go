package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/NIDJEL/assistant-ai/internal/api/dto"
	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/auth"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/clock"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/integrations/yandexcalendar"
	"github.com/NIDJEL/assistant-ai/internal/llm"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/stt"
	"github.com/NIDJEL/assistant-ai/migrations"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type API struct {
	Transcriber  stt.Transcriber
	Auth         *auth.Service
	CalendarSync *yandexcalendar.Service
	Config       config.Config
	Clock        clock.Clock
	Calendar     *calendar.Service
	Assistant    *assistant.Service
	Logger       *slog.Logger
}

func (a *API) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(a.logging)
	r.Use(a.recover)
	r.Use(a.security)
	r.Use(middleware.Timeout(45 * time.Second))
	r.Get("/health", a.health)
	if a.Auth != nil {
		a.authRoutes(r)
	}
	r.Group(func(r chi.Router) {
		if a.Auth != nil {
			r.Use(a.requireUser)
			r.Use(a.csrf)
		}
		r.Get("/api/preferences/{profile}", a.preferences)
		r.Put("/api/preferences/{profile}", a.preferences)
		r.Post("/api/chat", a.chat)
		r.Post("/api/voice/transcribe", a.transcribe)
		r.Get("/api/history/{session_id}", a.history)
		r.Get("/api/events", a.events(domain.EventUnknown))
		r.Get("/api/tasks", a.events(domain.EventTask))
		r.Get("/api/meetings", a.events(domain.EventMeeting))
		r.Get("/api/reminders", a.events(domain.EventReminder))
		r.Get("/api/reminders/due", a.due)
		r.Get("/api/notifications/push/config", a.pushConfig)
		r.Get("/api/notifications/push/subscriptions", a.pushSubscriptions)
		r.Post("/api/notifications/push/subscriptions", a.pushSubscriptions)
		r.Delete("/api/notifications/push/subscriptions/{id}", a.deletePushSubscription)
		r.Post("/api/notifications/push/test", a.testPush)
		r.Get("/api/calendar", a.events(domain.EventUnknown))
		r.Get("/api/archive", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			q.Set("deleted", "true")
			r.URL.RawQuery = q.Encode()
			a.events(domain.EventUnknown)(w, r)
		})
		r.Get("/api/integrations", a.integrations)
		r.Post("/api/integrations/yandex/sync", a.syncYandexCalendar)
		r.Delete("/api/integrations/yandex", a.disconnectYandexCalendar)
		r.Get("/api/events/{id}", a.get)
		r.Post("/api/events", a.create)
		r.Patch("/api/events/{id}", a.update)
		r.Delete("/api/events/{id}", a.delete)
		r.Post("/api/events/{id}/restore", a.restore)
		r.Post("/api/import-demo", a.importDemo)
	})
	r.Get("/openapi.json", func(w http.ResponseWriter, r *http.Request) { write(w, 200, OpenAPI()) })
	r.Get("/docs", a.docs)
	r.Handle("/diagrams/*", http.StripPrefix("/diagrams/", http.FileServer(http.Dir(filepath.Join(a.Config.Root, "docs/diagrams")))))
	r.Get("/architecture", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(a.Config.Root, "docs/architecture.html"))
	})
	a.frontendRoutes(r)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		a.fail(w, r, &domain.AppError{Code: "not_found", Message: "Маршрут не найден", Status: 404})
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		a.fail(w, r, &domain.AppError{Code: "method_not_allowed", Message: "Метод не поддерживается", Status: 405})
	})
	return r
}
func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func (a *API) fail(w http.ResponseWriter, r *http.Request, err error) {
	app := &domain.AppError{Code: "internal_error", Message: "Внутренняя ошибка сервера", Status: 500}
	if !errors.As(err, &app) {
		if errors.Is(err, context.DeadlineExceeded) {
			app = &domain.AppError{Code: "timeout", Message: "Превышено время ожидания", Status: 504}
		} else if errors.Is(err, context.Canceled) {
			app = &domain.AppError{Code: "request_canceled", Message: "Запрос отменён", Status: 408}
		} else {
			a.Logger.ErrorContext(r.Context(), "request failed", "request_id", middleware.GetReqID(r.Context()), "error", err)
		}
	}
	payload := map[string]any{"error": map[string]any{"code": app.Code, "message": app.Message}, "detail": app.Message, "request_id": middleware.GetReqID(r.Context())}
	if len(app.Conflicts) > 0 {
		payload["conflicts"] = app.Conflicts
	}
	write(w, app.Status, payload)
}
func decode(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var max *http.MaxBytesError
		if errors.As(err, &max) {
			return &domain.AppError{Code: "body_too_large", Message: "Слишком большой запрос", Status: 413}
		}
		return domain.Validation("Некорректный JSON: " + err.Error())
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		var max *http.MaxBytesError
		if errors.As(err, &max) {
			return &domain.AppError{Code: "body_too_large", Message: "Слишком большой запрос", Status: 413}
		}
		return domain.Validation("В запросе должно быть одно JSON-значение")
	}
	return nil
}
func id(r *http.Request) (int64, error) {
	value, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || value < 1 {
		return 0, domain.Validation("Некорректный номер записи")
	}
	return value, nil
}
func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if a.Auth != nil {
		u, err := a.currentUser(r)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		count := 0
		if u != nil {
			count, err = a.Calendar.Count(repository.WithUser(r.Context(), u.ID))
		} else if p, ok := a.Calendar.Store.(interface{ Ping(context.Context) error }); ok {
			err = p.Ping(r.Context())
		}
		if err != nil {
			a.fail(w, r, err)
			return
		}
		write(w, 200, map[string]any{"status": "ok", "events": count, "timezone": a.Config.Timezone, "nlp_provider": a.Config.EffectiveNLPProvider(), "now": a.Clock.Now(), "backend": "go", "storage": "postgresql", "schema_version": 1})
		return
	}
	count, err := a.Calendar.Count(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, map[string]any{"status": "ok", "events": count, "timezone": a.Config.Timezone, "nlp_provider": a.Config.EffectiveNLPProvider(), "now": a.Clock.Now(), "backend": "go", "schema_version": migrations.LatestVersion})
}
func (a *API) chat(w http.ResponseWriter, r *http.Request) {
	var body dto.Chat
	if err := decode(w, r, &body); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := body.Validate(); err != nil {
		a.fail(w, r, err)
		return
	}
	result, err := a.Assistant.Respond(llm.WithSource(r.Context(), body.Source), body.Message, body.SessionID, body.RequestID, a.Clock.Now())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, result)
}
func (a *API) history(w http.ResponseWriter, r *http.Request) {
	session, err := uuid.Parse(chi.URLParam(r, "session_id"))
	if err != nil {
		a.fail(w, r, domain.Validation("Некорректный UUID сессии"))
		return
	}
	history, state, err := a.Assistant.History(r.Context(), session.String())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, map[string]any{"exchanges": history, "state": state})
}
func (a *API) events(fixed domain.EventType) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		kind, err := domain.ParseEventType(q.Get("kind"))
		if err != nil {
			a.fail(w, r, domain.Validation(err.Error()))
			return
		}
		if fixed != domain.EventUnknown {
			kind = fixed
		}
		status, err := domain.ParseStatus(q.Get("status"))
		if err != nil {
			a.fail(w, r, domain.Validation(err.Error()))
			return
		}
		filter := domain.Filter{Kind: kind, Status: status}
		if raw := q.Get("deleted"); raw != "" {
			filter.Deleted, err = strconv.ParseBool(raw)
			if err != nil {
				a.fail(w, r, domain.Validation("deleted должен быть true или false"))
				return
			}
		}
		for name, target := range map[string]**domain.Date{"start": &filter.Start, "end": &filter.End} {
			if raw := q.Get(name); raw != "" {
				date, err := domain.ParseDate(raw)
				if err != nil {
					a.fail(w, r, domain.Validation("Некорректная дата "+name))
					return
				}
				*target = date
			}
		}
		limit, offset := 100, 0
		for name, target := range map[string]*int{"limit": &limit, "offset": &offset} {
			if raw := q.Get(name); raw != "" {
				n, err := strconv.Atoi(raw)
				if err != nil {
					a.fail(w, r, domain.Validation("Некорректная пагинация"))
					return
				}
				*target = n
			}
		}
		if limit < 1 || limit > 200 || offset < 0 || utf8.RuneCountInString(q.Get("q")) > 200 {
			a.fail(w, r, domain.Validation("Некорректные параметры списка"))
			return
		}
		result, err := a.Calendar.List(r.Context(), filter, q.Get("q"), limit, offset)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		write(w, 200, result)
	}
}
func (a *API) due(w http.ResponseWriter, r *http.Request) {
	now := a.Clock.Now()
	events, err := a.Calendar.Due(r.Context(), now)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, map[string]any{"events": events, "now": now})
}
func (a *API) importDemo(w http.ResponseWriter, r *http.Request) {
	result, err := a.Calendar.ImportDemo(r.Context(), a.Clock.Now())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, result)
}
func (a *API) get(w http.ResponseWriter, r *http.Request) {
	eventID, err := id(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	e, err := a.Calendar.Get(r.Context(), eventID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, e)
}
func (a *API) create(w http.ResponseWriter, r *http.Request) {
	var body dto.Event
	if err := decode(w, r, &body); err != nil {
		a.fail(w, r, err)
		return
	}
	value, err := body.Domain()
	if err != nil {
		a.fail(w, r, err)
		return
	}
	e, err := a.Calendar.Create(r.Context(), value, a.Clock.Now(), body.Force)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 201, e)
}
func (a *API) update(w http.ResponseWriter, r *http.Request) {
	eventID, err := id(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var body dto.Patch
	if err = decode(w, r, &body); err != nil {
		a.fail(w, r, err)
		return
	}
	patch, err := body.Domain()
	if err != nil {
		a.fail(w, r, domain.Validation(err.Error()))
		return
	}
	e, err := a.Calendar.Update(r.Context(), eventID, patch, a.Clock.Now())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, e)
}
func (a *API) delete(w http.ResponseWriter, r *http.Request) {
	eventID, err := id(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if err = a.Calendar.Delete(r.Context(), eventID); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(204)
}
func (a *API) restore(w http.ResponseWriter, r *http.Request) {
	eventID, err := id(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var body struct {
		Force bool `json:"force"`
	}
	if err = decode(w, r, &body); err != nil {
		a.fail(w, r, err)
		return
	}
	e, err := a.Calendar.Restore(r.Context(), eventID, a.Clock.Now(), body.Force)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, e)
}
