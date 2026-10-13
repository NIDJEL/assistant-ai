package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/migrations"
	_ "modernc.org/sqlite"
)

type Database struct {
	DB               *sql.DB
	Path, BackupPath string
	writer           chan struct{}
}

func Open(ctx context.Context, path string) (*Database, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(abs), 0750); err != nil {
		return nil, err
	}
	p := filepath.ToSlash(abs)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	uri := url.URL{Scheme: "file", Path: p}
	q := uri.Query()
	for _, pragma := range []string{"journal_mode(WAL)", "busy_timeout(5000)", "foreign_keys(1)"} {
		q.Add("_pragma", pragma)
	}
	uri.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	result := &Database{DB: db, Path: abs, writer: make(chan struct{}, 1)}
	if err = db.PingContext(ctx); err == nil {
		err = result.migrate(ctx)
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	return result, nil
}
func (d *Database) Close() error { return d.DB.Close() }
func (d *Database) Read(ctx context.Context, fn func(repository.Tx) error) error {
	return fn(&Repo{q: d.DB})
}
func (d *Database) Write(ctx context.Context, fn func(repository.Tx) error) (err error) {
	select {
	case d.writer <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-d.writer }()
	conn, err := d.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	// Rollback must still run when the request context has been canceled.
	committed := false
	defer func() {
		if !committed {
			cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, _ = conn.ExecContext(cleanup, "ROLLBACK")
		}
	}()
	if err = fn(&Repo{q: conn}); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}
func (d *Database) migrate(ctx context.Context) error {
	var version int
	if err := d.DB.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > migrations.LatestVersion {
		return fmt.Errorf("unsupported database schema version %d", version)
	}
	var hasEvents int
	if err := d.DB.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name='events'").Scan(&hasEvents); err != nil {
		return err
	}
	var hasMigrations int
	if err := d.DB.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'").Scan(&hasMigrations); err != nil {
		return err
	}
	entries, err := migrations.Files.ReadDir(".")
	if err != nil {
		return err
	}
	names := []string{}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	applied := map[string]bool{}
	if hasMigrations > 0 {
		rows, err := d.DB.QueryContext(ctx, "SELECT name FROM schema_migrations")
		if err != nil {
			return err
		}
		for rows.Next() {
			var name string
			if err = rows.Scan(&name); err != nil {
				rows.Close()
				return err
			}
			applied[name] = true
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	pending := []string{}
	for _, n := range names {
		if !applied[n] {
			pending = append(pending, n)
		}
	}
	if len(pending) == 0 {
		return nil
	}
	// VACUUM INTO snapshots the database including committed WAL pages.
	if hasEvents > 0 {
		backup := fmt.Sprintf("%s.backup-%d.sqlite3", d.Path, time.Now().UnixNano())
		if _, err = d.DB.ExecContext(ctx, "VACUUM INTO ?", backup); err != nil {
			return fmt.Errorf("backup before migration: %w", err)
		}
		d.BackupPath = backup
	}
	return d.Write(ctx, func(tx repository.Tx) error {
		r := tx.(*Repo)
		if _, err := r.q.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations(name TEXT PRIMARY KEY,applied_at TEXT NOT NULL)"); err != nil {
			return err
		}
		for _, name := range pending {
			body, err := migrations.Files.ReadFile(name)
			if err != nil {
				return err
			}
			if _, err = r.q.ExecContext(ctx, string(body)); err != nil {
				return fmt.Errorf("migration %s: %w", name, err)
			}
			if _, err = r.q.ExecContext(ctx, "INSERT INTO schema_migrations VALUES(?,?)", name, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
				return err
			}
		}
		_, err := r.q.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", migrations.LatestVersion))
		return err
	})
}
