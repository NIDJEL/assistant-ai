package contract

import (
	"context"
	"strings"
	"testing"

	"github.com/NIDJEL/assistant-ai/internal/assistant"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/social"
)

func personalRecordCount(t *testing.T, h *schedulingHarness) int {
	t.Helper()
	count, err := h.service.Calendar.Count(h.a)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func assertPersonalMeeting(t *testing.T, h *schedulingHarness, r domain.Response, names ...string) {
	t.Helper()
	if r.Action != domain.ActionCreate || r.Pending || len(r.Events) != 1 || r.Events[0].Collaborative {
		t.Fatalf("expected a saved personal meeting: %+v", r)
	}
	if !strings.Contains(r.Reply, "Приглашения не отправлены") {
		t.Fatal(r.Reply)
	}
	var invitations int
	if err := h.db.DB.QueryRow(`SELECT count(*) FROM meeting_participants`).Scan(&invitations); err != nil || invitations != 0 {
		t.Fatal("unexpected invitations", invitations, err)
	}
	if err := h.db.Read(h.a, func(tx repository.Tx) error {
		e, err := tx.Get(h.a, r.Events[0].ID)
		if err != nil {
			return err
		}
		if e == nil || len(e.Participants) != len(names) {
			t.Fatalf("participant names were not persisted: %+v", e)
		}
		for i, name := range names {
			if e.Participants[i].Name != name || e.Participants[i].Email != "" {
				t.Fatal(e.Participants)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresPersonalMeetingWithUnknownPerson(t *testing.T) {
	h := schedulingFixture(t)
	r := h.say(h.a, "create_meeting", `{"title":"Встреча с Иваном","friends":["Иван"],"date":{"type":"relative_day","offset":1},"time":"15:00","duration_minutes":45,"reminder_minutes":15}`, "Запиши встречу с Иваном завтра в 15")
	assertPersonalMeeting(t, h, r, "Иван")
	e := r.Events[0]
	if e.Title != "Встреча с Иваном" || e.Date.String() != domain.Day(h.now.AddDate(0, 0, 1)).String() || e.StartTime.String() != "15:00" || e.Duration.Minutes() != 45 || e.ReminderMinutes == nil || *e.ReminderMinutes != 15 {
		t.Fatal(e)
	}
	// Saving a personal meeting must not leak it into another user's calendar.
	if err := h.db.Read(h.b, func(tx repository.Tx) error {
		other, err := tx.Get(h.b, e.ID)
		if other != nil {
			t.Fatal("personal meeting visible to another user", other)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresPersonalMeetingWithUnlinkedRoleInTitle(t *testing.T) {
	for _, name := range []string{"дизайнером", "командой", "Иваном"} {
		t.Run(name, func(t *testing.T) {
			h := schedulingFixture(t)
			r := h.say(h.a, "create_meeting", `{"title":"Встреча с `+name+`","date":{"type":"relative_day","offset":1},"time":"15:00"}`, "Встреча с "+name+" завтра в 15")
			assertPersonalMeeting(t, h, r, name)
		})
	}
}

func TestPostgresPersonalMeetingMissingTimeSurvivesRestartAndNewFriend(t *testing.T) {
	h := schedulingFixture(t)
	r := h.say(h.a, "create_meeting", `{"title":"Встреча","friends":["Иван"],"date":{"type":"relative_day","offset":1}}`, "Завтра встреча с Иваном")
	if !r.Pending || personalRecordCount(t, h) != 0 || !strings.Contains(r.Reply, "Во сколько") {
		t.Fatal(r)
	}
	u, err := h.auth.Register(t.Context(), "Иван", "personal-new-friend@example.test", "test password 12345", "test password 12345")
	if err != nil {
		t.Fatal(err)
	}
	ctx := repository.WithUser(t.Context(), u.ID)
	var code string
	if err = h.db.DB.QueryRow(`SELECT friend_code FROM users WHERE id=$1`, u.ID).Scan(&code); err != nil {
		t.Fatal(err)
	}
	request := h.execute(h.a, social.Operation{Action: "request", Code: code})
	h.execute(ctx, social.Operation{Action: "accept_request", ID: request.ID})
	h.service = assistant.New(h.db, h.parser, h.service.Calendar)
	r = h.say(h.a, "continue", `{"changes":{"time":"15:00"}}`, "В три часа дня")
	assertPersonalMeeting(t, h, r, "Иван")
	if r.Events[0].Title != "Встреча с Иваном" || r.Events[0].StartTime.String() != "15:00" {
		t.Fatal(r)
	}
}

func TestPostgresPersonalMeetingMixedNamesDoesNotSendPartialInvitations(t *testing.T) {
	h := schedulingFixture(t)
	r := h.say(h.a, "create_meeting", `{"title":"Обсуждение сметы","friends":["Матвей Лапин","Иван"],"date":{"type":"relative_day","offset":1},"time":"15:00"}`, "Обсуждение сметы с Матвеем и Иваном завтра в 15")
	assertPersonalMeeting(t, h, r, "Матвей Лапин", "Иван")
	if r.Events[0].Title != "Обсуждение сметы" {
		t.Fatal(r)
	}
}

func TestPostgresPersonalMeetingDoesNotReplaceExplicitInvitation(t *testing.T) {
	for _, args := range []string{
		`{"title":"Встреча с Иваном","friends":["Иван"],"invite":true,"date":{"type":"relative_day","offset":1},"time":"15:00"}`,
		`{"title":"Встреча","friend_tag":"Неизвестный тег","date":{"type":"relative_day","offset":1},"time":"15:00"}`,
	} {
		t.Run(args, func(t *testing.T) {
			h := schedulingFixture(t)
			r := h.say(h.a, "create_meeting", args, "Пригласи участника на встречу завтра в 15")
			if !r.Pending || len(r.Events) != 0 || personalRecordCount(t, h) != 0 {
				t.Fatal(r)
			}
		})
	}
}

func TestPostgresPersonalMeetingModelOmissionDoesNotDropInvitationRequirement(t *testing.T) {
	h := schedulingFixture(t)
	r := h.say(h.a, "create_meeting", `{"title":"Встреча с Иваном","friends":["Иван"],"date":{"type":"relative_day","offset":1},"time":"15:00"}`, "Пригласи Ивана на новую встречу завтра в 15")
	if !r.Pending || personalRecordCount(t, h) != 0 || !strings.Contains(r.Reply, "не найден") {
		t.Fatal(r)
	}
}

func TestPostgresPersonalMeetingExplicitlyDeclinesInvitingKnownFriend(t *testing.T) {
	h := schedulingFixture(t)
	r := h.say(h.a, "create_meeting", `{"title":"Встреча с Матвеем Лапиным","friends":["Матвей Лапин"],"date":{"type":"relative_day","offset":1},"time":"15:00"}`, "Никого не приглашай, просто запиши встречу с Матвеем завтра в 15")
	assertPersonalMeeting(t, h, r, "Матвей Лапин")
}

func TestPostgresPersonalMeetingStorageFailureDoesNotBecomeMissingFriend(t *testing.T) {
	h := schedulingFixture(t)
	if _, err := h.db.DB.Exec(`DROP TABLE friend_tags`); err != nil {
		t.Fatal(err)
	}
	// The real service must roll back rather than treating an unreadable friend
	// list as evidence that a person is absent.
	h.parser.op.Intent, h.parser.op.Confidence = "create_meeting", "high"
	h.parser.op.Arguments = []byte(`{"title":"Встреча с Иваном","friends":["Иван"],"date":{"type":"relative_day","offset":1},"time":"15:00"}`)
	if r, err := h.service.Respond(h.a, "Встреча с Иваном завтра в 15", h.sid, "storage-error", h.now); err == nil || len(r.Events) != 0 {
		t.Fatal(r, err)
	}
	if count := personalRecordCount(t, h); count != 0 {
		t.Fatal(count)
	}
}

func TestPostgresPersonalMeetingMissingNameDoesNotHideAmbiguousFriend(t *testing.T) {
	h := schedulingFixture(t)
	u, err := h.auth.Register(t.Context(), "Матвей Другой", "personal-ambiguous@example.test", "test password 12345", "test password 12345")
	if err != nil {
		t.Fatal(err)
	}
	ctx := repository.WithUser(context.Background(), u.ID)
	var code string
	if err = h.db.DB.QueryRow(`SELECT friend_code FROM users WHERE id=$1`, u.ID).Scan(&code); err != nil {
		t.Fatal(err)
	}
	request := h.execute(h.a, social.Operation{Action: "request", Code: code})
	h.execute(ctx, social.Operation{Action: "accept_request", ID: request.ID})
	r := h.say(h.a, "create_meeting", `{"title":"Встреча","friends":["Иван","Матвей"],"date":{"type":"relative_day","offset":1},"time":"15:00"}`, "Встреча с Иваном и Матвеем завтра в 15")
	if !r.Pending || personalRecordCount(t, h) != 0 || !strings.Contains(r.Reply, "Уточните полное имя") {
		t.Fatal(r)
	}
}
