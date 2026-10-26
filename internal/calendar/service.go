package calendar

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/search"
)

type Service struct {
	Store    repository.Store
	Search   *search.Engine
	Location *time.Location
	DemoPath string
}
type List struct {
	Search *domain.SearchStatus `json:"search,omitempty"`
	Events []domain.Event       `json:"events"`
	Total  int                  `json:"total"`
	Offset int                  `json:"offset"`
	Limit  int                  `json:"limit"`
}
type ImportResult struct {
	Imported  int `json:"imported"`
	Skipped   int `json:"skipped"`
	TotalRows int `json:"total_rows"`
}

func (s *Service) Conflicts(ctx context.Context, tx repository.Tx, e domain.Event, exclude int64) ([]domain.Event, error) {
	result := []domain.Event{}
	start, _, ok := e.Interval(s.Location)
	if e.Kind != domain.EventMeeting || !ok || e.Status == domain.StatusDone {
		return result, nil
	}
	slots, err := e.MeetingSlots(s.Location)
	if err != nil {
		return nil, domain.Validation(err.Error())
	}
	if len(slots) == 0 {
		return result, nil
	}
	end := slots[len(slots)-1].End
	meetings, err := tx.Events(ctx, domain.Filter{Kind: domain.EventMeeting, Status: domain.StatusOpen})
	if err != nil {
		return nil, err
	}
	for _, other := range meetings {
		if other.ID == exclude {
			continue
		}
		if e.SeriesID != nil && *e.SeriesID == other.ID && e.RecurrenceID != nil && other.Recurrence != nil {
			rule, err := other.Recurrence.Excluding(*e.RecurrenceID)
			if err != nil {
				return nil, domain.Validation(err.Error())
			}
			other.Recurrence = rule
		}
		otherSlots, err := other.Slots(start, end, s.Location)
		if err != nil {
			return nil, domain.Validation(err.Error())
		}
		if domain.SlotsOverlap(slots, otherSlots) {
			result = append(result, other)
		}
	}
	return result, nil
}
func (s *Service) List(ctx context.Context, f domain.Filter, query string, limit, offset int) (List, error) {
	result := List{Events: []domain.Event{}, Limit: limit, Offset: offset}
	if f.Start != nil && f.End != nil && !f.End.After(f.Start.Time) {
		return result, domain.Validation("end должен быть позже start (правая граница не включается)")
	}
	var events []domain.Event
	err := s.Store.Read(ctx, func(tx repository.Tx) error { var err error; events, err = tx.Events(ctx, f); return err })
	if err != nil {
		return result, err
	}
	if query != "" {
		ctx = s.Search.Prepare(ctx, events, query)
		events, result.Search = s.Search.SearchContext(ctx, events, query, len(events))
	}
	result.Total = len(events)
	if offset < len(events) {
		end := offset + limit
		if end > len(events) {
			end = len(events)
		}
		result.Events = events[offset:end]
	}
	return result, nil
}
func (s *Service) Get(ctx context.Context, id int64) (*domain.Event, error) {
	var e *domain.Event
	err := s.Store.Read(ctx, func(tx repository.Tx) error { var err error; e, err = tx.Get(ctx, id); return err })
	if err == nil && e == nil {
		err = &domain.AppError{Code: "not_found", Message: "Запись не найдена", Status: 404}
	}
	return e, err
}
func (s *Service) Create(ctx context.Context, e domain.Event, now time.Time, force bool) (*domain.Event, error) {
	if e.Status == domain.StatusUnknown {
		e.Status = domain.StatusOpen
	}
	if e.Priority == domain.PriorityUnknown {
		e.Priority = domain.PriorityNormal
	}
	if e.Duration == 0 {
		e.Duration = time.Hour
	}
	if e.Recurrence == nil {
		e.OccurrenceAt = nil
	}
	if e.Recurrence != nil {
		if e.StartTime == nil {
			e.StartTime = e.Recurrence.LocalTime
		}
		if err := Rebase(&e, now, s.Location); err != nil {
			return nil, domain.Validation(err.Error())
		}
	}
	if err := e.Validate(); err != nil {
		return nil, domain.Validation(err.Error())
	}
	var result *domain.Event
	err := s.Store.Write(ctx, func(tx repository.Tx) error {
		conflicts, err := s.Conflicts(ctx, tx, e, 0)
		if err != nil {
			return err
		}
		if len(conflicts) > 0 && !force {
			return &domain.AppError{Code: "meeting_conflict", Message: "Время пересекается с другой встречей", Status: 409, Conflicts: conflicts}
		}
		result, err = tx.Create(ctx, e, now)
		return err
	})
	return result, err
}

