package config

import "testing"

func TestSTTConfiguration(t *testing.T) {
	t.Setenv("APP_ROOT", t.TempDir())
	t.Setenv("GENA_STT_PROVIDER", "groq")
	t.Setenv("GROQ_API_KEY", "test-key")
	c, err := Load()
	if err != nil || c.STTProvider != "groq" || c.GroqAPIKey != "test-key" || c.GroqSTTModel != "whisper-large-v3-turbo" {
		t.Fatal("STT configuration not loaded")
	}
	t.Setenv("GROQ_STT_MODEL", "custom-model")
	c, err = Load()
	if err != nil || c.GroqSTTModel != "custom-model" {
		t.Fatal("model override ignored")
	}
	t.Setenv("GENA_STT_PROVIDER", "unknown")
	if _, err = Load(); err == nil {
		t.Fatal("invalid provider accepted")
	}
}
