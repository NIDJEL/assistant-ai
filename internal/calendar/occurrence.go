package calendar

import (
	"context"
	"errors"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

// PrepareOccurrence is read-only. The caller validates the edited child before
// saving parent and child together in its existing write transaction.
func (s *Service) PrepareOccurrence(ctx context.Context, tx repository.Tx, master domain.Event, date *domain.Date, exact *time.Time) (domain.Event, domain.Event, error) {
	bad := errors.New("Выберите личную активную серию и исходную дату её повторения")
	if master.SeriesID != nil {
		if date == nil && exact == nil {
			exact = master.RecurrenceID
		}
		parent, err := tx.Get(ctx, *master.SeriesID)
		if err != nil {
			return master, master, err
		}
		if parent == nil {
			return master, master, bad
		}
		master = *parent
	}
	if master.Recurrence == nil || master.Collaborative {
		return master, master, bad
	}
	at, _, ok := master.Interval(s.Location)
	if !ok {
		return master, master, bad
	}
	if exact != nil {
		at = *exact
	} else if date != nil {
		if master.Recurrence.Elapsed() {
			return master, master, errors.New("Для часовой или минутной серии укажите исходную дату вместе со временем")
		}
		zone, _ := time.LoadLocation(master.Recurrence.Timezone)
		clock := master.Recurrence.LocalTime
		at = domain.LocalInstant(date.Year(), date.Month(), date.Day(), clock.Hour, clock.Minute, 0, zone)
	}
	if !master.Recurrence.Contains(at) {
		return master, master, errors.New("Указанная дата не является повторением этой серии")
	}
	children, err := tx.Events(ctx, domain.Filter{})
	if err != nil {
		return master, master, err
	}
	for _, child := range children {
		if child.SeriesID != nil && *child.SeriesID == master.ID && child.RecurrenceID != nil && child.RecurrenceID.Equal(at) {
			return master, child, nil
		}
	}
	if master.Status != domain.StatusOpen || master.Recurrence.Excluded(at) {
		return master, master, bad
	}
	child := master
	id, original := master.ID, at.UTC()
	child.SeriesID, child.RecurrenceID = &id, &original
	child.ID, child.SourceKey, child.ExternalCalendarID = 0, nil, nil
	child.SetOccurrence(at)
	child.LastNotificationAt, child.Recurrence = nil, nil
	master.Recurrence, err = master.Recurrence.Excluding(at)
	if err != nil {
		return master, child, err
	}
	current, _, _ := master.Interval(s.Location)
	if current.Equal(at) {
		next, nextErr := master.Recurrence.Next(at)
		if errors.Is(nextErr, domain.ErrRecurrenceExhausted) {
			master.Status = domain.StatusDone
		} else if nextErr != nil {
			return master, child, nextErr
		} else {
			master.SetOccurrence(next)
		}
		master.LastNotificationAt = nil
	}
	return master, child, nil
}

// Called inside the restoring write transaction so a conflicting detached
// occurrence rolls back restoration of both the parent and every child.
func (s *Service) RestoredChildrenConflicts(ctx context.Context, tx repository.Tx, id int64) ([]domain.Event, error) {
	children, err := tx.Events(ctx, domain.Filter{Status: domain.StatusOpen})
	if err != nil {
		return nil, err
	}
	conflicts := []domain.Event{}
	seen := map[int64]bool{}
	for _, child := range children {
		if child.SeriesID == nil || *child.SeriesID != id {
			continue
		}
		items, err := s.Conflicts(ctx, tx, child, child.ID)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if !seen[item.ID] {
				seen[item.ID] = true
				conflicts = append(conflicts, item)
			}
		}
	}
	return conflicts, nil
}
