// Read-only PostgreSQL probe used by Playwright; never serves an HTTP endpoint.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/jackc/pgx/v5/stdlib"
	"os"
	"time"
)

func main() {
	if len(os.Args) != 3 {
		os.Exit(2)
	}
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://rhythm@127.0.0.1:5433/rhythm?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		os.Exit(1)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var id string
	var raw []byte
	err = db.QueryRowContext(ctx, `SELECT id,payload FROM notification_occurrences WHERE user_id=$1 AND (($2='test' AND event_id IS NULL) OR event_id::text=$2) ORDER BY created_at DESC LIMIT 1`, os.Args[1], os.Args[2]).Scan(&id, &raw)
	if err == sql.ErrNoRows {
		fmt.Print("{}")
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "notification probe failed")
		os.Exit(1)
	}
	var payload any
	if json.Unmarshal(raw, &payload) != nil {
		os.Exit(1)
	}
	var count int
	if err = db.QueryRowContext(ctx, "SELECT count(*) FROM notification_deliveries WHERE notification_id=$1", id).Scan(&count); err != nil {
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"payload": payload, "deliveries": count})
}
