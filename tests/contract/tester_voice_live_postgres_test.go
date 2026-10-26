package contract

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/llm"
	"github.com/NIDJEL/assistant-ai/internal/stt"
	"github.com/NIDJEL/assistant-ai/internal/tts"
)

// Opt-in real-provider coverage with synthesized input, never a claim that a
// physical microphone or a user's device has been tested.
func TestReportedLiveSpeechRoundTrip(t *testing.T) {
	if os.Getenv("LIVE_SPEECH") != "1" {
		t.Skip("set LIVE_SPEECH=1 for Azure -> Groq -> GigaChat -> PostgreSQL -> Azure")
	}
	t.Setenv("APP_ROOT", root())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	speaker, err := tts.NewAzure(cfg.AzureSpeechKey, cfg.AzureSpeechRegion, cfg.AzureTTSVoice)
	if err != nil {
		t.Fatal(err)
	}
	defer speaker.Close()
	listener, err := stt.NewGroqWithProxy(cfg.GroqAPIKey, cfg.GroqSTTModel, cfg.GroqProxyURL)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	model, err := llm.NewGigaChat(cfg.GigaChat)
	if err != nil {
		t.Fatal(err)
	}
	defer model.Close()
	results := []map[string]any{}
	for _, tc := range []struct {
		text, title, day, clock string
		kind                    domain.EventType
	}{
		{"Напомни мне завтра в десять утра позвонить Ивану", "Иван", "2026-10-27", "10:00", domain.EventReminder},
		{"Через два часа напомни выпить таблетки", "таблет", "2026-10-27", "00:15", domain.EventReminder},
		{"В пятницу нужно отправить отчет", "отч", "2026-10-30", "", domain.EventTask},
	} {
		t.Run(tc.text, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			audio, err := speaker.Synthesize(ctx, tc.text, cfg.AzureTTSVoice)
			if err != nil {
				t.Fatal("input synthesis:", err)
			}
			transcript, err := listener.Transcribe(ctx, "tester-report.mp3", "audio/mpeg", audio)
			if err != nil {
				t.Fatal("speech transcription:", err)
			}
			h := schedulingFixture(t)
			h.now = time.Date(2026, 10, 26, 22, 15, 0, 0, h.now.Location())
			h.service = assistant.New(h.db, reviewLiveInterpreter{Simple: &llm.Simple{Provider: model}, t: t}, h.service.Calendar)
			r, err := h.service.Respond(llm.WithSource(h.a, "voice"), transcript, h.sid, "voice-report", h.now)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%q -> %s", transcript, r.Reply)
			if r.Action != domain.ActionCreate || r.Pending || len(r.Events) != 1 {
				t.Fatal(r)
			}
			e := r.Events[0]
			if e.Kind != tc.kind || e.Date == nil || e.Date.String() != tc.day || tc.clock != "" && (e.StartTime == nil || e.StartTime.String() != tc.clock) {
				t.Fatal(e)
			}
			answer, err := speaker.Synthesize(ctx, r.Reply, cfg.AzureTTSVoice)
			if err != nil || len(answer) < 100 {
				t.Fatal("answer synthesis:", err)
			}
			results = append(results, map[string]any{"input": tc.text, "transcript": transcript, "reply": r.Reply, "input_audio_bytes": len(audio), "answer_audio_bytes": len(answer), "date": e.Date.String(), "pass": true})
		})
	}
	if !t.Failed() {
		data, _ := json.MarshalIndent(results, "", "  ")
		p := filepath.Join(root(), "artifacts", "tester_voice_live.json")
		if err = os.MkdirAll(filepath.Dir(p), 0750); err == nil {
			err = os.WriteFile(p, data, 0600)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}
