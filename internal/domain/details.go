package domain

import (
	"errors"
	"strings"
	"unicode/utf8"
)

type Subtask struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

// Optional personal event metadata. Empty fields preserve the existing JSON contract.
type EventDetails struct {
	Tags            []string      `json:"tags,omitempty"`
	Subtasks        []Subtask     `json:"subtasks,omitempty"`
	Participants    []Participant `json:"participants,omitempty"`
	ReminderEnabled *bool         `json:"reminder_enabled,omitempty"`
}

func (d EventDetails) Validate() error {
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
