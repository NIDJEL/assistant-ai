package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/user"
)

type Provider interface {
	AuthorizationURL(state, challenge string) string
	Identity(context.Context, string, string, string, string) (repository.Identity, error)
}
type CalendarGrantProvider interface {
	CalendarAuthorizationURL(state, challenge string) string
	Grant(context.Context, string, string, string, string) (repository.OAuthGrant, error)
}
type CalendarConnector interface {
	Connect(context.Context, repository.OAuthGrant, time.Time) error
}
type OAuthResult struct {
	User             *user.User
	Linked, Calendar bool
}

func (s *Service) StartOAuth(ctx context.Context, provider, link string, now time.Time) (string, string, error) {
	return s.startOAuth(ctx, provider, link, "", now)
}
func (s *Service) StartCalendarOAuth(ctx context.Context, userID string, now time.Time) (string, string, error) {
	if userID == "" || s.Calendar == nil {
		return "", "", &domain.AppError{Code: "calendar_unavailable", Message: "Подключение календаря не настроено", Status: 503}
	}
	return s.startOAuth(ctx, "yandex", userID, "calendar", now)
}
func (s *Service) startOAuth(ctx context.Context, provider, link, purpose string, now time.Time) (redirect, browser string, err error) {
	p := s.Providers[provider]
	if p == nil {
		return "", "", &domain.AppError{Code: "oauth_unavailable", Message: "Этот способ входа ещё не настроен. Используйте почту.", Status: 503}
	}
	state, err := randomToken()
	if err != nil {
		return "", "", err
	}
	browser, err = randomToken()
	if err != nil {
		return "", "", err
	}
	verifier, err := randomToken()
	if err != nil {
		return "", "", err
	}
	challenge := sha256.Sum256([]byte(verifier))
	encoded := base64.RawURLEncoding.EncodeToString(challenge[:])
	if purpose == "calendar" {
		cp, ok := p.(CalendarGrantProvider)
		if !ok {
			return "", "", &domain.AppError{Code: "calendar_unavailable", Message: "Подключение календаря не настроено", Status: 503}
		}
		redirect = cp.CalendarAuthorizationURL(state, encoded)
	} else {
		redirect = p.AuthorizationURL(state, encoded)
	}
	err = s.Store.SaveOAuthState(ctx, repository.OAuthState{ID: s.digest(state), Provider: provider, BrowserHash: s.digest(browser), Verifier: verifier, LinkUserID: link, Purpose: purpose, ExpiresAt: now.Add(10 * time.Minute)})
	return redirect, browser, err
}
func (s *Service) FinishOAuth(ctx context.Context, provider, browser, currentUser string, q url.Values, now time.Time) (*user.User, bool, error) {
	result, err := s.FinishOAuthFlow(ctx, provider, browser, currentUser, q, now)
	return result.User, result.Linked, err
}
func (s *Service) FinishOAuthFlow(ctx context.Context, provider, browser, currentUser string, q url.Values, now time.Time) (result OAuthResult, err error) {
	invalid := &domain.AppError{Code: "oauth_invalid", Message: "Не удалось завершить вход. Попробуйте ещё раз.", Status: 400}
	if len(q.Get("state")) != 43 || len(browser) != 43 {
		return result, invalid
	}
	state, err := s.Store.ConsumeOAuthState(ctx, s.digest(q.Get("state")), provider, s.digest(browser), now)
	if err != nil {
		return result, err
	}
	if state == nil {
		return result, invalid
	}
	result.Calendar = state.Purpose == "calendar"
	if q.Get("error") != "" {
		return result, &domain.AppError{Code: "oauth_cancelled", Message: "Авторизация была отменена", Status: 400}
	}
	p := s.Providers[provider]
	if p == nil || q.Get("code") == "" || len(q.Get("code")) > 4096 {
		return result, invalid
	}
	if state.LinkUserID != "" && state.LinkUserID != currentUser {
		return result, invalid
	}
	var identity repository.Identity
	var grant repository.OAuthGrant
	if result.Calendar {
		cp, ok := p.(CalendarGrantProvider)
		if !ok || provider != "yandex" || state.LinkUserID == "" || s.Calendar == nil {
			return result, invalid
		}
		grant, err = cp.Grant(ctx, q.Get("code"), state.Verifier, q.Get("state"), q.Get("device_id"))
		identity = grant.Identity
	} else {
		identity, err = p.Identity(ctx, q.Get("code"), state.Verifier, q.Get("state"), q.Get("device_id"))
	}
	if err != nil || identity.Subject == "" || identity.Provider != provider {
		return result, invalid
	}
	identity.Email, _ = NormalizeEmail(identity.Email)
	if identity.Name == "" {
		identity.Name = "Пользователь Ритм"
	}
	u, err := s.Users.ResolveIdentity(ctx, identity, state.LinkUserID)
	if errors.Is(err, repository.ErrConflict) {
		return result, &domain.AppError{Code: "oauth_account_exists", Message: "Войдите в существующий аккаунт и подключите этот способ входа в настройках.", Status: 409}
	}
	if err != nil {
		return result, invalid
	}
	result.User, result.Linked = u, state.LinkUserID != ""
	if result.Calendar {
		err = s.Calendar.Connect(repository.WithUser(ctx, u.ID), grant, now)
		if err != nil {
			return result, err
		}
	}
	return result, nil
}
