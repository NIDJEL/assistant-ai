package assistant

import (
	"context"
	"errors"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

func simpleReadFilter(a *nlu.Read, state domain.State, now time.Time) (*domain.ExecutionFilter, error) {
	f, err := simpleFilter(a.Filter, now)
	if err != nil || !a.Inherit {
		return f, err
	}
	if !freshResult(state, now) || state.Dialogue.Search == nil {
		return nil, errors.New("Сначала задайте поиск, который нужно уточнить.")
	}
	q := state.Dialogue.Search
	base := domain.ExecutionFilter{Kind: q.Kind, Query: q.Query, Start: q.Start, End: q.End}
	if q.Filter != nil {
		base = *q.Filter
	}
	if a.Kind != "" {
		base.Kind = f.Kind
	}
	if a.Conference != "" {
		base.Conference = f.Conference
	}
	if a.Query != "" {
		base.Query = f.Query
	}
	if a.Date != nil || a.Period != nil {
		base.Start, base.End = f.Start, f.End
	}
	if a.Status != nil {
		base.Status = f.Status
	}
	return &base, nil
}

func (s *Service) prepareSearches(ctx context.Context, op nlu.Operation, state domain.State, now time.Time) (context.Context, error) {
	if s.Calendar.Search == nil || !s.Calendar.Search.HybridEnabled() {
		return ctx, nil
	}
	filters := []*domain.ExecutionFilter{}
	var collect func(nlu.Operation)
	collect = func(o nlu.Operation) {
		args, err := o.Args()
		if err != nil {
			return
		}
		switch a := args.(type) {
		case *nlu.Read:
			f, err := simpleReadFilter(a, state, now)
			if err == nil && f.Query != "" {
				filters = append(filters, f)
			}
		case *nlu.MultiAction:
			for _, step := range a.Steps {
				collect(step.Operation)
			}
		case *nlu.ResumeAction:
			collect(a.Operation)
		}
	}
	collect(op)
	if len(filters) == 0 {
		return ctx, nil
	}
	var events []domain.Event
	err := s.Store.Read(ctx, func(tx repository.Tx) error { var err error; events, err = tx.Events(ctx, domain.Filter{}); return err })
	if err != nil {
		return ctx, err
	}
	for _, f := range filters {
		query := f.Query
		copy := *f
		copy.Query = ""
		ctx = s.Calendar.Search.Prepare(ctx, s.filterEvents(events, &copy), query)
	}
	return ctx, nil
}
