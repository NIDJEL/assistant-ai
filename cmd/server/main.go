package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/api"
	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/clock"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/llm"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
	"github.com/NIDJEL/assistant-ai/internal/search"
	"github.com/NIDJEL/assistant-ai/internal/storage/sqlite"
)

func run(log *slog.Logger) error {
	health := flag.Bool("healthcheck", false, "check running server health")
	flag.Parse()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if *health {
		client := http.Client{Timeout: 3 * time.Second}
		address := cfg.Address
		if len(address) >= 8 && address[:8] == "0.0.0.0:" {
			address = "127.0.0.1:" + address[8:]
		}
		response, err := client.Get("http://" + address + "/health")
		if err != nil {
			return err
		}
		response.Body.Close()
		if response.StatusCode != 200 {
			return errors.New("unhealthy")
		}
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store, err := sqlite.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer store.Close()
	if store.BackupPath != "" {
		log.Info("database backed up before migration", "path", store.BackupPath)
	}
	engine, err := search.New()
	if err != nil {
		return err
	}
	cal := &calendar.Service{Store: store, Search: engine, Location: cfg.Location, DemoPath: filepath.Join(cfg.Root, "data/voice_assistant_events_1000.csv")}
	now := clock.System{Location: cfg.Location}
	if cfg.AutoImport {
		result, err := cal.ImportDemo(ctx, now.Now())
		if err != nil {
			return err
		}
		log.Info("demo import", "imported", result.Imported, "skipped", result.Skipped)
	}
	interpreter := nlp.Hybrid{}
	if cfg.NLPProvider != "rules" {
		interpreter.Fallback = &llm.Ollama{URL: cfg.OllamaURL, Model: cfg.OllamaModel, Timezone: cfg.Timezone, Client: &http.Client{Timeout: cfg.OllamaTimeout}}
	}
	service := assistant.New(store, interpreter, cal)
	app := &api.API{Config: cfg, Clock: now, Calendar: cal, Assistant: service, Logger: log}
	server := &http.Server{Addr: cfg.Address, Handler: app.Router(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 50 * time.Second, IdleTimeout: 60 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx }, ErrorLog: slog.NewLogLogger(log.Handler(), slog.LevelError)}
	done := make(chan error, 1)
	go func() {
		log.Info("server started", "address", cfg.Address, "backend", "go")
		done <- server.ListenAndServe()
	}()
	select {
	case err = <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		log.Info("server shutdown")
		return server.Shutdown(shutdown)
	}
}
func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
