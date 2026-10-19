package postgres

import (
	"context"
	"database/sql"
	"errors"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/social"
)

func (r *Repo) SocialExecutor() social.Executor { return r.q }
func (d *Database) PlanningEvent(ctx context.Context, provider, cal, remote string) (int64, error) {
	var id int64
	err := d.DB.QueryRowContext(ctx, `SELECT event_id FROM cloud_event_links WHERE user_id=$1 AND provider=$2 AND calendar_id=$3 AND remote_id=$4`, repository.UserID(ctx), provider, cal, remote).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}
