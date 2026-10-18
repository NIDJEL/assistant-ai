package repository

import (
	"context"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"time"
)

type ZoomEvent struct {
	Event, MeetingID, RunID, HostID string
	OccurredAt                      time.Time
	StartedAt                       *time.Time
}

// ZoomReader is optional for repositories without a Zoom integration (SQLite).
type ZoomReader interface {
	ZoomState(context.Context, int64, string) (runID, status string, err error)
}
type ZoomStore interface {
	ReceiveZoomEvent(context.Context, ZoomEvent, *time.Location, time.Time) error
	ChatNotifications(context.Context) ([]domain.Exchange, error)
	ReadChatNotifications(context.Context, []string, time.Time) error
}
