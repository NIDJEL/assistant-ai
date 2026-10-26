package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

var errDryRun = errors.New("discard preflight effects")

func (s *Service) preflightSimple(ctx context.Context, snapshot domain.State, op nlu.Operation, message string, now time.Time) (*nlu.ValidationIssue, error) {
	if err := nlu.ValidateSources(op, message); err != nil {
		var issue nlu.ValidationIssue
		if errors.As(err, &issue) {
			return &issue, nil
		}
		return nil, err
	}
	var issue *nlu.ValidationIssue
	err := s.Store.Write(ctx, func(tx repository.Tx) error {
		checkpoint, ok := tx.(repository.Checkpoint)
		if !ok {
			return nil
		}
		returnErr := checkpoint.WithinSavepoint(ctx, func() error {
			raw, e := json.Marshal(snapshot)
			if e != nil {
				return e
			}
			var local domain.State
			if e = json.Unmarshal(raw, &local); e != nil {
				return e
			}
			_, r, e := s.applySimpleActions(ctx, tx, &local, op, message, now)
			if e != nil {
				var app *domain.AppError
				if errors.As(e, &app) && app.Status < 500 {
					issue = &nlu.ValidationIssue{Code: "business_validation", Field: "arguments"}
				} else {
					return e
				}
			}
			// Missing fields and choices are valid dialogue outcomes. Per-step
			// failures remain repairable without regenerating successful siblings.
			if e == nil && r.Action == domain.ActionClarify && local.Pending == nil && local.Actions == nil {
				issue = &nlu.ValidationIssue{Code: "business_validation", Field: "arguments"}
				if target := actionTarget(op); target != nil && target.Type == "last" {
					issue = &nlu.ValidationIssue{Code: "named_target_required", Field: "arguments.target"}
				}
			}
			return errDryRun
		})
		if errors.Is(returnErr, errDryRun) {
			return nil
		}
		return returnErr
	})
	return issue, err
}
