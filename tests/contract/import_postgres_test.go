package contract

import (
	"context"
	"encoding/json"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/storage/sqlite"
	"path/filepath"
	"testing"
	"time"
)

func TestPostgresSQLiteImportPreservesDataAndIsIdempotent(t *testing.T) {
	db, svc, _, now := pgFixture(t)
	ctx := context.Background()
	u, err := svc.Register(ctx, "Разработчик", "dev@rhythm.local", "correct password 123", "correct password 123")
	if err != nil {
		t.Fatal(err)
	}
	source, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "legacy.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	err = source.Write(ctx, func(tx repository.Tx) error {
		e, err := tx.Create(ctx, domain.Event{Kind: domain.EventTask, Title: "Сохранённая задача", Status: domain.StatusOpen, Priority: domain.PriorityNormal, Duration: time.Hour}, now)
		if err != nil {
			return err
		}
		if err = tx.Delete(ctx, e.ID); err != nil {
			return err
		}
		if err = tx.SaveState(ctx, "old-dialogue", domain.State{}, now); err != nil {
			return err
		}
		if err = tx.SaveExchange(ctx, "old-dialogue", "old-request", "Сообщение", domain.Response{Reply: "Ответ", Action: domain.ActionHelp}, now); err != nil {
			return err
		}
		return tx.(interface {
			SavePreferences(context.Context, string, json.RawMessage, time.Time) error
		}).SavePreferences(ctx, "old-profile", json.RawMessage(`{"revision":1,"layouts":{"today":{"order":["tasks"]}}}`), now)
	})
	if err != nil {
		t.Fatal(err)
	}
	n, err := db.ImportSQLite(ctx, source.DB, u.ID, "old-profile")
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
	n, err = db.ImportSQLite(ctx, source.DB, u.ID, "old-profile")
	if err != nil || n != 0 {
		t.Fatal("repeat import", n, err)
	}
	ctx = repository.WithUser(ctx, u.ID)
	err = db.Read(ctx, func(tx repository.Tx) error {
		e, err := tx.GetDeleted(ctx, 1)
		if err != nil || e == nil || e.Title != "Сохранённая задача" {
			t.Fatal("archive/ID lost", err)
		}
		h, err := tx.History(ctx, "old-dialogue", 10)
		if err != nil || len(h) != 1 || h[0].Response.Reply != "Ответ" {
			t.Fatal("history lost", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