type Patch struct {
	OccurrenceAt         *time.Time
	Recurrence           **domain.Recurrence
	Tags                 *[]string
	Subtasks             *[]domain.Subtask
	Participants         *[]domain.Participant
	ReminderEnabled      *bool
	ReminderMinutes      *int
	Title, Description   *string
	Location, MeetingURL *string
	Date                 **domain.Date
	Time                 **domain.TimeOfDay
	Duration             *time.Duration
	Status               *domain.Status
	Priority             *domain.Priority
	Force                bool
}

func (s *Service) Update(ctx context.Context, id int64, p Patch, now time.Time) (*domain.Event, error) {
	var result *domain.Event
	err := s.Store.Write(ctx, func(tx repository.Tx) error {
		e, err := tx.Get(ctx, id)
		if err != nil {
			return err
		}
		if e == nil {
			return &domain.AppError{Code: "not_found", Message: "Запись не найдена", Status: 404}
		}
		old := *e
		var parent *domain.Event
		if p.OccurrenceAt != nil {
			if p.Recurrence != nil {
				return domain.Validation("Для одного повторения нельзя менять правило серии")
			}
			master, child, err := s.PrepareOccurrence(ctx, tx, *e, nil, p.OccurrenceAt)
			if err != nil {
				return domain.Validation(err.Error())
			}
			parent, e = &master, &child
			old = child
		}
		if p.Recurrence != nil {
			e.Recurrence = *p.Recurrence
			if e.Recurrence != nil && p.Time == nil && e.Recurrence.LocalTime != nil {
				e.StartTime = e.Recurrence.LocalTime
			}
		}
		if p.Tags != nil {
			e.Tags = *p.Tags
		}
		if p.Subtasks != nil {
			e.Subtasks = *p.Subtasks
		}
		if p.Participants != nil {
			e.Participants = *p.Participants
		}
		if p.ReminderEnabled != nil {
			e.ReminderEnabled = p.ReminderEnabled
		}
		if p.ReminderMinutes != nil {
			e.ReminderMinutes = p.ReminderMinutes
		}
		if p.Title != nil {
			e.Title = *p.Title
		}
		if p.Description != nil {
			e.Description = *p.Description
		}
		if p.Location != nil {
			e.Location = *p.Location
		}
		if p.MeetingURL != nil {
			e.MeetingURL = *p.MeetingURL
		}
		if p.Date != nil {
			e.Date = *p.Date
			if !reflect.DeepEqual(old.Date, e.Date) && p.Recurrence == nil {
				if err := MoveSeriesDate(e); err != nil {
					return domain.Validation(err.Error())
				}
			}
		}
		if p.Time != nil {
			e.StartTime = *p.Time
			if e.Recurrence == nil || !reflect.DeepEqual(old.StartTime, e.StartTime) {
				e.StartSecond = 0
			}
		}
		if p.Duration != nil {
			e.Duration = *p.Duration
		}
		if p.Status != nil {
			if *p.Status == domain.StatusDone {
				e.LastNotificationAt = nil
			}
			if e.Status == domain.StatusDone && *p.Status == domain.StatusOpen {
				if err := Resume(e, now); err != nil {
					return err
				}
			}
			e.Status = *p.Status
		}
		if p.Priority != nil {
			e.Priority = *p.Priority
		}
		if e.Recurrence != nil && (p.Recurrence != nil && !reflect.DeepEqual(old.Recurrence, e.Recurrence) || p.Date != nil && !reflect.DeepEqual(old.Date, e.Date) || p.Time != nil && !reflect.DeepEqual(old.StartTime, e.StartTime)) {
			if err := Rebase(e, now, s.Location); err != nil {
				return domain.Validation(err.Error())
			}
		}
		if e.Recurrence == nil {
			if p.Recurrence != nil || p.Date != nil || p.Time != nil {
				e.OccurrenceAt = nil
			}
			e.LastNotificationAt = nil
		}
		if err = e.Validate(); err != nil {
			return domain.Validation(err.Error())
		}
		if e.Status == domain.StatusOpen && (p.Date != nil || p.Time != nil || p.Duration != nil || p.Status != nil || p.Recurrence != nil) {
			conflicts, err := s.Conflicts(ctx, tx, *e, e.ID)
			if err != nil {
				return err
			}
			if len(conflicts) > 0 && !p.Force {
				return &domain.AppError{Code: "meeting_conflict", Message: "Время пересекается с другой встречей", Status: 409, Conflicts: conflicts}
			}
		}
		if parent != nil {
			if _, err = tx.Update(ctx, *parent, now); err != nil {
				return err
			}
		}
		if e.ID == 0 {
			result, err = tx.Create(ctx, *e, now)
		} else {
			result, err = tx.Update(ctx, *e, now)
		}
		return err
	})
	return result, err
}
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.Store.Write(ctx, func(tx repository.Tx) error {
		e, err := tx.Get(ctx, id)
		if err != nil {
			return err
		}
		if e == nil {
			return &domain.AppError{Code: "not_found", Message: "Запись не найдена", Status: 404}
		}
		return tx.Delete(ctx, id)
	})
}
func (s *Service) Restore(ctx context.Context, id int64, now time.Time, force bool) (*domain.Event, error) {
	var result *domain.Event
	err := s.Store.Write(ctx, func(tx repository.Tx) error {
		e, err := tx.GetDeleted(ctx, id)
		if err != nil {
			return err
		}
		if e == nil {
			return &domain.AppError{Code: "not_found", Message: "Удалённая запись не найдена", Status: 404}
		}
		if e.Status == domain.StatusOpen {
			conflicts, err := s.Conflicts(ctx, tx, *e, id)
			if err != nil {
				return err
			}
			if len(conflicts) > 0 && !force {
				return &domain.AppError{Code: "meeting_conflict", Message: "Время пересекается с другой встречей", Status: 409, Conflicts: conflicts}
			}
		}
		result, err = tx.Restore(ctx, id, now)
		if err == nil && !force {
			conflicts, e := s.RestoredChildrenConflicts(ctx, tx, id)
			if e != nil {
				return e
			}
			if len(conflicts) > 0 {
				return &domain.AppError{Code: "meeting_conflict", Message: "Отдельное повторение серии пересекается с другой встречей", Status: 409, Conflicts: conflicts}
			}
		}
		if err == nil && result != nil && result.Status == domain.StatusOpen && result.Recurrence != nil {
			if err = Resume(result, now); err != nil {
				return err
			}
			result, err = tx.Update(ctx, *result, now)
		}
		return err
	})
	return result, err
}

