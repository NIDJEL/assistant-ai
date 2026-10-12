package sqlite_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/NIDJEL/assistant-ai/internal/storage/sqlite"
)

func TestFutureSchemaIsRejected(t *testing.T) {
	path := fixture(t)
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = raw.Exec("PRAGMA user_version=99"); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	if db, err := sqlite.Open(context.Background(), path); err == nil {
		db.Close()
		t.Fatal("future schema accepted")
	}
}
