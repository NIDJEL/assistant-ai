package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/user"
	"net/url"
	"time"
)

type Provider interface {
	AuthorizationURL(state, challenge string) string
	Identity(context.Context, string, string, string, string) (repository.Identity, error)
}

func (s *Service) StartOAuth(ctx context.Context, provider, link string, now time.Time) (redirect, browser string, err error) {
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
	err = s.Store.SaveOAuthState(ctx, repository.OAuthState{ID: s.digest(state), Provider: provider, BrowserHash: s.digest(browser), Verifier: verifier, LinkUserID: link, ExpiresAt: now.Add(10 * time.Minute)})
	challenge := sha256.Sum256([]byte(verifier))
	return p.AuthorizationURL(state, base64.RawURLEncoding.EncodeToString(challenge[:])), browser, err
}
func (s *Service) FinishOAuth(ctx context.Context, provider, browser, currentUser string, q url.Values, now time.Time) (*user.User, bool, error) {
	invalid := &domain.AppError{Code: "oauth_invalid", Message: "Не удалось завершить вход. Попробуйте ещё раз.", Status: 400}
	if len(q.Get("state")) != 43 || len(browser) != 43 {
		return nil, false, invalid
	}
	state, err := s.Store.ConsumeOAuthState(ctx, s.digest(q.Get("state")), provider, s.digest(browser), now)
	if err != nil {
		return nil, false, err
	}
	if state == nil {
		return nil, false, invalid
	}
	if q.Get("error") != "" {
		return nil, false, &domain.AppError{Code: "oauth_cancelled", Message: "Авторизация была отменена", Status: 400}
	}
	p := s.Providers[provider]
	if p == nil || q.Get("code") == "" || len(q.Get("code")) > 4096 {
		return nil, false, invalid
	}
	if state.LinkUserID != "" && state.LinkUserID != currentUser {
		return nil, false, invalid
	}
	identity, err := p.Identity(ctx, q.Get("code"), state.Verifier, q.Get("state"), q.Get("device_id"))
	if err != nil || identity.Subject == "" || identity.Provider != provider {
		return nil, false, invalid
	}
	identity.Email, _ = NormalizeEmail(identity.Email)
	if identity.Name == "" {
		identity.Name = "Пользователь Ритм"
	}
	u, err := s.Users.ResolveIdentity(ctx, identity, state.LinkUserID)
	if errors.Is(err, repository.ErrConflict) {
		return nil, false, &domain.AppError{Code: "oauth_account_exists", Message: "Войдите в существующий аккаунт и подключите этот способ входа в настройках.", Status: 409}
	}
	if err != nil {
		return nil, false, invalid
	}
	return u, state.LinkUserID != "", nil
}
