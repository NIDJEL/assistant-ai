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

func TestMeetingPeopleLiveZoomDialogue(t *testing.T) {
	if os.Getenv("LIVE_SIMPLE_LLM") != "1" {
		t.Skip("LIVE_SIMPLE_LLM=1 and disposable TEST_DATABASE_URL required")
	}
	h := schedulingFixture(t)
	h.now = time.Date(2026, 10, 21, 8, 30, 0, 0, h.now.Location())
	if _, err := h.db.DB.Exec(`UPDATE users SET display_name='Лапин Матвей' WHERE id=$1`, h.bid); err != nil {
		t.Fatal(err)
	}
	h.shared(h.b, h.aid, domain.Day(h.now.AddDate(0, 0, 1)), 17)
	h.personal(h.b, domain.Day(h.now.AddDate(0, 0, 2)), 16, 90)
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
	h.service = assistant.New(h.db, reviewLiveInterpreter{Simple: &llm.Simple{Provider: provider}, t: t}, h.service.Calendar)
	z := &schedulingZoom{h: h}
	h.service.Integrations = z
	steps := []string{
		"Создай встречу с Лапиным Матвеем и добавь туда ссылку в Zoom.",
		"Нет, добавь на послезавтра встречу с Лапиной Матвеем и добавь туда ссылку в зуме.",
		"Поставь её в пол пятого.",
		"23 октября, 19:30–20:30",
		"Исправь участника встречи Лапин Матвей.",
	}
	var id int64
	for index, message := range steps {
		r, err := h.service.Respond(h.a, message, h.sid, "meeting-people-live-"+string(rune('a'+index)), h.now)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("turn %d -> %s: %s", index+1, r.Action, r.Reply)
		if index < 3 {
			if !r.Pending || len(r.Events) != 0 {
				t.Fatalf("expected a clarification/slot offer: %+v", r)
			}
			if index == 2 && (r.Scheduling == nil || len(r.Scheduling.Slots) < 3) {
				t.Fatal("missing conflict choices", r)
			}
			continue
		}
		if r.Pending || len(r.Events) != 1 || !r.Events[0].Collaborative || r.Events[0].Date.String() != "2026-10-23" || r.Events[0].StartTime.String() != "19:30" || len(r.ZoomMeetings) != 1 || !domain.ValidZoomJoinURL(r.ZoomMeetings[0].JoinURL) || strings.Contains(r.Reply, "Приглашения не отправлены") {
			t.Fatalf("friend/Zoom/date were lost: %+v", r)
		}
		if index == 3 {
			id = r.Events[0].ID
			if r.Action != domain.ActionCreate {
				t.Fatal(r)
			}
		} else if r.Action != domain.ActionUpdate || r.Events[0].ID != id {
			t.Fatal("participant correction changed another event", r)
		}
	}
	var status string
	if err := h.db.DB.QueryRow(`SELECT status FROM meeting_participants WHERE event_id=$1 AND user_id=$2`, id, h.bid).Scan(&status); err != nil || status != "pending" {
		t.Fatal(status, err)
	}
	if h.count() != 2 {
		t.Fatal("duplicate meeting", h.count())
	}
	// Reproduce an old deployment's unlinked, wrongly inflected guest record.
	legacy := h.personal(h.a, domain.Day(h.now.AddDate(0, 0, 3)), 14, 60)
	if err := h.db.Write(h.a, func(tx repository.Tx) error {
		e, err := tx.Get(h.a, legacy)
		if err != nil {
			return err
		}
		e.Title = "Встреча с Лапиной Матвеем"
		e.Participants = []domain.Participant{{Name: "Лапина Матвей"}}
		e.ConferenceProvider, e.MeetingURL = "zoom", "https://zoom.us/j/123456789"
		if _, err = tx.Update(h.a, *e, h.now); err != nil {
			return err
		}
		return tx.SaveState(h.a, h.sid, domain.State{LastIDs: []int64{legacy}, Dialogue: &domain.DialogueState{LastAction: domain.ActionList, ResultAt: h.now}}, h.now)
	}); err != nil {
		t.Fatal(err)
	}
	r, err := h.service.Respond(h.a, "Исправь участника встречи Лапин Матвей.", h.sid, "legacy-person-correction", h.now)
	if err != nil || r.Action != domain.ActionUpdate || len(r.Events) != 1 || r.Events[0].ID != legacy || !r.Events[0].Collaborative || r.Events[0].Label() != "Встреча с Лапиным Матвеем" || r.Events[0].MeetingURL != "https://zoom.us/j/123456789" || h.count() != 3 {
		t.Fatalf("legacy correction failed: %+v, %v", r, err)
	}
}
