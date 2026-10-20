package contract

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/config"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/llm"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

func TestAssignmentLiveTypedContextReference(t *testing.T) {
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
	for _, text := range []string{"Удали эту встречу", "Убери, пожалуйста, эту встречу", "Перенеси эту встречу на шесть вечера"} {
		t.Run(text, func(t *testing.T) {
			h := schedulingFixture(t)
			h.now = time.Date(2026, 10, 20, 12, 0, 0, 0, h.now.Location())
			h.service = assistant.New(h.db, reviewLiveInterpreter{Simple: &llm.Simple{Provider: provider}, t: t}, h.service.Calendar)
			var meetingID, taskID int64
			err := h.db.Write(h.a, func(tx repository.Tx) error {
				for i, seed := range []struct {
					title string
					kind  domain.EventType
					day   int
					hour  int
				}{
					{"Подготовиться к встрече с Пульсом", domain.EventTask, 1, 10},
					{"Встреча с Иваном", domain.EventMeeting, 1, 17},
					{"Встреча с дизайнером вне последнего списка", domain.EventMeeting, 2, 12},
				} {
					day := domain.Day(h.now.AddDate(0, 0, seed.day))
					e, err := tx.Create(h.a, domain.Event{Title: seed.title, Kind: seed.kind, Date: day, StartTime: &domain.TimeOfDay{Hour: seed.hour, Minute: 30}, Duration: time.Hour, Status: domain.StatusOpen, Priority: domain.PriorityNormal}, h.now)
					if err != nil {
						return err
					}
					if i == 0 {
						taskID = e.ID
					} else if i == 1 {
						meetingID = e.ID
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			say := func(message string) domain.Response {
				t.Helper()
				r, err := h.service.Respond(h.a, message, h.sid, "typed-"+message, h.now)
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("%s -> %s: %s", message, r.Action, r.Reply)
				return r
			}
			if r := say("Покажи в итоге, что у меня завтра"); r.Action != domain.ActionList || len(r.Events) != 2 {
				t.Fatal("mixed list missing", r)
			}
			r := say(text)
			if strings.HasPrefix(text, "Перенеси") {
				if r.Action != domain.ActionUpdate || r.Pending || len(r.Events) != 1 || r.Events[0].ID != meetingID || r.Events[0].StartTime.String() != "18:00" {
					t.Fatal("wrong contextual update", r)
				}
			} else {
				if !r.Pending || !strings.Contains(r.Reply, "Удалить") || !strings.Contains(r.Reply, "Иваном") || personalRecordCount(t, h) != 3 {
					t.Fatal("expected confirmation of the sole displayed meeting", r)
				}
				if r := say("Передумал, отмена"); r.Pending || r.Action != domain.ActionCancel || personalRecordCount(t, h) != 3 {
					t.Fatal("cancel did not preserve data", r)
				}
				if r := say("Удали эту встречу ещё раз"); !r.Pending || !strings.Contains(r.Reply, "Иваном") {
					t.Fatal(r)
				}
				if r := say("да"); r.Action != domain.ActionDelete || r.Pending || personalRecordCount(t, h) != 2 {
					t.Fatal("confirmed delete failed", r)
				}
			}
			if err := h.db.Read(h.a, func(tx repository.Tx) error {
				task, err := tx.Get(h.a, taskID)
				if task == nil || task.Kind != domain.EventTask || task.Title != "Подготовиться к встрече с Пульсом" || task.StartTime.String() != "10:30" {
					t.Fatal("task modified", task, err)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
