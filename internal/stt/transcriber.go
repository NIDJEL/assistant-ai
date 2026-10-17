// Package stt transcribes audio in memory. It does not persist audio or transcripts.
package stt

import (
	"context"
	"errors"
	"mime"
)

const MaxAudioBytes = 15 << 20

var (
	ErrUnavailable = errors.New("transcription unavailable")
	ErrTooLarge    = errors.New("audio too large")
	ErrEmpty       = errors.New("empty audio")
	ErrMIME        = errors.New("unsupported audio type")
)

type Transcriber interface {
	Transcribe(ctx context.Context, filename string, contentType string, audio []byte) (string, error)
}

func AudioMIME(value string) (string, error) {
	media, _, err := mime.ParseMediaType(value)
	if err == nil {
		switch media {
		case "audio/webm", "audio/ogg", "audio/wav", "audio/mpeg", "audio/mp4":
			return media, nil
		}
	}
	return "", ErrMIME
}

func Validate(contentType string, audio []byte) error {
	if len(audio) == 0 {
		return ErrEmpty
	}
	if len(audio) > MaxAudioBytes {
		return ErrTooLarge
	}
	_, err := AudioMIME(contentType)
	return err
}
