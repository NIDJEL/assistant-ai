package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/legal"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/user"
	"github.com/google/uuid"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"
)

const SessionTTL = 30 * 24 * time.Hour

type Service struct {
	Store         repository.AuthStore
	Users         *user.Service
	Secret        []byte
	Providers     map[string]Provider
	Calendar      CalendarConnector
	Integrations  CalendarConnector
	passwordSlots chan struct{}
	dummyHash     string
}

func New(store repository.AuthStore, secret string, providers map[string]Provider) (*Service, error) {
	if len(secret) < 32 {
		return nil, errors.New("SESSION_SECRET must contain at least 32 characters")
	}
	dummy, err := HashPassword("random-unusable-dummy-password")
	if err != nil {
		return nil, err
	}
	return &Service{Store: store, Users: &user.Service{Store: store}, Secret: []byte(secret), Providers: providers, passwordSlots: make(chan struct{}, 4), dummyHash: dummy}, nil
}
func randomToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b), err
}
func (s *Service) digest(value string) string {
	h := hmac.New(sha256.New, s.Secret)
	h.Write([]byte(value))
	return hex.EncodeToString(h.Sum(nil))
}
func NormalizeEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value || len(value) > 254 || !strings.Contains(value, ".") {
		return "", domain.Validation("Введите корректную почту")
	}
	return value, nil
}
func (s *Service) passwordWork(ctx context.Context, fn func() error) error {
	select {
	case s.passwordSlots <- struct{}{}:
		defer func() { <-s.passwordSlots }()
		return fn()
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *Service) Register(ctx context.Context, name, email, password, confirmation string, accepted ...legal.Acceptance) (*user.User, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	name, err = normalizeName(name)
	if err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(password) < 10 || len(password) > 1024 {
		return nil, domain.Validation("Пароль должен содержать от 10 символов")
	}
	if password != confirmation {
		return nil, domain.Validation("Пароли не совпадают")
	}
	var hash string
	err = s.passwordWork(ctx, func() error { var e error; hash, e = HashPassword(password); return e })
	if err != nil {
		return nil, err
	}
	u := &user.User{ID: uuid.NewString(), Email: email, Name: name, Providers: []string{"email"}}
	if err = s.Users.CreateEmail(ctx, *u, hash, accepted...); errors.Is(err, repository.ErrConflict) {
		return nil, &domain.AppError{Code: "email_in_use", Message: "Этот email уже используется", Status: 409}
	}
	return u, err
}
func (s *Service) Login(ctx context.Context, email, password string) (*user.User, error) {
	generic := &domain.AppError{Code: "invalid_credentials", Message: "Неверная почта или пароль", Status: 401}
	normalized, _ := NormalizeEmail(email)
	u, hash, err := s.Store.EmailIdentity(ctx, normalized)
	if err != nil {
		return nil, err
	}
	if hash == "" {
		hash = s.dummyHash
	}
	if len(password) > 1024 {
		return nil, generic
	}
	valid := false
	if err = s.passwordWork(ctx, func() error { valid = VerifyPassword(password, hash); return nil }); err != nil {
		return nil, err
	}
	if u == nil || !valid {
		return nil, generic
	}
	return u, nil
}

// EnableEmailLogin adds a password to the current OAuth account once. It never
// changes an existing password or accepts a different email/account identifier.
func (s *Service) EnableEmailLogin(ctx context.Context, uid, password, confirmation string) (*user.User, error) {
	if utf8.RuneCountInString(password) < 10 || len(password) > 1024 {
		return nil, domain.Validation("Пароль должен содержать от 10 символов")
	}
	if password != confirmation {
		return nil, domain.Validation("Пароли не совпадают")
	}
	var hash string
	if err := s.passwordWork(ctx, func() error { var err error; hash, err = HashPassword(password); return err }); err != nil {
		return nil, err
	}
	u, err := s.Store.EnableEmailLogin(ctx, uid, hash)
	if errors.Is(err, repository.ErrConflict) {
		return nil, &domain.AppError{Code: "email_login_unavailable", Message: "Вход по почте уже настроен или недоступен для этого аккаунта", Status: 409}
	}
	return u, err
}
func (s *Service) Session(ctx context.Context, token string, now time.Time) (*user.User, error) {
	if len(token) != 43 {
		return nil, nil
	}
	return s.Store.SessionUser(ctx, s.digest(token), now)
}
func (s *Service) NewSession(ctx context.Context, uid string, now time.Time) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	err = s.Store.CreateSession(ctx, s.digest(token), uid, now, now.Add(SessionTTL))
	return token, err
}
func (s *Service) Logout(ctx context.Context, token string) error {
	return s.Store.RevokeSession(ctx, s.digest(token))
}
func (s *Service) Update(ctx context.Context, uid, name string, complete bool) (*user.User, error) {
	if name != "" {
		var err error
		name, err = normalizeName(name)
		if err != nil {
			return nil, err
		}
	}
	return s.Users.Update(ctx, uid, name, nil, complete)
}
