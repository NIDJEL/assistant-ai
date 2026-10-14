package repository

import "context"

type userKey struct{}

// WithUser sets the authenticated tenant. Only authentication middleware and
// explicit administrative imports may supply this value, never request bodies.
func WithUser(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, userKey{}, id)
}
func UserID(ctx context.Context) string { id, _ := ctx.Value(userKey{}).(string); return id }
