package contract

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/social"
)

func TestPostgresMeetingParticipantPhotosAreProjectedForBothAccounts(t *testing.T) {
	h := schedulingFixture(t)
	const organizerPhoto = "https://example.test/organizer.png"
	const friendPhoto = "https://example.test/friend.png"
	if _, err := h.db.DB.Exec(`UPDATE users SET avatar_url=CASE WHEN id=$1 THEN $3 ELSE $4 END WHERE id IN ($1,$2)`, h.aid, h.bid, organizerPhoto, friendPhoto); err != nil {
		t.Fatal(err)
	}
	id := h.shared(h.a, h.bid, domain.Day(h.now.AddDate(0, 0, 1)), 17)
	h.execute(h.b, social.Operation{Action: "respond", EventID: id, Version: 1, Status: "accepted"})
	for _, ctx := range []context.Context{h.a, h.b} {
		if err := h.db.Read(ctx, func(tx repository.Tx) error {
			event, err := tx.Get(ctx, id)
			if err != nil {
				return err
			}
			if event == nil || event.OrganizerAvatarURL != organizerPhoto || len(event.Participants) != 1 || event.Participants[0].ID != h.bid || event.Participants[0].AvatarURL != friendPhoto || event.Participants[0].Email != "" {
				t.Fatalf("missing participant identity/photo or exposed private email: %+v", event)
			}
			items, err := tx.Events(ctx, domain.Filter{Kind: domain.EventMeeting})
			if err == nil && (len(items) != 1 || items[0].Participants[0].AvatarURL != friendPhoto || items[0].OrganizerAvatarURL != organizerPhoto) {
				t.Fatalf("list projection differs from meeting details: %+v", items)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPostgresMeetingFriendCasesSurviveZoomConflictDialogue(t *testing.T) {
	h := schedulingFixture(t)
	h.now = time.Date(2026, 10, 21, 8, 30, 0, 0, h.now.Location())
	if _, err := h.db.DB.Exec(`UPDATE users SET display_name='Лапин Матвей' WHERE id=$1`, h.bid); err != nil {
		t.Fatal(err)
	}
	old := h.shared(h.b, h.aid, domain.Day(h.now.AddDate(0, 0, 1)), 17)
	h.personal(h.b, domain.Day(h.now.AddDate(0, 0, 2)), 16, 90)
	z := &schedulingZoom{h: h}
	h.service.Integrations = z
	r := h.say(h.a, "create_meeting", `{"title":"Встреча","friends":["Лапин Матвей"],"conference":"zoom"}`, "Создай встречу с Лапиным Матвеем и добавь туда ссылку в Zoom")
	if !r.Pending || len(r.Events) != 0 || !strings.Contains(r.Reply, "день") || z.syncs != 0 {
		t.Fatal("a new meeting without a day must not reuse an unrelated invitation", r)
	}
	r = h.say(h.a, "create_meeting", `{"title":"Встреча","friends":["Лапина Матвей"],"date":{"type":"relative_day","offset":2},"conference":"zoom"}`, "Нет, добавь на послезавтра встречу с Лапиной Матвеем и добавь туда ссылку в зуме")
	if !r.Pending || !strings.Contains(r.Reply, "Во сколько") {
		t.Fatal(r)
	}
	r = h.say(h.a, "continue", `{"changes":{"time":"16:30"}}`, "Поставь её в пол пятого")
	if !r.Pending || r.Scheduling == nil || len(r.Scheduling.Slots) < 3 || h.count() != 1 {
		t.Fatal(r)
	}
	chosen := r.Scheduling.Slots[2]
	r = h.say(h.a, "choose_time", `{"option":3}`, "23 октября, 19:30–20:30")
	if r.Pending || r.Action != domain.ActionCreate || len(r.Events) != 1 || r.Events[0].ID == old || !r.Events[0].Collaborative || r.Events[0].Title != "Встреча с Лапиным Матвеем" || r.Events[0].StartTime.String() != chosen.Start.Format("15:04") || r.Events[0].Date.String() != "2026-10-23" || h.count() != 2 {
		t.Fatal("friend/date/choice were lost", r)
	}
	if len(r.ZoomMeetings) != 1 || r.ZoomMeetings[0].JoinURL == "" || z.syncs != 1 || strings.Contains(r.Reply, "Приглашения не отправлены") {
		t.Fatal("Zoom or invitation was lost", r)
	}
	var status string
	if err := h.db.DB.QueryRow(`SELECT status FROM meeting_participants WHERE event_id=$1 AND user_id=$2`, r.Events[0].ID, h.bid).Scan(&status); err != nil || status != "pending" {
		t.Fatal(status, err)
	}
}

func TestPostgresMeetingParticipantCorrectionLinksTheSameZoomEvent(t *testing.T) {
	h := schedulingFixture(t)
	day := domain.Day(h.now.AddDate(0, 0, 2))
	id := h.personal(h.a, day, 19, 60)
	const join = "https://zoom.us/j/123456789"
	if err := h.db.Write(h.a, func(tx repository.Tx) error {
		e, err := tx.Get(h.a, id)
		if err != nil {
			return err
		}
		e.Participants = []domain.Participant{{Name: "Лапина Матвей"}}
		e.Title = domain.MeetingDisplayTitle("Встреча", []string{"Лапина Матвей"})
		e.MeetingURL, e.ConferenceProvider = join, "zoom"
		_, err = tx.Update(h.a, *e, h.now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	h.say(h.a, "list", `{"kind":"meeting"}`, "Покажи встречи")
	r := h.say(h.a, "set_meeting_participants", `{"target":{"type":"last"},"friends":["Лапин Матвей"]}`, "Исправь участника встречи: Лапин Матвей")
	if r.Action != domain.ActionUpdate || len(r.Events) != 1 || r.Events[0].ID != id || !r.Events[0].Collaborative || r.Events[0].MeetingURL != join || r.Events[0].Date.String() != day.String() || r.Events[0].StartTime.Hour != 19 || r.Events[0].Label() != "Встреча с Матвеем Лапиным" || h.count() != 1 {
		t.Fatal("correction only renamed the event or replaced its Zoom/date", r)
	}
	var invitations int
	if err := h.db.DB.QueryRow(`SELECT count(*) FROM meeting_participants WHERE event_id=$1 AND user_id=$2 AND status='pending'`, id, h.bid).Scan(&invitations); err != nil || invitations != 1 {
		t.Fatal(invitations, err)
	}
	h.execute(h.b, social.Operation{Action: "respond", EventID: id, Version: 1, Status: "accepted"})
	// Repeating the correction must retain acceptance and not send a second invite.
	h.say(h.a, "set_meeting_participants", `{"target":{"type":"last"},"friends":["Матвеем Лапиным"]}`, "Участник — Матвей Лапин")
	var status string
	var generation int
	if err := h.db.DB.QueryRow(`SELECT status,invitation_generation FROM meeting_participants WHERE event_id=$1 AND user_id=$2`, id, h.bid).Scan(&status, &generation); err != nil || status != "accepted" || generation != 1 {
		t.Fatal(status, generation, err)
	}
	r = h.say(h.b, "set_meeting_participants", `{"target":{"type":"query","value":"Встреча","filter":{"kind":"meeting"}},"friends":["Никита Круглов"]}`, "Исправь участника")
	if r.Action != domain.ActionClarify || !strings.Contains(r.Reply, "организатор") {
		t.Fatal("guest changed the organizer's participants", r)
	}
}

func TestPostgresMeetingParticipantCorrectionRejectsBusyAndUnknownWithoutPromotion(t *testing.T) {
	h := schedulingFixture(t)
	day := domain.Day(h.now.AddDate(0, 0, 1))
	id := h.personal(h.a, day, 16, 60)
	h.personal(h.b, day, 16, 60)
	h.say(h.a, "list", `{"kind":"meeting"}`, "Покажи встречи")
	for _, name := range []string{"Матвей Лапин", "Незнакомый Человек"} {
		r := h.say(h.a, "set_meeting_participants", `{"target":{"type":"last"},"friends":["`+name+`"]}`, "Исправь участника")
		if r.Action != domain.ActionClarify || h.count() != 0 {
			t.Fatal("failed correction partially converted the event", r)
		}
	}
	if err := h.db.Read(h.a, func(tx repository.Tx) error {
		e, err := tx.Get(h.a, id)
		if err == nil && (e.Collaborative || e.Title != "PRIVATE EVENT" || e.StartTime.Hour != 16) {
			t.Fatal(e)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresInflectedFriendNameAmbiguityDoesNotSendPartialInvitations(t *testing.T) {
	h := schedulingFixture(t)
	c, err := h.auth.Register(t.Context(), "Матвей Орлов", "second-matvey@example.test", "password 12345", "password 12345")
	if err != nil {
		t.Fatal(err)
	}
	var code string
	if err = h.db.DB.QueryRow(`SELECT friend_code FROM users WHERE id=$1`, c.ID).Scan(&code); err != nil {
		t.Fatal(err)
	}
	request := h.execute(h.a, social.Operation{Action: "request", Code: code})
	h.execute(repository.WithUser(context.Background(), c.ID), social.Operation{Action: "accept_request", ID: request.ID})
	r := h.say(h.a, "create_meeting", `{"title":"Встреча","friends":["Матвеем"],"date":{"type":"relative_day","offset":1},"time":"15:00"}`, "Создай встречу с Матвеем завтра в три")
	if !r.Pending || !strings.Contains(r.Reply, "Матвей Лапин") || !strings.Contains(r.Reply, "Матвей Орлов") || h.count() != 0 {
		t.Fatal("ambiguous friend was silently chosen or made personal", r)
	}
}

func TestPostgresMeetingParticipantReplacementCancelsOnlyExcludedGuest(t *testing.T) {
	h := schedulingFixture(t)
	c, err := h.auth.Register(t.Context(), "Павел Сергеев", "replacement-friend@example.test", "password 12345", "password 12345")
	if err != nil {
		t.Fatal(err)
	}
	var code string
	if err = h.db.DB.QueryRow(`SELECT friend_code FROM users WHERE id=$1`, c.ID).Scan(&code); err != nil {
		t.Fatal(err)
	}
	request := h.execute(h.a, social.Operation{Action: "request", Code: code})
	h.execute(repository.WithUser(t.Context(), c.ID), social.Operation{Action: "accept_request", ID: request.ID})
	id := h.shared(h.a, h.bid, domain.Day(h.now.AddDate(0, 0, 1)), 17)
	h.execute(h.b, social.Operation{Action: "respond", EventID: id, Version: 1, Status: "accepted"})
	h.say(h.a, "list", `{"kind":"meeting"}`, "Покажи встречи")
	r := h.say(h.a, "set_meeting_participants", `{"target":{"type":"last"},"friends":["Сергеевым Павлом"]}`, "Замени участника встречи на Павла Сергеева")
	if r.Action != domain.ActionUpdate || len(r.Events) != 1 || r.Events[0].ID != id || h.count() != 1 {
		t.Fatal(r)
	}
	var oldStatus, newStatus string
	if err = h.db.DB.QueryRow(`SELECT status FROM meeting_participants WHERE event_id=$1 AND user_id=$2`, id, h.bid).Scan(&oldStatus); err != nil || oldStatus != "cancelled" {
		t.Fatal(oldStatus, err)
	}
	if err = h.db.DB.QueryRow(`SELECT status FROM meeting_participants WHERE event_id=$1 AND user_id=$2`, id, c.ID).Scan(&newStatus); err != nil || newStatus != "pending" {
		t.Fatal(newStatus, err)
	}
}
