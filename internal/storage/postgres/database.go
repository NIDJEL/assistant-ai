package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/repository"
	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Database struct{ DB *sql.DB }

func Open(ctx context.Context, url string) (*Database, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)
	d := &Database{DB: db}
	ready, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for {
		if err = db.PingContext(ready); err == nil {
			break
		}
		select {
		case <-ready.Done():
			db.Close()
			return nil, fmt.Errorf("PostgreSQL unavailable")
		case <-time.After(250 * time.Millisecond):
		}
	}
	if err = d.Migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return d, nil
}
func (d *Database) Close() error                   { return d.DB.Close() }
func (d *Database) Ping(ctx context.Context) error { return d.DB.PingContext(ctx) }
func (d *Database) Migrate(ctx context.Context) error {
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(73121012)"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations(name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())"); err != nil {
		return err
	}
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		var exists bool
		if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)", entry.Name()).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		body, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("migration %s: %w", entry.Name(), err)
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations(name) VALUES($1)", entry.Name()); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (d *Database) Read(ctx context.Context, fn func(repository.Tx) error) error {
	id := repository.UserID(ctx)
	if id == "" {
		return fmt.Errorf("authenticated user scope required")
	}
	return fn(&Repo{q: d.DB, userID: id})
}
func (d *Database) Write(ctx context.Context, fn func(repository.Tx) error) error {
	id := repository.UserID(ctx)
	if id == "" {
		return fmt.Errorf("authenticated user scope required")
	}
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Serializes domain writes per user across server instances; LLM HTTP remains
	// outside the transaction. Prevents meeting conflicts and idempotency races.
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", id); err != nil {
		return err
	}
	if err = fn(&Repo{q: tx, userID: id}); err != nil {
		return err
	}
	return tx.Commit()
}
