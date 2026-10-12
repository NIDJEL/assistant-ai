package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"
	_ "time/tzdata"

	"github.com/joho/godotenv"
)

type Config struct {
	Address, Root, DatabasePath, Timezone, NLPProvider, OllamaURL, OllamaModel string
	AutoImport                                                                 bool
	OllamaTimeout                                                              time.Duration
	Location                                                                   *time.Location
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
func Load() (Config, error) {
	root, err := filepath.Abs(env("APP_ROOT", "."))
	if err != nil {
		return Config{}, err
	}
	fileEnv, err := godotenv.Read(filepath.Join(root, ".env"))
	if err != nil && !os.IsNotExist(err) {
		return Config{}, fmt.Errorf(".env: %w", err)
	}
	value := func(name, fallback string) string {
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		if v, ok := fileEnv[name]; ok {
			return v
		}
		return fallback
	}
	c := Config{
		Address: value("APP_ADDR", "127.0.0.1:8081"), Root: root,
		DatabasePath: value("DATABASE_PATH", filepath.Join(root, "var/assistant-go.sqlite3")),
		Timezone:     value("APP_TIMEZONE", "Europe/Moscow"), NLPProvider: value("NLP_PROVIDER", "rules"),
		OllamaURL: value("OLLAMA_BASE_URL", "http://127.0.0.1:11434"), OllamaModel: value("OLLAMA_MODEL", "qwen2.5:7b"),
	}
	if c.Location, err = time.LoadLocation(c.Timezone); err != nil {
		return c, fmt.Errorf("APP_TIMEZONE: %w", err)
	}
	if c.AutoImport, err = strconv.ParseBool(value("AUTO_IMPORT_DEMO", "true")); err != nil {
		return c, fmt.Errorf("AUTO_IMPORT_DEMO: %w", err)
	}
	seconds, err := strconv.Atoi(value("OLLAMA_TIMEOUT", "20"))
	if err != nil || seconds < 1 || seconds > 120 {
		return c, errors.New("OLLAMA_TIMEOUT must be between 1 and 120 seconds")
	}
	c.OllamaTimeout = time.Duration(seconds) * time.Second
	if c.NLPProvider != "rules" && c.NLPProvider != "ollama" && c.NLPProvider != "hybrid" {
		return c, errors.New("NLP_PROVIDER must be rules, ollama or hybrid")
	}
	u, err := url.Parse(c.OllamaURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return c, errors.New("OLLAMA_BASE_URL must be an HTTP(S) URL")
	}
	return c, nil
}
