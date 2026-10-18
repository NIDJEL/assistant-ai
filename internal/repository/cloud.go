package repository

import (
	"context"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"time"
)

// Credentials are server-only. Public status uses CalendarConnection instead.
type CloudConnection struct {
	Provider, Subject, Login, Status, CalendarID, SyncToken string
	Ciphertext                                              []byte
	ExpiresAt                                               *time.Time
}
type CloudLink struct {
	EventID                 int64
	RemoteID, ETag, Updated string
}
type CloudStore interface {
	Store
	CloudConnection(context.Context, string) (*CloudConnection, error)
	SaveCloudConnection(context.Context, CloudConnection) error
	CloudConnections(context.Context) ([]CalendarConnection, error)
	CloudUsers(context.Context) ([][2]string, error)
	WithCloudLock(context.Context, string, func() error) error
	CloudStatus(context.Context, string, string, string, bool) error
	DisconnectCloud(context.Context, string) error
	SelectCloudCalendar(context.Context, string) error
	CloudChanges(context.Context, string) ([]CalendarChange, error)
	AckCloudChange(context.Context, string, CalendarChange) error
	HasCloudChange(context.Context, string, int64) (bool, error)
	CloudLink(context.Context, string, string, int64) (*CloudLink, error)
	SaveCloudLink(context.Context, string, string, CloudLink) error
	SetCloudSyncToken(context.Context, string) error
	ImportGoogleEvent(context.Context, string, CloudLink, *domain.Event, bool) error
}