// Due reads persisted notifications in the PostgreSQL runtime. Only the server
// worker schedules/advances them. The legacy SQLite processor below is retained
// for reference/parity consumers; it is never used by the production server.
func (s *Service) Due(ctx context.Context, now time.Time) ([]domain.Event, error) {
	if reader, ok := s.Store.(repository.NotificationReader); ok {
		return reader.DueNotifications(ctx, now)
	}
	result := []domain.Event{}
	err := s.Store.Write(ctx, func(tx repository.Tx) error {
		events, err := tx.Events(ctx, domain.Filter{Status: domain.StatusOpen})
		if err != nil {
			return err
		}
		for _, e := range events {
			if err := ctx.Err(); err != nil {
				return err
			}
			start, _, ok := e.Interval(s.Location)
			if !ok {
				continue
			}
			lead := time.Duration(e.ReminderLead()) * time.Minute
			age := now.Sub(start.Add(-lead))
			if e.Recurrence == nil {
				if e.HasReminder() && age >= 0 && age <= 5*time.Minute {
					result = append(result, e)
				}
				continue
			}
			if age >= 0 {
				if e.HasReminder() && age <= 5*time.Minute {
					stamp := start.UTC()
					e.LastNotificationAt = &stamp
					e.LastNotificationLead = e.ReminderLead()
				}
				next, err := e.Recurrence.Next(now.Add(lead))
				if errors.Is(err, domain.ErrRecurrenceExhausted) {
					e.Status = domain.StatusDone
					if _, err = tx.Update(ctx, e, now); err != nil {
						return err
					}
					if e.HasReminder() && age <= 5*time.Minute {
						result = append(result, e)
					}
					continue
				}
				if err != nil {
					return err
				}
				e.SetOccurrence(next)
				if _, err = tx.Update(ctx, e, now); err != nil {
					return err
				}
			}
			if e.HasReminder() && e.LastNotificationAt != nil {
				age = now.Sub(e.LastNotificationAt.Add(-time.Duration(e.LastNotificationLead) * time.Minute))
				if age >= 0 && age <= 5*time.Minute {
					snapshot := e
					snapshot.SetOccurrence(*e.LastNotificationAt)
					n := e.LastNotificationLead
					snapshot.ReminderMinutes = &n
					result = append(result, snapshot)
				}
			}
		}
		return nil
	})
	return result, err
}
func (s *Service) Count(ctx context.Context) (int, error) {
	var count int
	err := s.Store.Read(ctx, func(tx repository.Tx) error { var err error; count, err = tx.Count(ctx); return err })
	return count, err
}
func (s *Service) Import(ctx context.Context, input io.Reader, filename string, now time.Time) (ImportResult, error) {
	result := ImportResult{}
	reader := csv.NewReader(input)
	header, err := reader.Read()
	if err != nil {
		return result, domain.Validation("CSV должен содержать id,date,time,text")
	}
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "\ufeff")
	}
	cols := map[string]int{}
	for i, h := range header {
		h = strings.TrimSpace(h)
		if _, exists := cols[h]; exists {
			return result, domain.Validation("CSV содержит повторяющиеся названия столбцов")
		}
		cols[h] = i
	}
	for _, name := range []string{"id", "date", "time", "text"} {
		if _, ok := cols[name]; !ok {
			return result, domain.Validation("CSV должен содержать id,date,time,text")
		}
	}
	prepared := []domain.Event{}
	seen := map[int64]bool{}
	line := 1
	for {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		row, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		line++
		if readErr != nil {
			return result, domain.Validation(fmt.Sprintf("Ошибка CSV в строке %d: %v", line, readErr))
		}
		line, _ = reader.FieldPos(0)
		if len(prepared) >= CSVMaxRows {
			return result, domain.Validation("В CSV должно быть не больше 10000 записей")
		}
		id, idErr := strconv.ParseInt(row[cols["id"]], 10, 64)
		day, dayErr := domain.ParseDate(row[cols["date"]])
		clock, clockErr := domain.ParseTime(row[cols["time"]])
		if idErr != nil || id < 1 || seen[id] || dayErr != nil || clockErr != nil {
			return result, domain.Validation(fmt.Sprintf("Ошибка CSV в строке %d: некорректные id, дата или время", line))
		}
		seen[id] = true
		title := strings.TrimSpace(row[cols["text"]])
		if !utf8.ValidString(title) {
			return result, domain.Validation(fmt.Sprintf("Ошибка CSV в строке %d: используйте кодировку UTF-8", line))
		}
		kind := domain.EventTask
		firstWord := ""
		if fields := strings.Fields(title); len(fields) > 0 {
			firstWord = strings.ToLower(fields[0])
		}
		switch firstWord {
		case "созвон", "встреча", "демо", "совещание", "интервью":
			kind = domain.EventMeeting
		}
		if strings.HasPrefix(strings.ToLower(title), "напомнить") {
			kind = domain.EventReminder
		}
		key := fmt.Sprintf("csv:%s:%d", filename, id)
		e := domain.Event{Kind: kind, Title: title, Date: day, StartTime: clock, Duration: time.Hour, Status: domain.StatusOpen, Priority: domain.PriorityNormal, SourceKey: &key}
		if err = e.Validate(); err != nil {
			return result, domain.Validation(fmt.Sprintf("Ошибка CSV в строке %d: %v", line, err))
		}
		prepared = append(prepared, e)
	}
	if len(prepared) == 0 {
		return result, domain.Validation("CSV не содержит записей: добавьте хотя бы одну строку после заголовка")
	}
	result.TotalRows = len(prepared)
	err = s.Store.Write(ctx, func(tx repository.Tx) error {
		for _, e := range prepared {
			if err := ctx.Err(); err != nil {
				return err
			}
			created, err := tx.Create(ctx, e, now)
			if err != nil {
				return err
			}
			if created == nil {
				result.Skipped++
			} else {
				result.Imported++
			}
		}
		return nil
	})
	return result, err
}
