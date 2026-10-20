package contract

import (
	"os"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/llm"
)

func TestAssignmentLiveVoiceTimeContinuation(t *testing.T) {
	if os.Getenv("LIVE_SIMPLE_LLM") != "1" {
		t.Skip("set LIVE_SIMPLE_LLM=1 with disposable PostgreSQL for real model evaluation")
	}
	t.Setenv("APP_ROOT", root())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	provider, err := llm.NewGigaChat(cfg.GigaChat)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	for _, tc := range []struct {
		text, clock string
		duration    int
	}{
		{"В 4-15 дня на полчаса.", "16:15", 30},
		{"В 4.15 дня на полчаса.", "16:15", 30},
		{"В четыре пятнадцать дня, на полчаса", "16:15", 30},
		{"В 4.15 утра на полтора часа", "04:15", 90},
		{"В 16 15 на сорок пять минут", "16:15", 45},
	} {
		t.Run(tc.text, func(t *testing.T) {
			h := schedulingFixture(t)
			h.now = time.Date(2026, 10, 20, 16, 0, 0, 0, h.now.Location())
			h.service = assistant.New(h.db, reviewLiveInterpreter{Simple: &llm.Simple{Provider: provider}, t: t}, h.service.Calendar)
			ctx := llm.WithSource(h.a, "voice")
			r, err := h.service.Respond(ctx, "Запиши послезавтра встречу с дизайнером", h.sid, "voice-start", h.now)
			if err != nil || !r.Pending || personalRecordCount(t, h) != 0 {
				t.Fatal("expected time clarification", r, err)
			}
			r, err = h.service.Respond(ctx, tc.text, h.sid, "voice-time", h.now.Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%s -> %s: %s", tc.text, r.Action, r.Reply)
			if r.Action != domain.ActionCreate || r.Pending || len(r.Events) != 1 {
				t.Fatal("voice time continuation failed", r)
			}
			e := r.Events[0]
			if e.StartTime == nil || e.StartTime.String() != tc.clock || e.Date == nil || e.Date.String() != "2026-10-22" || int(e.Duration/time.Minute) != tc.duration || e.Collaborative {
				t.Fatal("voice fields lost or guessed", e)
			}
			if count := personalRecordCount(t, h); count != 1 {
				t.Fatal("duplicate record", count)
			}
		})
	}
}
