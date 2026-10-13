package sqlite_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/NIDJEL/assistant-ai/internal/storage/sqlite"
	"github.com/NIDJEL/assistant-ai/migrations"
)

func TestMigrateV2MeetingMetadata(t *testing.T) {
	path := fixture(t)
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	body, err := migrations.Files.ReadFile("002_event_metadata.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = raw.Exec(string(body)); err != nil {
		t.Fatal(err)
	}
	if _, err = raw.Exec("CREATE TABLE schema_migrations(name TEXT PRIMARY KEY,applied_at TEXT NOT NULL); INSERT INTO schema_migrations VALUES('001_base.sql','baseline'),('002_event_metadata.sql','baseline');PRAGMA user_version=2"); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	db, err := sqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count, version int
	var location, meetingURL string
	if err = db.DB.QueryRow("SELECT location,meeting_url FROM events WHERE id=1").Scan(&location, &meetingURL); err != nil {
		t.Fatal(err)
	}
	_ = db.DB.QueryRow("SELECT count(*) FROM events").Scan(&count)
	_ = db.DB.QueryRow("PRAGMA user_version").Scan(&version)
	if location != "" || meetingURL != "" || count != 1001 || version != migrations.LatestVersion || db.BackupPath == "" {
		t.Fatal("v2 migration changed existing data")
	}
}
