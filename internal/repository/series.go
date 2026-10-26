package repository

import (
	"context"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"time"
)

// A series delete must also remove detached occurrences. The marker records
// which children were removed by that operation, preserving earlier deletions.
func DeleteSeriesChildren(ctx context.Context, tx Tx, parent domain.Event, now time.Time) error {
	if parent.SeriesID != nil {
		return nil
	}
	children, err := tx.Events(ctx, domain.Filter{})
	if err != nil {
		return err
	}
	for _, child := range children {
		if child.SeriesID == nil || *child.SeriesID != parent.ID {
			continue
		}
		child.SeriesDeleted = true
		if _, err = tx.Update(ctx, child, now); err != nil {
			return err
		}
		if err = tx.Delete(ctx, child.ID); err != nil {
			return err
		}
	}
	return nil
}
func CheckSeriesRestore(ctx context.Context, tx Tx, id int64) error {
	e, err := tx.GetDeleted(ctx, id)
	if err != nil || e == nil || e.SeriesID == nil {
		return err
	}
	parent, err := tx.Get(ctx, *e.SeriesID)
	if err != nil {
		return err
	}
	if parent == nil {
		return domain.Validation("Сначала восстановите основную серию")
	}
	return nil
}
func RestoreSeriesChildren(ctx context.Context, tx Tx, parent *domain.Event, now time.Time) (*domain.Event, error) {
	if parent == nil {
		return parent, nil
	}
	if parent.SeriesID != nil {
		if parent.SeriesDeleted {
			parent.SeriesDeleted = false
			return tx.Update(ctx, *parent, now)
		}
		return parent, nil
	}
	children, err := tx.Events(ctx, domain.Filter{Deleted: true})
	if err != nil {
		return nil, err
	}
	for _, child := range children {
		if child.SeriesID != nil && *child.SeriesID == parent.ID && child.SeriesDeleted {
			if _, err = tx.Restore(ctx, child.ID, now); err != nil {
				return nil, err
			}
		}
	}
	return parent, nil
}
