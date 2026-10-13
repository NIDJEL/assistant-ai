package domain

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

type Event struct {
	ID                 int64         `json:"id"`
	Kind               EventType     `json:"kind"`
	Title              string        `json:"title"`
	Description        string        `json:"description"`
	Location           string        `json:"location"`
	MeetingURL         string        `json:"meeting_url"`
	Date               *Date         `json:"date"`
	StartTime          *TimeOfDay    `json:"time"`
	Duration           time.Duration `json:"-"`
	Status             Status        `json:"status"`
	Priority           Priority      `json:"priority"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
	SourceKey          *string       `json:"source_key"`
	ExternalCalendarID *string       `json:"external_calendar_id"`
	Score              *float64      `json:"score,omitempty"`
}
type Task struct{ Event }
type Reminder struct{ Event }
type Participant struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}
type Meeting struct {
	Event
	Participants            []Participant
	Location, URL, Provider string
}

func (e Event) Interval(loc *time.Location) (time.Time, time.Time, bool) {
	if e.Date == nil || e.StartTime == nil {
		return time.Time{}, time.Time{}, false
	}
	start := e.Date.At(*e.StartTime, loc)
	return start, start.Add(e.Duration), true
}
func (e Event) Validate() error {
	if utf8.RuneCountInString(e.Location) > 500 || len(e.MeetingURL) > 2048 {
		return errors.New("Сократите место встречи или ссылку")
	}
	if e.MeetingURL != "" {
		u, err := url.Parse(e.MeetingURL)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
			return errors.New("Ссылка встречи должна начинаться с http:// или https://")
		}
	}
	if e.Kind < EventTask || e.Kind > EventReminder {
		return errors.New("Некорректный тип записи")
	}
	if strings.TrimSpace(e.Title) == "" || utf8.RuneCountInString(e.Title) > 500 {
		return errors.New("Название должно содержать от 1 до 500 символов")
	}
	if e.Duration < 5*time.Minute || e.Duration > 24*time.Hour {
		return errors.New("Длительность должна быть от 5 минут до 24 часов")
	}
	if e.Status != StatusOpen && e.Status != StatusDone {
		return errors.New("Некорректный статус")
	}
	if e.Priority < PriorityNormal || e.Priority > PriorityHigh {
		return errors.New("Некорректный приоритет")
	}
	if (e.Kind == EventMeeting || e.Kind == EventReminder) && (e.Date == nil || e.StartTime == nil) {
		return errors.New("Для встречи и напоминания нужны дата и время")
	}
	if e.StartTime != nil && e.Date == nil {
		return errors.New("Для времени нужна дата")
	}
	if e.StartTime != nil && (e.StartTime.Hour < 0 || e.StartTime.Hour > 23 || e.StartTime.Minute < 0 || e.StartTime.Minute > 59) {
		return errors.New("Некорректное время")
	}
	return nil
}
func (e Event) MarshalJSON() ([]byte, error) {
	type base Event
	return json.Marshal(struct {
		base
		DurationMinutes int `json:"duration_minutes"`
	}{base(e), int(e.Duration / time.Minute)})
}
func (e *Event) UnmarshalJSON(b []byte) error {
	type base Event
	v := struct {
		base
		DurationMinutes int `json:"duration_minutes"`
	}{DurationMinutes: 60}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*e = Event(v.base)
	e.Duration = time.Duration(v.DurationMinutes) * time.Minute
	if e.Priority == PriorityUnknown {
		e.Priority = PriorityNormal
	}
	return nil
}

type Command struct {
	Action                 Action
	Kind                   EventType
	Title, Query           string
	Date                   *Date
	Time                   *TimeOfDay
	PeriodStart, PeriodEnd *Date
	Duration               time.Duration
	TargetID               *int64
	Error, Provider        string
	Confidence             float64
}
type commandJSON struct {
	Action          Action     `json:"action"`
	Kind            EventType  `json:"kind"`
	Title           string     `json:"title"`
	Query           string     `json:"query"`
	Date            *Date      `json:"date"`
	Time            *TimeOfDay `json:"time"`
	PeriodStart     *Date      `json:"period_start"`
	PeriodEnd       *Date      `json:"period_end"`
	DurationMinutes int        `json:"duration_minutes"`
	TargetID        *int64     `json:"target_id"`
	Error           *string    `json:"error"`
	Provider        string     `json:"provider"`
}

func (c Command) MarshalJSON() ([]byte, error) {
	var errText *string
	if c.Error != "" {
		s := c.Error
		errText = &s
	}
	return json.Marshal(commandJSON{c.Action, c.Kind, c.Title, c.Query, c.Date, c.Time, c.PeriodStart, c.PeriodEnd, int(c.Duration / time.Minute), c.TargetID, errText, c.Provider})
}
func (c *Command) UnmarshalJSON(b []byte) error {
	v := commandJSON{Action: ActionHelp, DurationMinutes: 60, Provider: "rules"}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if err := d.Decode(&v); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("unexpected trailing JSON")
	}
	if v.DurationMinutes < 5 || v.DurationMinutes > 1440 {
		return errors.New("invalid duration")
	}
	*c = Command{Action: v.Action, Kind: v.Kind, Title: v.Title, Query: v.Query, Date: v.Date, Time: v.Time, PeriodStart: v.PeriodStart, PeriodEnd: v.PeriodEnd, Duration: time.Duration(v.DurationMinutes) * time.Minute, TargetID: v.TargetID, Provider: v.Provider}
	if v.Error != nil {
		c.Error = *v.Error
	}
	return c.Validate()
}
func (c Command) Validate() error {
	if !c.Action.Interpretable() {
		return errors.New("invalid interpreter action")
	}
	if c.Duration < 5*time.Minute || c.Duration > 24*time.Hour {
		return errors.New("invalid duration")
	}
	if c.TargetID != nil && *c.TargetID < 1 {
		return errors.New("invalid target ID")
	}
	if c.PeriodStart != nil && c.PeriodEnd != nil && !c.PeriodStart.Before(c.PeriodEnd.Time) {
		return errors.New("invalid period")
	}
	if utf8.RuneCountInString(c.Title) > 500 || utf8.RuneCountInString(c.Query) > 2000 {
		return errors.New("command too long")
	}
	return nil
}

type Pending struct {
	Command     Command   `json:"command"`
	Expectation string    `json:"expectation"`
	At          time.Time `json:"at"`
	Candidates  []Event   `json:"candidates,omitempty"`
}
type State struct {
	Pending *Pending `json:"pending,omitempty"`
	LastIDs []int64  `json:"last_ids,omitempty"`
}

func (s State) MarshalJSON() ([]byte, error) {
	value := map[string]any{}
	if s.Pending != nil {
		value["pending"] = s.Pending
	}
	if s.LastIDs != nil {
		value["last_ids"] = s.LastIDs
	}
	return json.Marshal(value)
}

type Response struct {
	Reply      string  `json:"reply"`
	Action     Action  `json:"action"`
	Events     []Event `json:"events"`
	Candidates []Event `json:"candidates"`
	Conflicts  []Event `json:"conflicts"`
	Total      *int    `json:"total,omitempty"`
	SessionID  string  `json:"session_id"`
	RequestID  string  `json:"request_id"`
	Provider   string  `json:"provider"`
	Timezone   string  `json:"timezone"`
	Pending    bool    `json:"pending"`
}
type Exchange struct {
	Message   string    `json:"message"`
	Response  Response  `json:"response"`
	CreatedAt time.Time `json:"created_at"`
}
type Filter struct {
	Kind                 EventType
	Status               Status
	Start, End, Upcoming *Date
}
type AppError struct {
	Code, Message string
	Status        int
	Conflicts     []Event
}

func (e *AppError) Error() string { return e.Message }
func Validation(message string) error {
	return &AppError{Code: "validation_error", Message: message, Status: 422}
}
