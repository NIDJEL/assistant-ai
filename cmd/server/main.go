package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
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
	"github.com/NIDJEL/assistant-ai/internal/auth"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/clock"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/integrations/yandex"
	"github.com/NIDJEL/assistant-ai/internal/integrations/yandexcalendar"
	"github.com/NIDJEL/assistant-ai/internal/llm"
	"github.com/NIDJEL/assistant-ai/internal/nlp"
	"github.com/NIDJEL/assistant-ai/internal/notifications"
	"github.com/NIDJEL/assistant-ai/internal/search"
	"github.com/NIDJEL/assistant-ai/internal/storage/postgres"
	"github.com/NIDJEL/assistant-ai/internal/stt"
	"github.com/NIDJEL/assistant-ai/internal/tts"
	webpush "github.com/SherClockHolmes/webpush-go"
)

func run(log *slog.Logger) error {
	health := flag.Bool("healthcheck", false, "check running server health")
	generatePushKeys := flag.Bool("generate-web-push-keys", false, "generate VAPID keys without starting the server")
	flag.Parse()
	if *generatePushKeys {
		private, public, err := webpush.GenerateVAPIDKeys()
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(os.Stdout, "WEB_PUSH_VAPID_PUBLIC_KEY=%s\nWEB_PUSH_VAPID_PRIVATE_KEY=%s\n", public, private)
		return err
	}
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
	if cfg.DatabaseURL == "" {
		return errors.New("DATABASE_URL is required; runtime uses PostgreSQL. See .env.example")
	}
	if len(cfg.SessionSecret) < 32 {
		return errors.New("SESSION_SECRET must contain at least 32 characters")
	}
	store, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	providers := map[string]auth.Provider{}
	if cfg.YandexClientID != "" && cfg.YandexClientSecret != "" && cfg.YandexRedirectURI != "" {
		providers["yandex"] = yandex.New(cfg.YandexClientID, cfg.YandexClientSecret, cfg.YandexRedirectURI)
	}
	authService, err := auth.New(store, cfg.SessionSecret, providers)
	if err != nil {
		return err
	}
	calendarSync, err := yandexcalendar.New(store, store, &yandexcalendar.DAV{}, yandex.New(cfg.YandexClientID, cfg.YandexClientSecret, cfg.YandexRedirectURI), cfg.SessionSecret, cfg.Location, log)
	if err != nil {
		return err
	}
	authService.Calendar = calendarSync
	workerCtx, workerStop := context.WithCancel(ctx)
	defer workerStop()
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); calendarSync.Run(workerCtx) }()
	defer func() { workerStop(); <-workerDone }()
	var pushSender notifications.Sender
	if cfg.WebPush.Enabled() {
		pushSender = notifications.NewSender(cfg.WebPush)
	}
	reminderWorker := &notifications.Worker{Store: store, Sender: pushSender, Location: cfg.Location, Logger: log}
	pushDone := make(chan struct{})
	go func() { defer close(pushDone); reminderWorker.Run(workerCtx) }()
	defer func() { workerStop(); <-pushDone }()
	engine, err := search.New()
	if err != nil {
		return err
	}
	cal := &calendar.Service{Store: store, Search: engine, Location: cfg.Location, DemoPath: filepath.Join(cfg.Root, "data/voice_assistant_events_1000.csv")}
	now := clock.System{Location: cfg.Location}
	var interpreter assistant.Interpreter
	if cfg.NLUMode == "simple_llm" {
		switch cfg.SemanticProvider {
		case "", "gigachat":
			provider, err := llm.NewGigaChat(cfg.GigaChat)
			if err != nil {
				return err
			}
			defer provider.Close()
			interpreter = &llm.Simple{Provider: provider}
			if cfg.SearchMode == "cloud" {
				engine.EnableCloud(llm.CloudSearch{Client: provider}, cfg.SearchBudget)
			}
			if cfg.SearchMode == "hybrid" {
				engine.EnableHybrid(llm.GigaEmbeddings{Client: provider, Model: cfg.EmbeddingModel}, cfg.SearchBudget)
			}
		default:
			return errors.New("simple_llm requires gigachat; use semantic_plan_legacy for rules")
		}
	} else {
		switch cfg.SemanticProvider {
		case "gigachat":
			provider, err := llm.NewGigaChat(cfg.GigaChat)
			if err != nil {
				return err
			}
			defer provider.Close()
			interpreter = &llm.Planner{Provider: provider}
		case "rules":
			interpreter = nlp.Parser{}
		default:
			interpreter = nlp.Parser{}
		}
	}
	service := assistant.New(store, interpreter, cal)
	service.TurnTimeout = cfg.TurnTimeout
	service.AskReminderSettings = cfg.NLUMode != "simple_llm"
	service.ObserveTurn = func(o assistant.DialogueObservation) {
		if o.Route.Path != "" {
			log.Info("nlu_route", "route", o.Route.Path, "reason", o.Route.Reason, "semantic_calls", o.SemanticCalls, "search_calls", o.Metrics.SearchCalls, "plan_valid", o.PlanValid, "repairs", o.Metrics.Repairs, "plan_steps", o.Metrics.PlanSteps, "waiting_steps", o.Metrics.WaitingSteps, "state_bytes", o.Metrics.StateBytes, "precommit_ms", o.Metrics.DurationMS, "latency_ms", float64(o.InterpretationLatency.Microseconds())/1000)
		}
	}
	app := &api.API{Config: cfg, Clock: now, Calendar: cal, CalendarSync: calendarSync, Assistant: service, Auth: authService, Logger: log}
	if cfg.TTSProvider == "azure" {
		synthesizer, err := tts.NewAzure(cfg.AzureSpeechKey, cfg.AzureSpeechRegion, cfg.AzureTTSVoice)
		if err != nil {
			return err
		}
		defer synthesizer.Close()
		app.Synthesizer = synthesizer
	}
	if cfg.STTProvider == "groq" {
		transcriber, err := stt.NewGroqWithProxy(cfg.GroqAPIKey, cfg.GroqSTTModel, cfg.GroqProxyURL)
		if err != nil {
			return err
		}
		defer transcriber.Close()
		app.Transcriber = transcriber
	}
	server := &http.Server{Addr: cfg.Address, Handler: app.Router(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: max(50*time.Second, cfg.TurnTimeout+5*time.Second), IdleTimeout: 60 * time.Second,
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
