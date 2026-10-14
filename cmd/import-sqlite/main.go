package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/NIDJEL/assistant-ai/internal/auth"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/storage/postgres"
	"github.com/NIDJEL/assistant-ai/internal/storage/sqlite"
	"os"
)

func run() error {
	path := flag.String("sqlite", "", "SQLite database (schema v5; older versions migrate with backup)")
	profile := flag.String("profile", "", "legacy preferences profile UUID, required when multiple exist")
	flag.Parse()
	if *path == "" {
		return errors.New("-sqlite path required")
	}
	if _, err := os.Stat(*path); err != nil {
		return errors.New("SQLite source file does not exist")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("DATABASE_URL required")
	}
	ctx := context.Background()
	db, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	svc, err := auth.New(db, cfg.SessionSecret, nil)
	if err != nil {
		return err
	}
	email := cfg.ImportUserEmail
	if email == "" {
		email = "dev@rhythm.local"
	}
	name := cfg.ImportUserName
	if name == "" {
		name = "Разработчик"
	}
	password := cfg.ImportUserPassword
	if password == "" {
		return errors.New("set IMPORT_USER_PASSWORD (at least 10 characters); never pass passwords as command arguments")
	}
	u, err := svc.Login(ctx, email, password)
	if err != nil {
		u, err = svc.Register(ctx, name, email, password, password)
	}
	if err != nil {
		return err
	}
	source, err := sqlite.Open(ctx, *path)
	if err != nil {
		return err
	}
	defer source.Close()
	imported, err := db.ImportSQLite(ctx, source.DB, u.ID, *profile)
	if err != nil {
		return err
	}
	fmt.Printf("Imported %d events into user %s (repeat snapshots are skipped)\n", imported, u.ID)
	return nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Import failed:", err)
		os.Exit(1)
	}
}
