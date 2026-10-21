// Package tts synthesizes assistant replies independently of STT and NLU.
package tts

import (
	"bytes"
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	DefaultVoice  = "ru-RU-DmitryNeural"
	MaxTextRunes  = 2000
	MaxAudioBytes = 4 << 20
	OutputFormat  = "audio-16khz-128kbitrate-mono-mp3"
	cacheBytes    = 16 << 20
	cacheEntries  = 64
	cacheTTL      = 15 * time.Minute
	// With one process, this bounds Azure usage to at most 372k characters/31 days.
	dailyCharacters = 12000
)

var (
	ErrUnavailable = errors.New("speech synthesis unavailable")
	ErrRateLimited = errors.New("speech synthesis rate limited")
	ErrInvalidText = errors.New("invalid speech text")
	ErrVoice       = errors.New("voice unavailable")
	regionPattern  = regexp.MustCompile(`^[a-z][a-z0-9]{1,39}$`)
)

type Voice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

var allowedVoices = []Voice{{DefaultVoice, "Гена, мужской"}}

func AllowedVoice(id string) bool {
	for _, v := range allowedVoices {
		if v.ID == id {
			return true
		}
	}
	return false
}

type Synthesizer interface {
	Synthesize(context.Context, string, string) ([]byte, error)
	Voices(context.Context) ([]Voice, error)
}

type cachedAudio struct {
	key     [32]byte
	audio   []byte
	expires time.Time
}
type AzureSynthesizer struct {
	key, endpoint, defaultVoice string
	client                      *http.Client
	gate, voicesGate            chan struct{}
	mu                          sync.Mutex
	voices                      []Voice
	voicesUntil                 time.Time
	cache                       map[[32]byte]*list.Element
	lru                         *list.List
	bytes, dailyUsed            int
	day                         string
}

func NewAzure(key, region, voice string) (*AzureSynthesizer, error) {
	if region == "" {
		region = "eastus"
	}
	if voice == "" {
		voice = DefaultVoice
	}
	if !regionPattern.MatchString(region) || !AllowedVoice(voice) {
		return nil, errors.New("invalid Azure region or default voice")
	}
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("AZURE_SPEECH_KEY is required for Azure TTS")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil // No Groq proxy or process-wide routing changes.
	return &AzureSynthesizer{key: key, endpoint: "https://" + region + ".tts.speech.microsoft.com/cognitiveservices", defaultVoice: voice,
		client: &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		gate:   make(chan struct{}, 1), voicesGate: make(chan struct{}, 1), cache: make(map[[32]byte]*list.Element), lru: list.New()}, nil
}

func (s *AzureSynthesizer) Close() { s.client.CloseIdleConnections() }

func (s *AzureSynthesizer) fetch(ctx context.Context, method, path, body string, limit int) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, s.endpoint+path, strings.NewReader(body))
	if err != nil {
		return nil, ErrUnavailable
	}
	req.Header.Set("Ocp-Apim-Subscription-Key", s.key)
	req.Header.Set("User-Agent", "Gena-TTS")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/ssml+xml")
		req.Header.Set("X-Microsoft-OutputFormat", OutputFormat)
	}
	r, err := s.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrUnavailable
	}
	defer r.Body.Close()
	if r.StatusCode == 429 {
		return nil, ErrRateLimited
	}
	// No automatic retry: a failed or cancelled request may already consume quota.
	if r.StatusCode != 200 {
		return nil, ErrUnavailable
	}
	if method == http.MethodPost {
		media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if media != "audio/mpeg" {
			return nil, ErrUnavailable
		}
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, int64(limit)+1))
	if err != nil || len(data) == 0 || len(data) > limit {
		return nil, ErrUnavailable
	}
	return data, nil
}

func (s *AzureSynthesizer) Voices(ctx context.Context) ([]Voice, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	select {
	case s.voicesGate <- struct{}{}:
		defer func() { <-s.voicesGate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	s.mu.Lock()
	if time.Now().Before(s.voicesUntil) {
		result := append([]Voice(nil), s.voices...)
		s.mu.Unlock()
		return result, nil
	}
	s.mu.Unlock()
	data, err := s.fetch(ctx, http.MethodGet, "/voices/list", "", 2<<20)
	if err != nil {
		return nil, err
	}
	var available []struct {
		ShortName string
		Locale    string
	}
	if json.Unmarshal(data, &available) != nil {
		return nil, ErrUnavailable
	}
	result := []Voice{}
	for _, allowed := range allowedVoices {
		for _, v := range available {
			if v.ShortName == allowed.ID && v.Locale == "ru-RU" {
				result = append(result, allowed)
				break
			}
		}
	}
	s.mu.Lock()
	s.voices = result
	s.voicesUntil = time.Now().Add(10 * time.Minute)
	s.mu.Unlock()
	return append([]Voice(nil), result...), nil
}

func ssml(text, voice string) string {
	var escaped bytes.Buffer
	_ = xml.EscapeText(&escaped, []byte(text))
	return `<speak version="1.0" xmlns="http://www.w3.org/2001/10/synthesis" xml:lang="ru-RU"><voice name="` + voice + `"><prosody rate="-5%">` + escaped.String() + `</prosody></voice></speak>`
}

func (s *AzureSynthesizer) Synthesize(ctx context.Context, text, voice string) ([]byte, error) {
	if !ValidText(text) {
		return nil, ErrInvalidText
	}
	text = PrepareText(text)
	if text == "" {
		return nil, ErrInvalidText
	}
	if voice == "" {
		voice = s.defaultVoice
	}
	if !AllowedVoice(voice) {
		return nil, ErrVoice
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	// Serializes misses, coalesces identical concurrent requests, bounds memory and spend.
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	body := ssml(text, voice)
	key := sha256.Sum256([]byte(OutputFormat + body))
	s.mu.Lock()
	if e := s.cache[key]; e != nil && time.Now().Before(e.Value.(cachedAudio).expires) {
		s.lru.MoveToFront(e)
		audio := bytes.Clone(e.Value.(cachedAudio).audio)
		s.mu.Unlock()
		return audio, nil
	}
	s.mu.Unlock()
	voices, err := s.Voices(ctx)
	if err != nil {
		return nil, err
	}
	found := false
	for _, v := range voices {
		if v.ID == voice {
			found = true
		}
	}
	if !found {
		return nil, ErrVoice
	}
	s.mu.Lock()
	day := time.Now().UTC().Format("2006-01-02")
	if s.day != day {
		s.day = day
		s.dailyUsed = 0
	}
	chars := utf8.RuneCountInString(text)
	if s.dailyUsed+chars > dailyCharacters {
		s.mu.Unlock()
		return nil, ErrRateLimited
	}
	s.dailyUsed += chars // Reserve before the call, including ambiguous upstream failures.
	s.mu.Unlock()
	audio, err := s.fetch(ctx, http.MethodPost, "/v1", body, MaxAudioBytes)
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if old := s.cache[key]; old != nil {
		s.bytes -= len(old.Value.(cachedAudio).audio)
		s.lru.Remove(old)
		delete(s.cache, key)
	}
	for e := s.lru.Back(); e != nil && (s.bytes+len(audio) > cacheBytes || s.lru.Len() >= cacheEntries); e = s.lru.Back() {
		entry := e.Value.(cachedAudio)
		s.bytes -= len(entry.audio)
		delete(s.cache, entry.key)
		s.lru.Remove(e)
	}
	s.cache[key] = s.lru.PushFront(cachedAudio{key, bytes.Clone(audio), time.Now().Add(cacheTTL)})
	s.bytes += len(audio)
	return audio, nil
}
