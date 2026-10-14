package repository

import (
	"context"
	"time"
)

// OAuth tokens stay on the server and are encrypted before persistence.
type OAuthToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
}

type OAuthGrant struct {
	Identity Identity
	Login    string
	Token    OAuthToken
}

type CalendarConnection struct {
	Provider      string     `json:"provider"`
	Status        string     `json:"status"`
	LastSync      *time.Time `json:"last_sync"`
	LastError     string     `json:"last_error"`
	PendingEvents int        `json:"pending_events"`
}

type CalendarCredentials struct {
	CalendarConnection
	Subject, Login, CalendarURL string
	Ciphertext                  []byte
	ExpiresAt                   *time.Time
}

type CalendarChange struct{ EventID, Version int64 }

type CalendarStore interface {
	Connections(context.Context) ([]CalendarConnection, error)
	CalendarCredentials(context.Context) (*CalendarCredentials, error)
	SaveCalendarCredentials(context.Context, CalendarCredentials) error
	SetCalendarTarget(context.Context, string) error
	CalendarChanges(context.Context, int) ([]CalendarChange, error)
	AcknowledgeCalendarChange(context.Context, CalendarChange) error
	CalendarSyncStatus(context.Context, string, string, *time.Time) error
	DisconnectCalendar(context.Context) error
	CalendarUsers(context.Context) ([]string, error)
	WithCalendarLock(context.Context, func() error) error
}
