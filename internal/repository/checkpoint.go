package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func SQLCheckpoint(ctx context.Context, sequence *uint64, exec func(context.Context, string, ...any) (sql.Result, error), work func() error) (err error) {
	*sequence++
	name := fmt.Sprintf("gena_checkpoint_%d", *sequence)
	if _, err = exec(ctx, "SAVEPOINT "+name); err != nil {
		return err
	}
	finished := false
	defer func() {
		if !finished {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			_, rollbackErr := exec(cleanup, "ROLLBACK TO SAVEPOINT "+name)
			_, releaseErr := exec(cleanup, "RELEASE SAVEPOINT "+name)
			if rollbackErr != nil {
				err = fmt.Errorf("checkpoint rollback failed: %w", rollbackErr)
			} else if releaseErr != nil {
				err = fmt.Errorf("checkpoint release failed: %w", releaseErr)
			}
		}
	}()
	if err = work(); err != nil {
		return err
	}
	_, err = exec(ctx, "RELEASE SAVEPOINT "+name)
	finished = err == nil
	return err
}
