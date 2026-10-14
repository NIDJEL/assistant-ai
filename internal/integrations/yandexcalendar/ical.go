package yandexcalendar

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

func escapeText(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	return strings.NewReplacer("\\", "\\\\", "\n", "\\n", ";", "\\;", ",", "\\,").Replace(s)
}
func folded(s string) string {
	var out strings.Builder
	limit := 75
	for len(s) > limit {
		n := limit
		for n > 0 && !utf8.RuneStart(s[n]) {
			n--
		}
		out.WriteString(s[:n])
		out.WriteString("\r\n ")
		s = s[n:]
		limit = 74
	}
	out.WriteString(s)
	out.WriteString("\r\n")
	return out.String()
}
func eventName(userID string, id int64) string     { return fmt.Sprintf("rhythm-%s-%d", userID, id) }
func eventUID(userID string, id int64) string      { return eventName(userID, id) + "@rhythm" }
func eventResource(userID string, id int64) string { return eventUID(userID, id) + ".ics" }

func eventCalendar(e domain.Event, userID string, loc *time.Location) string {
	lines := []string{"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//Rhythm//Yandex Calendar Sync//RU", "CALSCALE:GREGORIAN", "BEGIN:VEVENT", "UID:" + eventUID(userID, e.ID), "DTSTAMP:" + e.UpdatedAt.UTC().Format("20060102T150405Z"), "CREATED:" + e.CreatedAt.UTC().Format("20060102T150405Z"), "LAST-MODIFIED:" + e.UpdatedAt.UTC().Format("20060102T150405Z")}
	if e.StartTime == nil {
		lines = append(lines, "DTSTART;VALUE=DATE:"+e.Date.Format("20060102"), "DTEND;VALUE=DATE:"+e.Date.AddDate(0, 0, 1).Format("20060102"))
	} else {
		start, end, _ := e.Interval(loc)
		lines = append(lines, "DTSTART:"+start.UTC().Format("20060102T150405Z"), "DTEND:"+end.UTC().Format("20060102T150405Z"))
	}
	title := e.Title
	if e.Status == domain.StatusDone {
		title = "✓ " + title
		lines = append(lines, "TRANSP:TRANSPARENT")
	} else {
		lines = append(lines, "TRANSP:OPAQUE")
	}
	lines = append(lines, "SUMMARY:"+escapeText(title), "STATUS:CONFIRMED")
	description := e.Description
	for _, sub := range e.Subtasks {
		mark := "☐"
		if sub.Done {
			mark = "☑"
		}
		description += "\n" + mark + " " + sub.Title
	}
	for _, participant := range e.Participants {
		description += "\nУчастник: " + participant.Name
		if participant.Email != "" {
			description += " (" + participant.Email + ")"
		}
	}
	if e.MeetingURL != "" {
		description += "\n" + e.MeetingURL
	}
	lines = append(lines, "DESCRIPTION:"+escapeText(strings.TrimSpace(description)))
	if e.Location != "" {
		lines = append(lines, "LOCATION:"+escapeText(e.Location))
	}
	if len(e.Tags) > 0 {
		tags := []string{}
		for _, tag := range e.Tags {
			tags = append(tags, escapeText(tag))
		}
		lines = append(lines, "CATEGORIES:"+strings.Join(tags, ","))
	}
	priority := "5"
	if e.Priority == domain.PriorityHigh {
		priority = "1"
	}
	if e.Priority == domain.PriorityLow {
		priority = "9"
	}
	lines = append(lines, "PRIORITY:"+priority)
	if e.Status != domain.StatusDone && e.StartTime != nil && e.HasReminder() {
		trigger := "TRIGGER:PT0S"
		if minutes := e.ReminderLead(); minutes > 0 {
			trigger = fmt.Sprintf("TRIGGER:-PT%dM", minutes)
		}
		lines = append(lines, "BEGIN:VALARM", "ACTION:DISPLAY", trigger, "DESCRIPTION:"+escapeText(e.Title), "END:VALARM")
	}
	lines = append(lines, "END:VEVENT", "END:VCALENDAR")
	var result strings.Builder
	for _, line := range lines {
		result.WriteString(folded(line))
	}
	return result.String()
}
