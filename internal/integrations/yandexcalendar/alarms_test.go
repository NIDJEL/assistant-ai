package yandexcalendar

import (
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

func TestEarlyAlarmsKeepEventStartAndSupportDisable(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Moscow")
	day, _ := domain.ParseDate("2026-10-15")
	clock, _ := domain.ParseTime("12:50")
	minutes, enabled := 50, true
	e := domain.Event{EventDetails: domain.EventDetails{ReminderMinutes: &minutes, ReminderEnabled: &enabled}, ID: 42, Kind: domain.EventMeeting, Title: "Пара", Date: day, StartTime: clock, Duration: time.Hour, Status: domain.StatusOpen}
	raw := eventCalendar(e, "user", loc)
	if !strings.Contains(raw, "DTSTART:20261015T095000Z") || !strings.Contains(raw, "TRIGGER:-PT50M") || !strings.Contains(raw, "ACTION:DISPLAY") {
		t.Fatal(raw)
	}
	minutes = 60
	if !strings.Contains(eventCalendar(e, "user", loc), "TRIGGER:-PT60M") {
		t.Fatal("alarm not updated")
	}
	enabled = false
	if strings.Contains(eventCalendar(e, "user", loc), "BEGIN:VALARM") {
		t.Fatal("disabled alarm exported")
	}
	enabled = true
	e.Status = domain.StatusDone
	if strings.Contains(eventCalendar(e, "user", loc), "BEGIN:VALARM") {
		t.Fatal("completed event still notifies")
	}
}
