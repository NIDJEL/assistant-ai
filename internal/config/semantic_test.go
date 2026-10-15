package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSemanticProviderDefaultsAndGigaChatConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("APP_ROOT", root)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SemanticProvider != "" || cfg.NLPProvider != "rules" || cfg.GigaChat.Timeout != 30*time.Second || cfg.GigaChat.MaxConcurrent != 1 {
		t.Fatal("production defaults changed")
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("GENA_SEMANTIC_PROVIDER=gigachat\nGIGACHAT_AUTHORIZATION_KEY=unit-only\nGIGACHAT_TIMEOUT=12s\nGIGACHAT_MAX_CONCURRENT=1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SemanticProvider != "gigachat" || cfg.GigaChat.Timeout != 12*time.Second || cfg.GigaChat.AuthorizationKey == "" {
		t.Fatal("GigaChat .env not loaded")
	}
	t.Setenv("GENA_SEMANTIC_PROVIDER", "rules")
	cfg, err = Load()
	if err != nil || cfg.SemanticProvider != "rules" {
		t.Fatal("environment should override .env")
	}
}

func TestInvalidSemanticConfiguration(t *testing.T) {
	for key, value := range map[string]string{"GENA_SEMANTIC_PROVIDER": "bad", "GIGACHAT_TIMEOUT": "secret-invalid-duration", "GIGACHAT_MAX_CONCURRENT": "0"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv("APP_ROOT", t.TempDir())
			t.Setenv(key, value)
			if _, err := Load(); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}
