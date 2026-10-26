package repository

import (
	"context"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

// NotificationReader is deliberately read-only: browser polling never schedules.
type NotificationReader interface {
	DueNotifications(context.Context, time.Time) ([]domain.Event, error)
}

type ExecutionStatusReader interface {
	ExecutionStatuses(context.Context, []int64, time.Time) ([]domain.ExecutionStatus, error)
}
