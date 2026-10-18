package api

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/tts"
)

func (a *API) ttsError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message := 503, "tts_unavailable", "Озвучивание временно недоступно. Попробуйте позже."
	switch {
	case errors.Is(err, tts.ErrRateLimited):
		status, code, message = 429, "tts_rate_limited", "Лимит озвучивания исчерпан. Попробуйте позже."
		w.Header().Set("Retry-After", "60")
	case errors.Is(err, tts.ErrVoice):
		status, code, message = 400, "tts_voice_unavailable", "Этот голос недоступен. Выберите другой в настройках."
	case errors.Is(err, tts.ErrInvalidText):
		status, code, message = 400, "tts_invalid_text", "Для озвучивания нужен обычный текст длиной до 2000 символов."
	case errors.Is(err, context.DeadlineExceeded):
		status, code, message = 504, "tts_timeout", "Голос не успел загрузиться. Попробуйте ещё раз."
	case errors.Is(err, context.Canceled):
		status, code, message = 408, "request_canceled", "Озвучивание отменено."
	}
	// Never log provider errors, response bodies, or user text.
	a.fail(w, r, &domain.AppError{Status: status, Code: code, Message: message})
}

func (a *API) ttsVoices(w http.ResponseWriter, r *http.Request) {
	if a.Synthesizer == nil {
		a.ttsError(w, r, tts.ErrUnavailable)
		return
	}
	voices, err := a.Synthesizer.Voices(r.Context())
	if err != nil {
		a.ttsError(w, r, err)
		return
	}
	voice := a.Config.AzureTTSVoice
	if voice == "" {
		voice = tts.DefaultVoice
	}
	write(w, 200, map[string]any{"voices": voices, "default_voice": voice})
}

func (a *API) synthesize(w http.ResponseWriter, r *http.Request) {
	input, ok := r.Context().Value(ttsInputKey{}).(synthesisInput)
	if !ok {
		if err := decode(w, r, &input); err != nil {
			a.fail(w, r, err)
			return
		}
	}
	if !tts.ValidText(input.Text) || tts.PrepareText(input.Text) == "" {
		a.ttsError(w, r, tts.ErrInvalidText)
		return
	}
	if input.Voice != "" && !tts.AllowedVoice(input.Voice) {
		a.ttsError(w, r, tts.ErrVoice)
		return
	}
	if a.Synthesizer == nil {
		a.ttsError(w, r, tts.ErrUnavailable)
		return
	}
	audio, err := a.Synthesizer.Synthesize(r.Context(), input.Text, input.Voice)
	if err != nil {
		a.ttsError(w, r, err)
		return
	}
	if len(audio) == 0 || len(audio) > tts.MaxAudioBytes {
		a.ttsError(w, r, tts.ErrUnavailable)
		return
	}
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	_, _ = w.Write(audio)
}

// Authenticated user limits are shared across sessions and cannot be bypassed by
// changing IP. Bounded state, one daily budget, and a minute request window.
func (a *API) ttsLimiter() func(http.Handler) http.Handler {
	type window struct {
		minute, day          time.Time
		requests, characters int
	}
	var mu sync.Mutex
	windows := map[string]window{}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uid := repository.UserID(r.Context())
			if a.Auth == nil || uid == "" {
				a.fail(w, r, &domain.AppError{Status: 401, Code: "unauthorized", Message: "Войдите в аккаунт"})
				return
			}
			now := time.Now().UTC()
			day := now.Truncate(24 * time.Hour)
			mu.Lock()
			for key, value := range windows {
				if value.day.Before(day) {
					delete(windows, key)
				}
			}
			slot, exists := windows[uid]
			if !slot.minute.After(now) {
				slot.minute = now.Add(time.Minute)
				slot.requests = 0
			}
			if slot.day != day {
				slot.day = day
				slot.characters = 0
			}
			limited := slot.requests >= 12 || slot.characters >= 6000 || (!exists && len(windows) >= 10000)
			if !limited {
				slot.requests++
				windows[uid] = slot
			}
			mu.Unlock()
			if limited {
				a.ttsError(w, r, tts.ErrRateLimited)
				return
			}
			if r.Method == http.MethodPost {
				// Decode once here to apply the budget before any upstream request.
				var input synthesisInput
				if err := decode(w, r, &input); err != nil {
					a.fail(w, r, err)
					return
				}
				if !tts.ValidText(input.Text) {
					a.ttsError(w, r, tts.ErrInvalidText)
					return
				}
				chars := utf8.RuneCountInString(input.Text)
				mu.Lock()
				slot = windows[uid]
				limited = slot.characters+chars > 6000
				if !limited {
					slot.characters += chars
					windows[uid] = slot
				}
				mu.Unlock()
				if limited {
					a.ttsError(w, r, tts.ErrRateLimited)
					return
				}
				// Pass the validated payload through context; do not retain user text.
				r = r.WithContext(context.WithValue(r.Context(), ttsInputKey{}, input))
			}
			next.ServeHTTP(w, r)
		})
	}
}

type ttsInputKey struct{}
type synthesisInput struct {
	Text  string `json:"text"`
	Voice string `json:"voice"`
}
