// Package social provides user-scoped friendship and collaborative scheduling.
// It reuses the calendar repository transaction, including chat idempotency.
package social

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

type Executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
type Session struct {
	Q        Executor
	UserID   string
	Location *time.Location
}

func In(ctx context.Context, tx repository.Tx, loc *time.Location) (*Session, error) {
	t, ok := tx.(interface{ SocialExecutor() Executor })
	if !ok || repository.UserID(ctx) == "" {
		return nil, problem(503, "social_unavailable", "Совместное планирование недоступно")
	}
	return &Session{Q: t.SocialExecutor(), UserID: repository.UserID(ctx), Location: loc}, nil
}

type Person struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	AvatarURL string   `json:"avatar_url"`
	Email     string   `json:"email,omitempty"`
	Tags      []string `json:"tags"`
}
type Profile struct {
	Person
	Code       string `json:"code"`
	ShareEmail bool   `json:"share_email"`
	Timezone   string `json:"timezone"`
	StartHour  int    `json:"start_hour"`
	EndHour    int    `json:"end_hour"`
}
type FriendRequest struct {
	ID        string    `json:"id"`
	Person    Person    `json:"person"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}
type People struct {
	Profile  Profile         `json:"profile"`
	Friends  []Person        `json:"friends"`
	Incoming []FriendRequest `json:"incoming"`
	Outgoing []FriendRequest `json:"outgoing"`
}
type Slot struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}
type BusySlot struct {
	Slot
	EventID int64 `json:"event_id,omitempty"`
}
type Availability struct {
	Users    []AvailabilityUser `json:"users"`
	Slots    []Slot             `json:"slots"`
	Complete bool               `json:"complete"`
}
type AvailabilityUser struct {
	UserID   string `json:"user_id"`
	Status   string `json:"status"`
	Complete bool   `json:"complete"`
}
type Member struct {
	Person
	Status  string `json:"status"`
	Version int    `json:"version"`
}
type Proposal struct {
	ID         string `json:"id"`
	ProposerID string `json:"proposer_id"`
	Name       string `json:"name"`
	Version    int    `json:"version"`
	Slots      []Slot `json:"slots"`
	Status     string `json:"status"`
}
type Meeting struct {
	Event     domain.Event `json:"event"`
	Organizer Person       `json:"organizer"`
	Members   []Member     `json:"members"`
	Proposals []Proposal   `json:"proposals"`
	Version   int          `json:"version"`
	Cancelled bool         `json:"cancelled"`
	Timezone  string       `json:"timezone"`
}
type Operation struct {
	RequestID  string        `json:"request_id"`
	Action     string        `json:"action"`
	Code       string        `json:"code,omitempty"`
	ID         string        `json:"id,omitempty"`
	UserID     string        `json:"user_id,omitempty"`
	Tags       []string      `json:"tags,omitempty"`
	EventID    int64         `json:"event_id,omitempty"`
	Event      *domain.Event `json:"event,omitempty"`
	FriendIDs  []string      `json:"friend_ids,omitempty"`
	Status     string        `json:"status,omitempty"`
	Version    int           `json:"version,omitempty"`
	Slots      []Slot        `json:"slots,omitempty"`
	SlotIndex  int           `json:"slot_index,omitempty"`
	ShareEmail *bool         `json:"share_email,omitempty"`
	Timezone   string        `json:"timezone,omitempty"`
	StartHour  *int          `json:"start_hour,omitempty"`
	EndHour    *int          `json:"end_hour,omitempty"`
}
type Result struct {
	Message string `json:"message"`
	EventID int64  `json:"event_id,omitempty"`
	ID      string `json:"id,omitempty"`
}

func problem(status int, code, message string) error {
	return &domain.AppError{Status: status, Code: code, Message: message}
}
func notFound() error {
	return problem(404, "not_found", "Запись не найдена или доступ запрещён")
}
func jsonText(v any) string { b, _ := json.Marshal(v); return string(b) }
