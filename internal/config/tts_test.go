package config

import (
	"github.com/NIDJEL/assistant-ai/internal/tts"
	"testing"
)

func TestTTSConfiguration(t *testing.T) {
	t.Setenv("APP_ROOT", t.TempDir())
	t.Setenv("GENA_TTS_PROVIDER", "azure")
	t.Setenv("AZURE_SPEECH_KEY", "test-key")
	c, err := Load()
	if err != nil || c.TTSProvider != "azure" || c.AzureTTSVoice != tts.DefaultVoice || c.AzureSpeechRegion != "eastus" || c.AzureSpeechKey != "test-key" {
		t.Fatal("TTS configuration not loaded")
	}
	t.Setenv("GENA_TTS_PROVIDER", "unknown")
	if _, err := Load(); err == nil {
		t.Fatal("invalid TTS provider accepted")
	}
}
