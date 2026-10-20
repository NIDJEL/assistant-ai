package user

import (
	"context"
	"github.com/NIDJEL/assistant-ai/internal/legal"
)

type Identity struct{ Provider, Subject, Email, Name, AvatarURL string }
type Store interface {
	CreateEmailUser(context.Context, User, string, ...legal.Acceptance) error
	OAuthUser(context.Context, Identity, string, ...legal.Acceptance) (*User, error)
	UpdateUser(context.Context, string, string, *string, bool) (*User, error)
}
type Service struct{ Store Store }

func (s *Service) CreateEmail(ctx context.Context, u User, hash string, accepted ...legal.Acceptance) error {
	return s.Store.CreateEmailUser(ctx, u, hash, accepted...)
}
func (s *Service) ResolveIdentity(ctx context.Context, i Identity, link string, accepted ...legal.Acceptance) (*User, error) {
	return s.Store.OAuthUser(ctx, i, link, accepted...)
}
func (s *Service) Update(ctx context.Context, id, name string, avatar *string, complete bool) (*User, error) {
	return s.Store.UpdateUser(ctx, id, name, avatar, complete)
}
