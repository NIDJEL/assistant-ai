package yandexcalendar

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/integrations/oauth"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

type TokenRefresher interface {
	Refresh(context.Context, string) (repository.OAuthToken, error)
}
type Remote interface {
	EnsureCalendar(context.Context, string, string, string) (string, error)
	Put(context.Context, string, string, string, string) error
	Delete(context.Context, string, string, string) error
}
type Service struct {
	Store    repository.CalendarStore
	Events   repository.Store
	Remote   Remote
	Provider TokenRefresher
	Location *time.Location
	Logger   *slog.Logger
	key      cipher.AEAD
	mu       sync.Mutex
	wake     chan struct{}
}

func New(store repository.CalendarStore, events repository.Store, remote Remote, provider TokenRefresher, secret string, loc *time.Location, logger *slog.Logger) (*Service, error) {
	key := sha256.Sum256([]byte("rhythm-yandex-calendar-v1:" + secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Service{Store: store, Events: events, Remote: remote, Provider: provider, Location: loc, Logger: logger, key: aead, wake: make(chan struct{}, 1)}, nil
}
func (s *Service) encrypt(uid string, token repository.OAuthToken) ([]byte, error) {
	raw, err := json.Marshal(token)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, s.key.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return s.key.Seal(nonce, nonce, raw, []byte(uid)), nil
}
func (s *Service) decrypt(uid string, encrypted []byte) (repository.OAuthToken, error) {
	var token repository.OAuthToken
	n := s.key.NonceSize()
	if len(encrypted) < n {
		return token, ErrAuthorization
	}
	raw, err := s.key.Open(nil, encrypted[:n], encrypted[n:], []byte(uid))
	if err != nil {
		return token, ErrAuthorization
	}
	if json.Unmarshal(raw, &token) != nil || token.AccessToken == "" {
		return token, ErrAuthorization
	}
	return token, nil
}
func expiration(now time.Time, seconds int64) *time.Time {
	if seconds <= 0 {
		return nil
	}
	value := now.Add(time.Duration(seconds) * time.Second)
	return &value
}
func (s *Service) Connect(ctx context.Context, grant repository.OAuthGrant, now time.Time) error {
	uid := repository.UserID(ctx)
	if uid == "" || grant.Identity.Provider != "yandex" || grant.Identity.Subject == "" || grant.Login == "" || grant.Token.AccessToken == "" || grant.Token.ExpiresIn < 0 || grant.Token.ExpiresIn > 10*365*24*60*60 {
		return &domain.AppError{Code: "calendar_permission", Message: "Яндекс не предоставил доступ к календарю. Повторите подключение.", Status: 422}
	}
	if grant.Token.Scope != "" {
		allowed := false
		for _, scope := range strings.Fields(grant.Token.Scope) {
			if scope == "calendar:all" {
				allowed = true
			}
		}
		if !allowed {
			return &domain.AppError{Code: "calendar_permission", Message: "Разрешите чтение и изменение календарей в Яндексе", Status: 422}
		}
	}
	encrypted, err := s.encrypt(uid, grant.Token)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	err = s.Store.WithCalendarLock(ctx, func() error {
		return s.Store.SaveCalendarCredentials(ctx, repository.CalendarCredentials{Subject: grant.Identity.Subject, Login: grant.Login, Ciphertext: encrypted, ExpiresAt: expiration(now, grant.Token.ExpiresIn)})
	})
	if err == nil {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
	return err
}
func (s *Service) Disconnect(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Store.WithCalendarLock(ctx, func() error { return s.Store.DisconnectCalendar(ctx) })
}
func (s *Service) refresh(ctx context.Context, c *repository.CalendarCredentials, token *repository.OAuthToken) error {
	if s.Provider == nil || token.RefreshToken == "" {
		return ErrAuthorization
	}
	updated, err := s.Provider.Refresh(ctx, token.RefreshToken)
	if errors.Is(err, oauth.ErrTokenRejected) {
		return ErrAuthorization
	}
	if err != nil {
		return err
	}
	if updated.AccessToken == "" || updated.ExpiresIn < 0 || updated.ExpiresIn > 10*365*24*60*60 {
		return ErrAuthorization
	}
	if updated.RefreshToken == "" {
		updated.RefreshToken = token.RefreshToken
	}
	if updated.Scope == "" {
		updated.Scope = token.Scope
	}
	c.Ciphertext, err = s.encrypt(repository.UserID(ctx), updated)
	if err != nil {
		return err
	}
	c.ExpiresAt = expiration(time.Now(), updated.ExpiresIn)
	if err = s.Store.SaveCalendarCredentials(ctx, *c); err != nil {
		return err
	}
	*token = updated
	return nil
}
func (s *Service) sync(ctx context.Context) error {
	c, err := s.Store.CalendarCredentials(ctx)
	if err != nil {
		return err
	}
	if c == nil || c.Status == "disconnected" {
		return &domain.AppError{Code: "calendar_disconnected", Message: "Сначала подключите Яндекс Календарь", Status: 409}
	}
	token, err := s.decrypt(repository.UserID(ctx), c.Ciphertext)
	if err != nil {
		return err
	}
	refreshed := false
	if c.ExpiresAt != nil && time.Until(*c.ExpiresAt) < time.Minute {
		if err = s.refresh(ctx, c, &token); err != nil {
			return err
		}
		refreshed = true
	}
	// Retry authorization once after refreshing; never retry remote 401 forever.
	remoteCall := func(fn func() error) error {
		err := fn()
		if errors.Is(err, ErrAuthorization) && !refreshed {
			refreshed = true
			if err = s.refresh(ctx, c, &token); err != nil {
				return err
			}
			return fn()
		}
		return err
	}
	initial := c.CalendarURL == ""
	if initial || c.Status == "error" {
		previous := c.CalendarURL
		err = remoteCall(func() error {
			var e error
			c.CalendarURL, e = s.Remote.EnsureCalendar(ctx, c.Login, token.AccessToken, repository.UserID(ctx))
			return e
		})
		if err != nil {
			return err
		}
		if c.CalendarURL != previous {
			if err = s.Store.SetCalendarTarget(ctx, c.CalendarURL); err != nil {
				return err
			}
		}
	}
	changes, err := s.Store.CalendarChanges(ctx, 50)
	if err != nil {
		return err
	}
	if len(changes) > 0 {
		if branding, ok := s.Remote.(interface {
			RefreshBrand(context.Context, string, string, string) error
		}); ok {
			if err := remoteCall(func() error { return branding.RefreshBrand(ctx, c.CalendarURL, c.Login, token.AccessToken) }); err != nil {
				return err
			}
		}
	}
	for _, change := range changes {
		var event *domain.Event
		err = s.Events.Read(ctx, func(tx repository.Tx) error { var err error; event, err = tx.Get(ctx, change.EventID); return err })
		if err != nil {
			return err
		}
		target := strings.TrimRight(c.CalendarURL, "/") + "/" + url.PathEscape(eventResource(repository.UserID(ctx), change.EventID))
		err = remoteCall(func() error {
			if event == nil || event.Date == nil {
				return s.Remote.Delete(ctx, target, c.Login, token.AccessToken)
			}
			return s.Remote.Put(ctx, target, c.Login, token.AccessToken, eventCalendar(*event, repository.UserID(ctx), s.Location))
		})
		if err != nil {
			return err
		}
		if err = s.Store.AcknowledgeCalendarChange(ctx, change); err != nil {
			return err
		}
	}
	var stamp *time.Time
	if initial || len(changes) > 0 {
		now := time.Now()
		stamp = &now
	}
	return s.Store.CalendarSyncStatus(ctx, "connected", "", stamp)
}
func syncError(err error) (string, string) {
	if errors.Is(err, ErrAuthorization) {
		return "reauthorization_required", "Доступ к календарю истёк или отозван. Подключите Яндекс Календарь снова."
	}
	return "error", "Не удалось синхронизировать календарь. Изменения сохранены и будут отправлены повторно."
}
func (s *Service) Sync(ctx context.Context) error {
	if !s.mu.TryLock() {
		return &domain.AppError{Code: "calendar_busy", Message: "Синхронизация уже выполняется", Status: 409}
	}
	defer s.mu.Unlock()
	return s.Store.WithCalendarLock(ctx, func() error {
		err := s.sync(ctx)
		if err == nil {
			return nil
		}
		var app *domain.AppError
		if errors.As(err, &app) {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var remote *HTTPError
		if s.Logger != nil && errors.As(err, &remote) {
			s.Logger.Warn("calendar provider rejected request", "operation", remote.Operation, "status", remote.Status)
		}
		status, message := syncError(err)
		if updateErr := s.Store.CalendarSyncStatus(ctx, status, message, nil); updateErr != nil {
			return updateErr
		}
		return &domain.AppError{Code: "calendar_sync_failed", Message: message, Status: 502}
	})
}
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		ids, err := s.Store.CalendarUsers(ctx)
		if err != nil && ctx.Err() == nil && s.Logger != nil {
			s.Logger.Error("calendar queue unavailable")
		}
		for _, id := range ids {
			if ctx.Err() != nil {
				return
			}
			job, cancel := context.WithTimeout(repository.WithUser(ctx, id), 40*time.Second)
			err := s.Sync(job)
			cancel()
			if err != nil && s.Logger != nil && ctx.Err() == nil {
				s.Logger.Warn("calendar sync deferred", "user_id", id)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.wake:
		}
	}
}
