package repository

import (
	"context"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

// Tx exposes domain operations, not an unrestricted SQL executor to services.
type Tx interface {
	Count(context.Context) (int, error)
	Get(context.Context, int64) (*domain.Event, error)
	Events(context.Context, domain.Filter) ([]domain.Event, error)
	Create(context.Context, domain.Event, time.Time) (*domain.Event, error)
	Update(context.Context, domain.Event, time.Time) (*domain.Event, error)
	Delete(context.Context, int64) error
	State(context.Context, string) (domain.State, error)
	SaveState(context.Context, string, domain.State, time.Time) error
	Cached(context.Context, string, string) (*domain.Exchange, error)
	SaveExchange(context.Context, string, string, string, domain.Response, time.Time) error
	History(context.Context, string, int) ([]domain.Exchange, error)
}
type Store interface {
	Read(context.Context, func(Tx) error) error
	Write(context.Context, func(Tx) error) error
}
