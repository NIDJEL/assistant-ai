package assistant

import (
	"context"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

func (s *Service) zoomCards(ctx context.Context, tx repository.Tx, r *domain.Response) error {
	prior := map[int64]string{}
	for _, c := range r.ZoomMeetings {
		prior[c.EventID] = c.RunID
	}
	r.ZoomMeetings = nil
	for _, snapshot := range r.Events {
		if snapshot.ConferenceProvider != "zoom" {
			continue
		}
		e, err := tx.Get(ctx, snapshot.ID)
		if err != nil {
			return err
		}
		if e == nil {
			continue
		}
		card := domain.ZoomCard(*e, s.Calendar.Location)
		if card == nil {
			continue
		}
		if reader, ok := tx.(repository.ZoomReader); ok {
			run, status, err := reader.ZoomState(ctx, e.ID, prior[e.ID])
			if err != nil {
				return err
			}
			if status != "" {
				card.RunID, card.Status = run, status
			}
		}
		r.ZoomMeetings = append(r.ZoomMeetings, *card)
	}
	return nil
}

// Refresh cards from owned records on history load, including conferences whose
// join URL arrived after the original chat transaction finished.
func (s *Service) RefreshZoomCards(ctx context.Context, r *domain.Response) error {
	return s.Store.Read(ctx, func(tx repository.Tx) error { return s.zoomCards(ctx, tx, r) })
}
