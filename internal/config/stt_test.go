package config

import (
	"strings"
	"testing"
)

func TestSTTConfiguration(t *testing.T) {
	t.Setenv("APP_ROOT", t.TempDir())
	t.Setenv("GENA_STT_PROVIDER", "groq")
	t.Setenv("GROQ_API_KEY", "test-key")
	c, err := Load()
	if err != nil || c.STTProvider != "groq" || c.GroqAPIKey != "test-key" || c.GroqSTTModel != "whisper-large-v3-turbo" {
		t.Fatal("STT configuration not loaded")
	}
	t.Setenv("GROQ_STT_MODEL", "custom-model")
	t.Setenv("GROQ_PROXY_URL", "socks5h://test-user:test-password@127.0.0.1:1080")
	c, err = Load()
	if err != nil || c.GroqSTTModel != "custom-model" || c.GroqProxyURL == "" {
		t.Fatal("model override ignored")
	}
	t.Setenv("GROQ_PROXY_URL", "socks5h://test-user:test-password@localhost:invalid")
	if _, err = Load(); err == nil || strings.Contains(err.Error(), "test-password") {
		t.Fatal("invalid proxy or unsafe configuration error")
	}
	t.Setenv("GROQ_PROXY_URL", "")
	t.Setenv("GENA_STT_PROVIDER", "unknown")
	if _, err = Load(); err == nil {
		t.Fatal("invalid provider accepted")
	}
}
