package repository

import (
	"context"
	"errors"
	"github.com/NIDJEL/assistant-ai/internal/user"
	"time"
)

var ErrConflict = errors.New("identity conflict")

type Identity = user.Identity
type OAuthState struct {
	ID, Provider, BrowserHash, Verifier, LinkUserID string
	Purpose                                         string
	ExpiresAt                                       time.Time
}
type AuthStore interface {
	CreateEmailUser(context.Context, user.User, string) error
	EmailIdentity(context.Context, string) (*user.User, string, error)
	OAuthUser(context.Context, Identity, string) (*user.User, error)
	CreateSession(context.Context, string, string, time.Time, time.Time) error
	SessionUser(context.Context, string, time.Time) (*user.User, error)
	RevokeSession(context.Context, string) error
	SaveOAuthState(context.Context, OAuthState) error
	ConsumeOAuthState(context.Context, string, string, string, time.Time) (*OAuthState, error)
	UpdateUser(context.Context, string, string, *string, bool) (*user.User, error)
}
