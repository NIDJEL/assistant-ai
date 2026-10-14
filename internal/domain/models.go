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
	EventDetails
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
	DeletedAt          *time.Time    `json:"deleted_at,omitempty"`
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
	start := e.Date.At(*e.StartTime, loc).Add(time.Duration(e.StartSecond) * time.Second)
	if e.OccurrenceAt != nil {
		start = *e.OccurrenceAt
	}
	return start, start.Add(e.Duration), true
}
func (e Event) Validate() error {
	if e.Recurrence != nil {
		if err := e.Recurrence.Validate(true); err != nil {
			return err
		}
		if e.Kind == EventMeeting {
			return errors.New("Повторение поддерживается для задач и напоминаний")
		}
		if e.Date == nil || e.StartTime == nil {
			return errors.New("Для повторения нужны дата и время")
		}
	}
	if err := e.EventDetails.Validate(); err != nil {
		return err
	}
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
	ExactAt                *time.Time
	Recurrence             *Recurrence
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
	ReminderEnabled        *bool
	ReminderMinutes        *int
	StartSecond            int
	NotificationContext    bool
	Selector               *EventSelector
}

// EventSelector describes existing records. Command.Date/Time describe the
// destination of a move, so the two must never be used interchangeably.
type EventSelector struct {
	All        bool       `json:"all,omitempty"`
	ExactTitle bool       `json:"exact_title,omitempty"`
	Start      *Date      `json:"start,omitempty"`
	End        *Date      `json:"end,omitempty"`
	Time       *TimeOfDay `json:"time,omitempty"`
}

type commandJSON struct {
	ExactAt             *time.Time     `json:"exact_at,omitempty"`
	Recurrence          *Recurrence    `json:"recurrence,omitempty"`
	Action              Action         `json:"action"`
	Kind                EventType      `json:"kind"`
	Title               string         `json:"title"`
	Query               string         `json:"query"`
	Date                *Date          `json:"date"`
	Time                *TimeOfDay     `json:"time"`
	PeriodStart         *Date          `json:"period_start"`
	PeriodEnd           *Date          `json:"period_end"`
	DurationMinutes     int            `json:"duration_minutes"`
	TargetID            *int64         `json:"target_id"`
	Error               *string        `json:"error"`
	Provider            string         `json:"provider"`
	ReminderEnabled     *bool          `json:"reminder_enabled,omitempty"`
	ReminderMinutes     *int           `json:"reminder_minutes,omitempty"`
	StartSecond         int            `json:"start_second,omitempty"`
	NotificationContext bool           `json:"notification_context,omitempty"`
	Selector            *EventSelector `json:"selector,omitempty"`
}

func (c Command) MarshalJSON() ([]byte, error) {
	var errText *string
	if c.Error != "" {
		s := c.Error
		errText = &s
	}
	return json.Marshal(commandJSON{ExactAt: c.ExactAt, Recurrence: c.Recurrence, Action: c.Action, Kind: c.Kind, Title: c.Title, Query: c.Query, Date: c.Date, Time: c.Time, PeriodStart: c.PeriodStart, PeriodEnd: c.PeriodEnd, DurationMinutes: int(c.Duration / time.Minute), TargetID: c.TargetID, Error: errText, Provider: c.Provider, ReminderEnabled: c.ReminderEnabled, ReminderMinutes: c.ReminderMinutes, StartSecond: c.StartSecond, NotificationContext: c.NotificationContext, Selector: c.Selector})
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
	*c = Command{ExactAt: v.ExactAt, Recurrence: v.Recurrence, Action: v.Action, Kind: v.Kind, Title: v.Title, Query: v.Query, Date: v.Date, Time: v.Time, PeriodStart: v.PeriodStart, PeriodEnd: v.PeriodEnd, Duration: time.Duration(v.DurationMinutes) * time.Minute, TargetID: v.TargetID, Provider: v.Provider}
	c.ReminderEnabled, c.ReminderMinutes = v.ReminderEnabled, v.ReminderMinutes
	c.StartSecond = v.StartSecond
	c.NotificationContext = v.NotificationContext
	c.Selector = v.Selector
	if v.Error != nil {
		c.Error = *v.Error
	}
	return c.Validate()
}
func (c Command) Validate() error {
	if err := (EventDetails{ReminderEnabled: c.ReminderEnabled, ReminderMinutes: c.ReminderMinutes, StartSecond: c.StartSecond, Recurrence: c.Recurrence}).Validate(); err != nil {
		return err
	}
	if !c.Action.Interpretable() {
		return errors.New("invalid interpreter action")
	}
	if c.Duration < 5*time.Minute || c.Duration > 24*time.Hour {
		return errors.New("invalid duration")
	}
	if c.TargetID != nil && *c.TargetID < 1 {
		return errors.New("invalid target ID")
	}
	if c.Selector != nil {
		if c.Selector.Start != nil && c.Selector.End != nil && !c.Selector.Start.Before(c.Selector.End.Time) {
			return errors.New("invalid selection period")
		}
		if t := c.Selector.Time; t != nil && (t.Hour < 0 || t.Hour > 23 || t.Minute < 0 || t.Minute > 59) {
			return errors.New("invalid selection time")
		}
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
	Reply       string   `json:"reply"`
	Action      Action   `json:"action"`
	Events      []Event  `json:"events"`
	Candidates  []Event  `json:"candidates"`
	Conflicts   []Event  `json:"conflicts"`
	Total       *int     `json:"total,omitempty"`
	SessionID   string   `json:"session_id"`
	RequestID   string   `json:"request_id"`
	Provider    string   `json:"provider"`
	Timezone    string   `json:"timezone"`
	Pending     bool     `json:"pending"`
	Suggestions []string `json:"suggestions,omitempty"`
}
type Exchange struct {
	Message   string    `json:"message"`
	Response  Response  `json:"response"`
	CreatedAt time.Time `json:"created_at"`
}
type Filter struct {
	Deleted              bool
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
