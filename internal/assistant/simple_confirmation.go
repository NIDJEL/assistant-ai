package assistant

import (
	"context"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

func (s *Service) confirmSimple(ctx context.Context, tx repository.Tx, state *domain.State, now time.Time) (domain.Response, error) {
	p := state.Pending
	refuse := func() (domain.Response, error) {
		state.Pending = nil
		return answer("Записи или пересечения изменились. Повторите запрос для нового предпросмотра. Ничего не изменено.", domain.ActionClarify), nil
	}
	switch p.Expectation {
	case "batch":
		return s.confirmBatch(ctx, tx, state, now)
	case "delete", "delete_many":
		return s.confirmDeletion(ctx, tx, state, p, now)
	case "conflict":
		c := p.Command
		var proposed domain.Event
		if c.Action == domain.ActionCreate {
			proposed = domain.Event{EventDetails: domain.EventDetails{CalendarProvider: c.CalendarProvider, ConferenceProvider: c.ConferenceProvider, ReminderEnabled: c.ReminderEnabled, ReminderMinutes: c.ReminderMinutes, StartSecond: c.StartSecond, OccurrenceAt: c.ExactAt}, Kind: c.Kind, Title: c.Title, Date: c.Date, StartTime: c.Time, Duration: c.Duration, Status: domain.StatusOpen, Priority: domain.PriorityNormal}
		} else if c.Action == domain.ActionUpdate && c.TargetID != nil && p.TargetSnapshot != nil {
			current, err := tx.Get(ctx, *c.TargetID)
			if err != nil {
				return domain.Response{}, err
			}
			if current == nil || !sameRecord(*p.TargetSnapshot, *current) {
				return refuse()
			}
			proposed, err = s.proposedBatchEvent(*current, c, now)
			if err != nil {
				return refuse()
			}
		} else {
			return refuse()
		}
		if err := proposed.Validate(); err != nil {
			return refuse()
		}
		conflicts, err := s.Calendar.Conflicts(ctx, tx, proposed, proposed.ID)
		if err != nil {
			return domain.Response{}, err
		}
		if len(conflicts) != len(p.Candidates) {
			return refuse()
		}
		for _, old := range p.Candidates {
			found := false
			for _, current := range conflicts {
				if sameRecord(old, current) {
					found = true
					break
				}
			}
			if !found {
				return refuse()
			}
		}
		state.Pending = nil
		return s.executeValidated(ctx, tx, state, c, now, true)
	default:
		return answer("Сначала ответьте на уточнение.", domain.ActionClarify), nil
	}
}
