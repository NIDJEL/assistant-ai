package calendar

import (
	"context"
	"sort"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

// ListOccurrences is a read-only projection. Virtual rows retain the master's
// ID and carry a separate original timestamp for explicitly scoped edits.
func (s *Service) ListOccurrences(ctx context.Context, f domain.Filter, query string, limit, offset int) (List, error) {
	result := List{Events: []domain.Event{}, Limit: limit, Offset: offset}
	if f.Start == nil || f.End == nil || !f.End.After(f.Start.Time) || f.End.Sub(f.Start.Time) > 93*24*time.Hour {
		return result, domain.Validation("Для календаря укажите период от 1 до 93 дней")
	}
	from, to := f.Start.At(domain.TimeOfDay{}, s.Location), f.End.At(domain.TimeOfDay{}, s.Location)
	start, end := f.Start, f.End
	f.Start, f.End = nil, nil
	var records []domain.Event
	err := s.Store.Read(ctx, func(tx repository.Tx) error { var err error; records, err = tx.Events(ctx, f); return err })
	if err != nil {
		return result, err
	}
	masters := []domain.Event{}
	expanded := map[int64][]domain.Event{}
	for _, record := range records {
		if record.Recurrence == nil {
			if record.Date != nil && !record.Date.Before(start.Time) && record.Date.Before(end.Time) {
				masters = append(masters, record)
				expanded[record.ID] = []domain.Event{record}
			}
			continue
		}
		if record.Kind != domain.EventMeeting && record.Recurrence.Elapsed() {
			for day := start; day.Before(end.Time); day = day.AddDays(1) {
				at, count, err := record.Recurrence.ElapsedRange(day.At(domain.TimeOfDay{}, s.Location), day.AddDays(1).At(domain.TimeOfDay{}, s.Location))
				if err != nil {
					return result, domain.Validation(err.Error())
				}
				if count == 0 {
					continue
				}
				copy := record
				copy.SetOccurrence(at)
				stamp := at.UTC()
				copy.RecurrenceID = &stamp
				copy.VirtualOccurrence = true
				copy.OccurrencesInDay = count
				expanded[record.ID] = append(expanded[record.ID], copy)
			}
			if len(expanded[record.ID]) > 0 {
				masters = append(masters, record)
			}
			continue
		}
		times, err := record.Recurrence.Occurrences(from, to, domain.MaxSeriesOccurrences)
		if err != nil {
			return result, domain.Validation(err.Error())
		}
		if len(times) == 0 {
			continue
		}
		masters = append(masters, record)
		for _, at := range times {
			copy := record
			copy.SetOccurrence(at)
			stamp := at.UTC()
			copy.RecurrenceID = &stamp
			copy.VirtualOccurrence = true
			expanded[record.ID] = append(expanded[record.ID], copy)
		}
	}
	if query != "" {
		ctx = s.Search.Prepare(ctx, masters, query)
		masters, result.Search = s.Search.SearchContext(ctx, masters, query, len(masters))
	}
	rows := []domain.Event{}
	for _, master := range masters {
		rows = append(rows, expanded[master.ID]...)
		if len(rows) > 10000 {
			return result, domain.Validation("Слишком много повторений: сократите период календаря")
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if !a.Date.Equal(b.Date.Time) {
			return a.Date.Before(b.Date.Time)
		}
		if a.StartTime != nil && b.StartTime != nil && a.StartTime.String() != b.StartTime.String() {
			return a.StartTime.String() < b.StartTime.String()
		}
		return a.ID < b.ID
	})
	result.Total = len(rows)
	if offset < len(rows) {
		result.Events = rows[offset:min(offset+limit, len(rows))]
	}
	return result, nil
}
