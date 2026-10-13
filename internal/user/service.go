package user

import "context"

type Identity struct{ Provider, Subject, Email, Name, AvatarURL string }
type Store interface {
	CreateEmailUser(context.Context, User, string) error
	OAuthUser(context.Context, Identity, string) (*User, error)
	UpdateUser(context.Context, string, string, bool) (*User, error)
}
type Service struct{ Store Store }

func (s *Service) CreateEmail(ctx context.Context, u User, hash string) error {
	return s.Store.CreateEmailUser(ctx, u, hash)
}
func (s *Service) ResolveIdentity(ctx context.Context, i Identity, link string) (*User, error) {
	return s.Store.OAuthUser(ctx, i, link)
}
func (s *Service) Update(ctx context.Context, id, name string, complete bool) (*User, error) {
	return s.Store.UpdateUser(ctx, id, name, complete)
}
