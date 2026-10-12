package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/storage/sqlite"
)

func open(t *testing.T) *sqlite.Database {
	t.Helper()
	db, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
func event() domain.Event {
	return domain.Event{Kind: domain.EventTask, Title: "Проверить договор", Duration: time.Hour, Status: domain.StatusOpen, Priority: domain.PriorityNormal}
}
func TestPragmasEveryConnection(t *testing.T) {
	db := open(t)
	held := []*sql.Conn{}
	defer func() {
		for _, c := range held {
			c.Close()
		}
	}()
	for range 4 {
		conn, err := db.DB.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, conn)
		for name, wanted := range map[string]any{"journal_mode": "wal", "busy_timeout": int64(5000), "foreign_keys": int64(1)} {
			var v any
			if err := conn.QueryRowContext(context.Background(), "PRAGMA "+name).Scan(&v); err != nil {
				t.Fatal(err)
			}
			if v != wanted {
				t.Fatalf("%s: expected %v got %v", name, wanted, v)
			}
		}
	}
}
func TestForeignKeysEnforced(t *testing.T) {
	db := open(t)
	if _, err := db.DB.Exec("CREATE TABLE parent(id INTEGER PRIMARY KEY);CREATE TABLE child(parent_id INTEGER REFERENCES parent(id))"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.Exec("INSERT INTO child VALUES(999)"); err == nil {
		t.Fatal("foreign key was not enforced")
	}
}
func TestAtomicRollbackAndCanceledContext(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	wanted := errors.New("abort")
	err := db.Write(ctx, func(tx repository.Tx) error {
		if _, err := tx.Create(ctx, event(), time.Now()); err != nil {
			return err
		}
		if err := tx.SaveState(ctx, "session", domain.State{}, time.Now()); err != nil {
			return err
		}
		if err := tx.SaveExchange(ctx, "session", "request", "text", domain.Response{Action: domain.ActionHelp, Events: []domain.Event{}}, time.Now()); err != nil {
			return err
		}
		return wanted
	})
	if !errors.Is(err, wanted) {
		t.Fatal(err)
	}
	for _, table := range []string{"events", "sessions", "exchanges"} {
		var count int
		if err = db.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	err = db.Write(cancelled, func(tx repository.Tx) error {
		if _, err := tx.Create(cancelled, event(), time.Now()); err != nil {
			return err
		}
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if err = db.Write(ctx, func(tx repository.Tx) error { _, err := tx.Create(ctx, event(), time.Now()); return err }); err != nil {
		t.Fatalf("canceled transaction leaked connection or lock: %v", err)
	}
	var count int
	_ = db.DB.QueryRow("SELECT count(*) FROM events").Scan(&count)
	if count != 1 {
		t.Fatalf("rollback failed: %d", count)
	}
}
func fixture(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../testdata/parity/legacy_v1.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "v1.sqlite3")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestMigrateV1PreservesIDsHistoryAndPending(t *testing.T) {
	path := fixture(t)
	db, err := sqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if db.BackupPath == "" {
		t.Fatal("no backup")
	}
	backup, err := sql.Open("sqlite", "file:"+filepath.ToSlash(db.BackupPath)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	for _, table := range []string{"events", "sessions", "exchanges"} {
		var before, after int
		if err = backup.QueryRow("SELECT count(*) FROM " + table).Scan(&before); err != nil {
			t.Fatal(err)
		}
		if err = db.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&after); err != nil {
			t.Fatal(err)
		}
		if before != after {
			t.Fatalf("%s before=%d after=%d", table, before, after)
		}
	}
	var version, backupVersion int
	_ = db.DB.QueryRow("PRAGMA user_version").Scan(&version)
	_ = backup.QueryRow("PRAGMA user_version").Scan(&backupVersion)
	if version != 2 || backupVersion != 1 {
		t.Fatalf("versions new=%d backup=%d", version, backupVersion)
	}
	err = db.Read(context.Background(), func(tx repository.Tx) error {
		first, err := tx.Get(context.Background(), 1)
		if err != nil {
			return err
		}
		last, err := tx.Get(context.Background(), 1001)
		if err != nil {
			return err
		}
		if first == nil || last == nil || first.Title != "Подготовить техническое задание" || last.Title != "Проверить договор" {
			t.Fatal("event IDs/data changed")
		}
		state, err := tx.State(context.Background(), "00000000-0000-0000-0000-000000000001")
		if err != nil {
			return err
		}
		if state.Pending == nil || state.Pending.Command.Kind != domain.EventMeeting || state.Pending.Command.Date.String() != "2026-10-13" {
			t.Fatal("Python pending state lost")
		}
		history, err := tx.History(context.Background(), "00000000-0000-0000-0000-000000000001", 50)
		if err != nil {
			return err
		}
		if len(history) != 2 || history[0].Response.Action != domain.ActionCreate {
			t.Fatal("history lost")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	reopened, err := sqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.BackupPath != "" {
		t.Fatal("reopening repeated the migration")
	}
}
func TestFailedMigrationRollsBack(t *testing.T) {
	path := fixture(t)
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = raw.Exec("ALTER TABLE events ADD COLUMN description TEXT NOT NULL DEFAULT ''"); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	if db, err := sqlite.Open(context.Background(), path); err == nil {
		db.Close()
		t.Fatal("malformed schema should reject migration")
	}
	raw, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var migrations, version, count int
	_ = raw.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='schema_migrations'").Scan(&migrations)
	_ = raw.QueryRow("PRAGMA user_version").Scan(&version)
	_ = raw.QueryRow("SELECT count(*) FROM events").Scan(&count)
	if migrations != 0 || version != 1 || count != 1001 {
		t.Fatalf("partial migration leaked: %d %d %d", migrations, version, count)
	}
}
