package postgres

import (
	"context"
	"fmt"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

type Connection struct {
	Provider string `json:"provider"`
	Status   string `json:"status"`
}

func (d *Database) Connections(ctx context.Context) ([]Connection, error) {
	id := repository.UserID(ctx)
	if id == "" {
		return nil, fmt.Errorf("authenticated user scope required")
	}
	rows, err := d.DB.QueryContext(ctx, "SELECT provider,status FROM calendar_connections WHERE user_id=$1 ORDER BY provider", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Connection{}
	for rows.Next() {
		var c Connection
		if err = rows.Scan(&c.Provider, &c.Status); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}
