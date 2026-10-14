package dto

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/google/uuid"
)

type Chat struct {
	Message   string `json:"message"`
	SessionID string `json:"session_id"`
	RequestID string `json:"request_id"`
}

func (c *Chat) Validate() error {
	c.Message = strings.TrimSpace(c.Message)
	if c.Message == "" || utf8.RuneCountInString(c.Message) > 2000 {
		return domain.Validation("Сообщение должно содержать от 1 до 2000 символов")
	}
	for _, p := range []*string{&c.SessionID, &c.RequestID} {
		if *p == "" {
			*p = uuid.NewString()
		} else {
			v, err := uuid.Parse(*p)
			if err != nil {
				return domain.Validation("Некорректный UUID")
			}
			*p = v.String()
		}
	}
	return nil
}

type Event struct {
	domain.EventDetails
	Kind            domain.EventType  `json:"kind"`
	Title           string            `json:"title"`
	Description     string            `json:"description"`
	Location        string            `json:"location"`
	MeetingURL      string            `json:"meeting_url"`
	Date            *domain.Date      `json:"date"`
	Time            *domain.TimeOfDay `json:"time"`
	DurationMinutes *int              `json:"duration_minutes"`
	Status          domain.Status     `json:"status"`
	Priority        domain.Priority   `json:"priority"`
	Force           bool              `json:"force"`
}

func (d Event) Domain() (domain.Event, error) {
	duration := time.Hour
	if d.DurationMinutes != nil {
		if *d.DurationMinutes < 5 || *d.DurationMinutes > 1440 {
			return domain.Event{}, domain.Validation("Длительность должна быть от 5 минут до 24 часов")
		}
		duration = time.Duration(*d.DurationMinutes) * time.Minute
	}
	return domain.Event{EventDetails: d.EventDetails, Kind: d.Kind, Title: d.Title, Description: d.Description, Location: d.Location, MeetingURL: d.MeetingURL, Date: d.Date, StartTime: d.Time, Duration: duration, Status: d.Status, Priority: d.Priority}, nil
}

type Patch struct {
	Tags            *[]string             `json:"tags"`
	Subtasks        *[]domain.Subtask     `json:"subtasks"`
	Participants    *[]domain.Participant `json:"participants"`
	ReminderEnabled *bool                 `json:"reminder_enabled"`
	Title           *string               `json:"title"`
	Description     *string               `json:"description"`
	Location        *string               `json:"location"`
	MeetingURL      *string               `json:"meeting_url"`
	Date            json.RawMessage       `json:"date"`
	Time            json.RawMessage       `json:"time"`
	DurationMinutes *int                  `json:"duration_minutes"`
	Status          *domain.Status        `json:"status"`
	Priority        *domain.Priority      `json:"priority"`
	Force           bool                  `json:"force"`
}

func (p Patch) Domain() (calendar.Patch, error) {
	result := calendar.Patch{Tags: p.Tags, Subtasks: p.Subtasks, Participants: p.Participants, ReminderEnabled: p.ReminderEnabled, Title: p.Title, Description: p.Description, Location: p.Location, MeetingURL: p.MeetingURL, Status: p.Status, Priority: p.Priority, Force: p.Force}
	if p.DurationMinutes != nil {
		if *p.DurationMinutes < 5 || *p.DurationMinutes > 1440 {
			return result, fmt.Errorf("Длительность должна быть от 5 минут до 24 часов")
		}
		duration := time.Duration(*p.DurationMinutes) * time.Minute
		result.Duration = &duration
	}
	if len(p.Date) > 0 {
		var d *domain.Date
		if err := json.Unmarshal(p.Date, &d); err != nil {
			return result, fmt.Errorf("invalid date: %w", err)
		}
		result.Date = &d
	}
	if len(p.Time) > 0 {
		var t *domain.TimeOfDay
		if err := json.Unmarshal(p.Time, &t); err != nil {
			return result, fmt.Errorf("invalid time: %w", err)
		}
		result.Time = &t
	}
	if result.Tags == nil && result.Subtasks == nil && result.Participants == nil && result.ReminderEnabled == nil && result.Title == nil && result.Description == nil && result.Location == nil && result.MeetingURL == nil && result.Date == nil && result.Time == nil && result.Duration == nil && result.Status == nil && result.Priority == nil {
		return result, fmt.Errorf("Не указаны поля для изменения")
	}
	return result, nil
}
