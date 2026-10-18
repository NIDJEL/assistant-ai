package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

type Subtask struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

// Optional personal event metadata. Empty fields preserve the existing JSON contract.
type EventDetails struct {
	GoogleAllDayDays      int           `json:"google_all_day_days,omitempty"`
	GoogleDurationSeconds int64         `json:"google_duration_seconds,omitempty"`
	CalendarProvider      string        `json:"calendar_provider,omitempty"`
	ConferenceProvider    string        `json:"conference_provider,omitempty"`
	Recurrence            *Recurrence   `json:"recurrence,omitempty"`
	OccurrenceAt          *time.Time    `json:"occurrence_at,omitempty"`
	LastNotificationAt    *time.Time    `json:"last_notification_at,omitempty"`
	LastNotificationLead  int           `json:"last_notification_lead,omitempty"`
	Tags                  []string      `json:"tags,omitempty"`
	Subtasks              []Subtask     `json:"subtasks,omitempty"`
	Participants          []Participant `json:"participants,omitempty"`
	ReminderEnabled       *bool         `json:"reminder_enabled,omitempty"`
	ReminderMinutes       *int          `json:"reminder_minutes,omitempty"`
	// Sub-minute precision for relative NLU commands, stored in existing metadata.
	StartSecond int `json:"start_second,omitempty"`
}

func (d EventDetails) Validate() error {
	if d.GoogleAllDayDays < 0 || d.GoogleAllDayDays > 36600 || d.GoogleDurationSeconds < 0 || d.GoogleDurationSeconds > 315360000 {
		return errors.New("Некорректная длительность внешнего события")
	}
	if d.CalendarProvider != "" && d.CalendarProvider != "google" {
		return errors.New("Некорректный календарь")
	}
	if d.ConferenceProvider != "" && d.ConferenceProvider != "zoom" {
		return errors.New("Некорректный сервис конференций")
	}
	if d.Recurrence != nil {
		if err := d.Recurrence.Validate(false); err != nil {
			return err
		}
	}
	if d.StartSecond < 0 || d.StartSecond > 59 {
		return errors.New("Секунды должны быть от 0 до 59")
	}
	if d.ReminderMinutes != nil && (*d.ReminderMinutes < 0 || *d.ReminderMinutes > 10080) {
		return errors.New("Напоминание можно поставить от 0 минут до 7 дней до события")
	}
	if len(d.Tags) > 12 || len(d.Subtasks) > 50 || len(d.Participants) > 50 {
		return errors.New("Слишком много меток, подзадач или участников")
	}
	for _, tag := range d.Tags {
		if strings.TrimSpace(tag) == "" || utf8.RuneCountInString(tag) > 50 {
			return errors.New("Метка должна содержать от 1 до 50 символов")
		}
	}
	ids := map[string]bool{}
	for _, task := range d.Subtasks {
		if task.ID == "" || len(task.ID) > 80 || ids[task.ID] || strings.TrimSpace(task.Title) == "" || utf8.RuneCountInString(task.Title) > 500 {
			return errors.New("Некорректная подзадача")
		}
		ids[task.ID] = true
	}
	for _, person := range d.Participants {
		if strings.TrimSpace(person.Name) == "" || utf8.RuneCountInString(person.Name) > 100 || len(person.Email) > 254 {
			return errors.New("Некорректный участник")
		}
	}
	return nil
}

// Legacy reminders without an explicit offset still fire at the event time.
func (e Event) HasReminder() bool {
	if e.ReminderEnabled != nil {
		return *e.ReminderEnabled
	}
	return e.Kind == EventReminder || e.ReminderMinutes != nil
}

func (e Event) ReminderLead() int {
	if e.ReminderMinutes != nil {
		return *e.ReminderMinutes
	}
	return 0
}
