package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"time"
)

// ImportSQLite reads a consistent SQLite snapshot and imports it atomically.
// The destination must be empty. Existing event IDs, deletion metadata,
// dialogue state, idempotency keys and one explicitly chosen profile survive.
func (d *Database) ImportSQLite(ctx context.Context, source *sql.DB, uid, profile string) (int, error) {
	if uid == "" {
		return 0, errors.New("destination user required")
	}
	snapshot, err := source.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return 0, err
	}
	defer snapshot.Rollback()
	type rowData struct {
		table  string
		values []any
	}
	data := []rowData{}
	hash := sha256.New()
	tables := []struct {
		name, query string
		cols        int
	}{
		{"events", "SELECT " + columns + " FROM events ORDER BY id", 17},
		{"dialogue_sessions", "SELECT id,state,updated_at FROM sessions ORDER BY id", 3},
		{"dialogue_exchanges", "SELECT session_id,request_id,message,response,created_at FROM exchanges ORDER BY id", 5},
	}
	for _, table := range tables {
		rows, err := snapshot.QueryContext(ctx, table.query)
		if err != nil {
			return 0, fmt.Errorf("SQLite schema v5 required: %w", err)
		}
		for rows.Next() {
			values := make([]any, table.cols)
			ptrs := make([]any, table.cols)
			for i := range values {
				ptrs[i] = &values[i]
			}
			if err = rows.Scan(ptrs...); err != nil {
				rows.Close()
				return 0, err
			}
			for i, v := range values {
				if b, ok := v.([]byte); ok {
					values[i] = string(b)
				}
			}
			raw, _ := json.Marshal(values)
			hash.Write([]byte(table.name))
			hash.Write(raw)
			data = append(data, rowData{table.name, values})
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return 0, err
		}
	}
	var preferences string
	if profile != "" {
		err = snapshot.QueryRowContext(ctx, "SELECT value FROM preferences WHERE id=?", profile).Scan(&preferences)
		if err != nil {
			return 0, fmt.Errorf("selected SQLite profile missing: %w", err)
		}
	} else {
		var n int
		if err = snapshot.QueryRowContext(ctx, "SELECT count(*) FROM preferences").Scan(&n); err != nil {
			return 0, err
		}
		if n > 1 {
			return 0, errors.New("multiple SQLite profiles; specify -profile to choose explicitly")
		}
		if n == 1 {
			if err = snapshot.QueryRowContext(ctx, "SELECT value FROM preferences LIMIT 1").Scan(&preferences); err != nil {
				return 0, err
			}
		}
	}
	hash.Write([]byte(preferences))
	digest := hex.EncodeToString(hash.Sum(nil))
	ctx = repository.WithUser(ctx, uid)
	imported := 0
	err = d.Write(ctx, func(repo repository.Tx) error {
		r := repo.(*Repo)
		if _, err := r.q.ExecContext(ctx, "LOCK TABLE events IN SHARE ROW EXCLUSIVE MODE"); err != nil {
			return err
		}
		var done bool
		if err := r.q.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM legacy_imports WHERE user_id=$1 AND source_hash=$2)", uid, digest).Scan(&done); err != nil {
			return err
		}
		if done {
			return nil
		}
		var n int
		if err := r.q.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM events WHERE user_id=$1)+(SELECT count(*) FROM dialogue_sessions WHERE user_id=$1)+(SELECT count(*) FROM preferences WHERE user_id=$1)", uid).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			return errors.New("destination already contains data; import into an empty development user")
		}
		for _, row := range data {
			args := append(row.values, uid)
			var query string
			switch row.table {
			case "events":
				query = "INSERT INTO events(" + columns + ",user_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)"
				imported++
			case "dialogue_sessions":
				query = "INSERT INTO dialogue_sessions(id,state,updated_at,user_id) VALUES($1,$2,$3,$4)"
			case "dialogue_exchanges":
				query = "INSERT INTO dialogue_exchanges(session_id,request_id,message,response,created_at,user_id) VALUES($1,$2,$3,$4,$5,$6)"
			}
			if _, err := r.q.ExecContext(ctx, query, args...); err != nil {
				return fmt.Errorf("import %s: %w", row.table, err)
			}
		}
		if preferences != "" {
			if err := r.SavePreferences(ctx, uid, json.RawMessage(preferences), time.Now().UTC()); err != nil {
				return err
			}
		}
		if _, err := r.q.ExecContext(ctx, "SELECT setval(pg_get_serial_sequence('events','id'),greatest(coalesce((SELECT max(id) FROM events),0),1),EXISTS(SELECT 1 FROM events))"); err != nil {
			return err
		}
		_, err := r.q.ExecContext(ctx, "INSERT INTO legacy_imports(user_id,source_hash) VALUES($1,$2)", uid, digest)
		return err
	})
	return imported, err
}
