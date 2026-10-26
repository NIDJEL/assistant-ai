package assistant

import (
	"context"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

func (s *Service) refreshExecutionStatus(ctx context.Context, responses []*domain.Response) {
	if s.ReadExecutionStatus == nil {
		return
	}
	ids := []int64{}
	seen := map[int64]bool{}
	for i := len(responses) - 1; i >= 0; i-- {
		for _, e := range responses[i].Events {
			if !seen[e.ID] && len(ids) < 100 {
				seen[e.ID] = true
				ids = append(ids, e.ID)
			}
		}
	}
	statuses, err := s.ReadExecutionStatus(ctx, ids, time.Now().In(s.Calendar.Location))
	if err != nil {
		return
	}
	for _, response := range responses {
		response.Execution = nil
		for _, status := range statuses {
			for _, e := range response.Events {
				if e.ID == status.EventID {
					entry := status
					for _, step := range response.Operations {
						for _, id := range step.EventIDs {
							if id == e.ID {
								entry.StepKeys = append(entry.StepKeys, step.Key)
							}
						}
					}
					response.Execution = append(response.Execution, entry)
					break
				}
			}
		}
	}
}
