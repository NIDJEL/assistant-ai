package api

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/stt"
)

func (a *API) transcribe(w http.ResponseWriter, r *http.Request) {
	fail := func(status int, code, message string) {
		a.fail(w, r, &domain.AppError{Status: status, Code: code, Message: message})
	}
	if a.Auth == nil {
		fail(401, "unauthorized", "Войдите в аккаунт")
		return
	}
	// Unlike ParseMultipartForm, MultipartReader never spills user audio to disk.
	r.Body = http.MaxBytesReader(w, r.Body, stt.MaxAudioBytes+(64<<10))
	reader, err := r.MultipartReader()
	if err != nil {
		fail(400, "invalid_audio", "Ожидается multipart audio file")
		return
	}
	var audio []byte
	var filename, contentType string
	found := false
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			var max *http.MaxBytesError
			if errors.As(err, &max) {
				fail(413, "audio_too_large", "Запись должна быть не больше 15 МБ")
			} else {
				fail(400, "invalid_audio", "Не удалось прочитать запись")
			}
			return
		}
		if found || (part.FormName() != "file" && part.FormName() != "audio") || part.FileName() == "" {
			fail(400, "invalid_audio", "Передайте один audio file")
			return
		}
		found = true
		filename, contentType = part.FileName(), part.Header.Get("Content-Type")
		if _, err := stt.AudioMIME(contentType); err != nil {
			fail(415, "unsupported_audio", "Формат аудио не поддерживается")
			return
		}
		audio, err = io.ReadAll(io.LimitReader(part, stt.MaxAudioBytes+1))
		var max *http.MaxBytesError
		if len(audio) > stt.MaxAudioBytes || errors.As(err, &max) {
			fail(413, "audio_too_large", "Запись должна быть не больше 15 МБ")
			return
		}
		if err != nil {
			fail(400, "invalid_audio", "Не удалось прочитать запись")
			return
		}
	}
	if len(audio) == 0 {
		fail(400, "empty_audio", "Запись пуста. Попробуйте ещё раз")
		return
	}
	if a.Transcriber == nil {
		fail(503, "stt_unavailable", "Распознавание временно недоступно. Черновик сохранён")
		return
	}
	text, err := a.Transcriber.Transcribe(r.Context(), filename, contentType, audio)
	if err != nil || strings.TrimSpace(text) == "" {
		// Provider errors and response bodies must never reach logs or the client.
		fail(503, "stt_unavailable", "Распознавание временно недоступно. Черновик сохранён")
		return
	}
	write(w, 200, map[string]string{"text": text})
}
