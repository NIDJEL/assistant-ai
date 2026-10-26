package assistant

import (
	"errors"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
)

// A date selecting one occurrence belongs to the occurrence selector, not to
// the stored master row's date. Keep all non-date target constraints intact.
func normalizeOccurrenceTarget(a *nlu.Update, now time.Time) error {
	if a.Changes.Scope != "occurrence" || a.Target.Filter == nil || a.Target.Filter.Date == nil {
		return nil
	}
	date := a.Target.Filter.Date
	if a.Changes.Occurrence != nil {
		day, _, exact, err := date.Resolve(now)
		other, _, otherExact, otherErr := a.Changes.Occurrence.Resolve(now)
		if err != nil || otherErr != nil || day == nil || other == nil || day.String() != other.String() || exact != nil && otherExact != nil && !exact.Equal(*otherExact) {
			return errors.New("Укажите одну исходную дату повторения: в запросе указаны разные даты")
		}
	} else {
		a.Changes.Occurrence = date
	}
	filter := *a.Target.Filter
	filter.Date = nil
	a.Target.Filter = &filter
	return nil
}

// A master and its detached occurrences are one series, not several possible
// matches for a request naming that series. Distinct series still need a choice.
func occurrenceCandidates(events []domain.Event) []domain.Event {
	masters := map[int64]bool{}
	for _, e := range events {
		if e.Recurrence != nil {
			masters[e.ID] = true
		}
	}
	result := make([]domain.Event, 0, len(events))
	for _, e := range events {
		if e.SeriesID != nil && masters[*e.SeriesID] {
			continue
		}
		result = append(result, e)
	}
	return result
}
