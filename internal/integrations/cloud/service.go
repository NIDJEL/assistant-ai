// Package cloud implements per-user Google Calendar and Zoom integrations.
package cloud

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/integrations/oauth"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

type Service struct {
	Store                repository.CloudStore
	Providers            map[string]*oauth.Provider
	Location             *time.Location
	Client               *http.Client
	GoogleBase, ZoomBase string
	key                  cipher.AEAD
}

func Google(id, secret, redirect string) *oauth.Provider {
	return &oauth.Provider{Name: "google", ClientID: id, ClientSecret: secret, RedirectURI: redirect, AuthorizeEndpoint: "https://accounts.google.com/o/oauth2/v2/auth", TokenEndpoint: "https://oauth2.googleapis.com/token", UserEndpoint: "https://openidconnect.googleapis.com/v1/userinfo"}
}
func Zoom(id, secret, redirect string) *oauth.Provider {
	return &oauth.Provider{Name: "zoom", ClientID: id, ClientSecret: secret, RedirectURI: redirect, AuthorizeEndpoint: "https://zoom.us/oauth/authorize", TokenEndpoint: "https://zoom.us/oauth/token", UserEndpoint: "https://api.zoom.us/v2/users/me"}
}
func New(store repository.CloudStore, providers map[string]*oauth.Provider, secret string, loc *time.Location) (*Service, error) {
	if len(secret) < 32 {
		return nil, errors.New("integration encryption requires SESSION_SECRET")
	}
	k := sha256.Sum256([]byte("gena-cloud-v1:" + secret))
	b, err := aes.NewCipher(k[:])
	if err != nil {
		return nil, err
	}
	a, err := cipher.NewGCM(b)
	if err != nil {
		return nil, err
	}
	return &Service{Store: store, Providers: providers, Location: loc, key: a, GoogleBase: "https://www.googleapis.com/calendar/v3", ZoomBase: "https://api.zoom.us/v2", Client: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func unavailable(p string) error {
	return &domain.AppError{Code: "integration_unavailable", Message: "Подключите " + p + " в разделе «Интеграции»", Status: 409}
}
func (s *Service) encrypt(ctx context.Context, p string, t repository.OAuthToken) ([]byte, error) {
	raw, err := json.Marshal(t)
	if err != nil {
		return nil, err
	}
	n := make([]byte, s.key.NonceSize())
	if _, err = rand.Read(n); err != nil {
		return nil, err
	}
	return s.key.Seal(n, n, raw, []byte(repository.UserID(ctx)+":"+p)), nil
}
func (s *Service) decrypt(ctx context.Context, c *repository.CloudConnection) (repository.OAuthToken, error) {
	var t repository.OAuthToken
	n := s.key.NonceSize()
	if len(c.Ciphertext) < n {
		return t, oauth.ErrTokenRejected
	}
	raw, err := s.key.Open(nil, c.Ciphertext[:n], c.Ciphertext[n:], []byte(repository.UserID(ctx)+":"+c.Provider))
	if err != nil || json.Unmarshal(raw, &t) != nil || t.AccessToken == "" {
		return t, oauth.ErrTokenRejected
	}
	return t, nil
}
func (s *Service) Connect(ctx context.Context, g repository.OAuthGrant, now time.Time) error {
	p := g.Identity.Provider
	if s.Providers[p] == nil || repository.UserID(ctx) == "" || g.Identity.Subject == "" || g.Token.AccessToken == "" || g.Token.RefreshToken == "" || g.Token.ExpiresIn <= 0 || g.Token.ExpiresIn > 31536000 {
		return domain.Validation("Провайдер не предоставил фоновый доступ. Подключите аккаунт заново и разрешите необходимые права.")
	}
	if p == "google" {
		for _, scope := range []string{"https://www.googleapis.com/auth/calendar.events", "https://www.googleapis.com/auth/calendar.calendarlist.readonly"} {
			if !strings.Contains(" "+g.Token.Scope+" ", " "+scope+" ") {
				return domain.Validation("Разрешите чтение календарей и изменение событий Google Calendar")
			}
		}
	}
	return s.Store.WithCloudLock(ctx, p, func() error {
		previous, err := s.Store.CloudConnection(ctx, p)
		if err != nil {
			return err
		}
		c := repository.CloudConnection{Provider: p, Subject: g.Identity.Subject, Login: g.Login, Status: "connected"}
		expiry := now.Add(time.Duration(g.Token.ExpiresIn) * time.Second)
		c.ExpiresAt = &expiry
		if previous != nil && previous.Subject == c.Subject {
			c.CalendarID, c.SyncToken = previous.CalendarID, previous.SyncToken
		}
		// Remote mappings belong to the connected account; switching accounts requires
		// an explicit disconnect first, keeping old meetings outside another account.
		if previous != nil && previous.Subject != c.Subject && previous.Status != "disconnected" {
			return domain.Validation("Сначала отключите прежний аккаунт в разделе «Интеграции».")
		}
		c.Ciphertext, err = s.encrypt(ctx, p, g.Token)
		if err != nil {
			return err
		}
		return s.Store.SaveCloudConnection(ctx, c)
	})
}
func (s *Service) Ready(ctx context.Context, p string) error {
	if s == nil || s.Providers[p] == nil {
		return unavailable(p)
	}
	c, err := s.Store.CloudConnection(ctx, p)
	if err != nil {
		return err
	}
	if c == nil || c.Status == "disconnected" || c.Status == "reauthorization_required" {
		return unavailable(p)
	}
	if p == "google" && c.CalendarID == "" {
		return &domain.AppError{Code: "calendar_not_selected", Message: "Выберите календарь Google в разделе «Интеграции»", Status: 409}
	}
	return nil
}
func (s *Service) refresh(ctx context.Context, c *repository.CloudConnection, t *repository.OAuthToken) error {
	if t.RefreshToken == "" {
		return oauth.ErrTokenRejected
	}
	next, err := s.Providers[c.Provider].Refresh(ctx, t.RefreshToken)
	if err != nil {
		return err
	}
	if next.ExpiresIn <= 0 || next.ExpiresIn > 31536000 {
		return oauth.ErrTokenRejected
	}
	if next.RefreshToken == "" {
		next.RefreshToken = t.RefreshToken
	}
	if next.Scope == "" {
		next.Scope = t.Scope
	}
	c.Ciphertext, err = s.encrypt(ctx, c.Provider, next)
	if err != nil {
		return err
	}
	expiry := time.Now().Add(time.Duration(next.ExpiresIn) * time.Second)
	c.ExpiresAt = &expiry
	if err = s.Store.SaveCloudConnection(ctx, *c); err != nil {
		return err
	}
	*t = next
	return nil
}

type remoteError struct{ status int }

func (e *remoteError) Error() string {
	return fmt.Sprintf("integration provider returned HTTP %d", e.status)
}
func status(err error, n int) bool { var e *remoteError; return errors.As(err, &e) && e.status == n }
func (s *Service) call(ctx context.Context, c *repository.CloudConnection, t *repository.OAuthToken, method, endpoint string, body any, etag string, out any) error {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(data))
		if err != nil {
			return errors.New("invalid integration request")
		}
		req.Header.Set("Authorization", "Bearer "+t.AccessToken)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if etag != "" {
			req.Header.Set("If-Match", etag)
		}
		res, err := s.Client.Do(req)
		if err != nil {
			return errors.New("integration provider unavailable")
		}
		if res.StatusCode == 401 {
			res.Body.Close()
			if attempt == 0 {
				if err = s.refresh(ctx, c, t); err != nil {
					return err
				}
				continue
			}
			return oauth.ErrTokenRejected
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			res.Body.Close()
			return &remoteError{res.StatusCode}
		}
		if out != nil {
			err = json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(out)
		}
		res.Body.Close()
		return err
	}
	return oauth.ErrTokenRejected
}
func (s *Service) authorized(ctx context.Context, p string, fn func(*repository.CloudConnection, *repository.OAuthToken) error) error {
	if s == nil || s.Providers[p] == nil {
		return unavailable(p)
	}
	return s.Store.WithCloudLock(ctx, p, func() error {
		c, err := s.Store.CloudConnection(ctx, p)
		if err != nil {
			return err
		}
		if c == nil || c.Status == "disconnected" {
			return unavailable(p)
		}
		t, err := s.decrypt(ctx, c)
		if err == nil && c.ExpiresAt != nil && time.Until(*c.ExpiresAt) < time.Minute {
			err = s.refresh(ctx, c, &t)
		}
		if err == nil {
			err = fn(c, &t)
		}
		if err == nil {
			return nil
		}
		state, message := "error", "Сервис временно недоступен. Изменения сохранены и будут отправлены повторно."
		if errors.Is(err, oauth.ErrTokenRejected) {
			state, message = "reauthorization_required", "Доступ истёк или отозван. Подключите аккаунт заново."
		}
		if status(err, 412) {
			state, message = "error", "Событие изменено в Google Calendar. Повторите синхронизацию для согласования изменений."
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if saveErr := s.Store.CloudStatus(ctx, p, state, message, false); saveErr != nil {
			return saveErr
		}
		var app *domain.AppError
		if errors.As(err, &app) {
			return err
		}
		return &domain.AppError{Code: "integration_sync_failed", Message: message, Status: 502}
	})
}
func (s *Service) Disconnect(ctx context.Context, p string) error {
	return s.Store.WithCloudLock(ctx, p, func() error { return s.Store.DisconnectCloud(ctx, p) })
}
func (s *Service) Sync(ctx context.Context, p string) error {
	return s.authorized(ctx, p, func(c *repository.CloudConnection, t *repository.OAuthToken) error {
		var err error
		if p == "zoom" {
			err = s.pushZoom(ctx, c, t)
		} else {
			if c.CalendarID == "" {
				return &domain.AppError{Code: "calendar_not_selected", Message: "Выберите календарь Google", Status: 409}
			}
			// Pull first. Local changes in the outbox take priority, while a clean local
			// record receives remote edits. Writes use current provider ETags.
			err = s.pullGoogle(ctx, c, t)
			if err == nil {
				err = s.pushGoogle(ctx, c, t)
			}
		}
		if err != nil {
			return err
		}
		return s.Store.CloudStatus(ctx, p, "connected", "", true)
	})
}
func (s *Service) Run(ctx context.Context) {
	timer := time.NewTicker(30 * time.Second)
	defer timer.Stop()
	for {
		users, err := s.Store.CloudUsers(ctx)
		if err == nil {
			for _, u := range users {
				if ctx.Err() != nil {
					return
				}
				job, cancel := context.WithTimeout(repository.WithUser(ctx, u[0]), 40*time.Second)
				_ = s.Sync(job, u[1])
				cancel()
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}

type Calendar struct {
	ID         string `json:"id"`
	Summary    string `json:"summary"`
	AccessRole string `json:"accessRole"`
	Primary    bool   `json:"primary"`
}

func (s *Service) Calendars(ctx context.Context) ([]Calendar, error) {
	list := []Calendar{}
	err := s.authorized(ctx, "google", func(c *repository.CloudConnection, t *repository.OAuthToken) error {
		page := ""
		for i := 0; i < 100; i++ {
			var v struct {
				Items []Calendar `json:"items"`
				Next  string     `json:"nextPageToken"`
			}
			q := url.Values{"minAccessRole": {"writer"}, "maxResults": {"250"}}
			if page != "" {
				q.Set("pageToken", page)
			}
			if err := s.call(ctx, c, t, "GET", s.GoogleBase+"/users/me/calendarList?"+q.Encode(), nil, "", &v); err != nil {
				return err
			}
			list = append(list, v.Items...)
			if v.Next == "" {
				return nil
			}
			page = v.Next
		}
		return errors.New("calendar list too large")
	})
	return list, err
}
func (s *Service) SelectCalendar(ctx context.Context, id string) error {
	if id == "" || len(id) > 1024 {
		return domain.Validation("Выберите календарь")
	}
	list, err := s.Calendars(ctx)
	if err != nil {
		return err
	}
	found := false
	for _, c := range list {
		if c.ID == id && (c.AccessRole == "owner" || c.AccessRole == "writer") {
			found = true
		}
	}
	if !found {
		return domain.Validation("Нет права записи в выбранный календарь")
	}
	return s.Store.WithCloudLock(ctx, "google", func() error {
		c, err := s.Store.CloudConnection(ctx, "google")
		if err != nil {
			return err
		}
		if c == nil || c.Status == "disconnected" {
			return unavailable("google")
		}
		if c.CalendarID == id {
			return nil
		}
		return s.Store.SelectCloudCalendar(ctx, id)
	})
}

func (s *Service) event(ctx context.Context, id int64) (e *domain.Event, err error) {
	err = s.Store.Read(ctx, func(tx repository.Tx) error { e, err = tx.Get(ctx, id); return err })
	return
}
func (s *Service) SyncEvent(ctx context.Context, id int64, google, zoom bool) (*domain.Event, error) {
	// Zoom must finish before Google exports the local event's join URL.
	if zoom {
		if err := s.flushEvent(ctx, "zoom", id); err != nil {
			return nil, err
		}
	}
	if google {
		if err := s.flushEvent(ctx, "google", id); err != nil {
			return nil, err
		}
	}
	for _, p := range []string{"zoom", "google"} {
		if (p == "zoom" && !zoom) || (p == "google" && !google) {
			continue
		}
		pending, err := s.Store.HasCloudChange(ctx, p, id)
		if err != nil {
			return nil, err
		}
		if pending {
			return nil, &domain.AppError{Code: "integration_pending", Message: "Изменение ожидает отправки", Status: 409}
		}
	}
	return s.event(ctx, id)
}

func (s *Service) flushEvent(ctx context.Context, p string, id int64) error {
	for attempt := 0; attempt < 3; attempt++ {
		if err := s.Sync(ctx, p); err != nil {
			return err
		}
		pending, err := s.Store.HasCloudChange(ctx, p, id)
		if err != nil {
			return err
		}
		if !pending {
			return nil
		}
	}
	return &domain.AppError{Code: "integration_pending", Message: "Изменение ожидает отправки", Status: 409}
}
