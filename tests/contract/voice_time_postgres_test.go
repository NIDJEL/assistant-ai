package contract

import (
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

func TestPostgresMeetingTimeReplyAlsoChangesDuration(t *testing.T) {
	for _, reply := range []string{"continue", "choose_time"} {
		for _, knownFriend := range []bool{false, true} {
			name := "personal"
			if knownFriend {
				name = "friend invitation"
			}
			t.Run(reply+"/"+name, func(t *testing.T) {
				h := schedulingFixture(t)
				h.now = time.Date(2026, 10, 20, 23, 55, 0, 0, h.now.Location())
				args := `{"title":"Встреча с дизайнером","friends":["дизайнер"],"date":{"type":"relative_day","offset":2},"duration_minutes":60,"reminder_minutes":40}`
				if knownFriend {
					args = `{"title":"Встреча с Матвеем Лапиным","friends":["Матвей Лапин"],"invite":true,"date":{"type":"relative_day","offset":2},"duration_minutes":60,"reminder_minutes":40}`
				}
				if r := h.say(h.a, "create_meeting", args, "Запиши послезавтра встречу"); !r.Pending || personalRecordCount(t, h) != 0 {
					t.Fatal(r)
				}
				if r := h.say(h.a, reply, `{"changes":{"time":"16:15","duration_minutes":0}}`, "В четыре пятнадцать, на ноль минут"); !r.Pending || personalRecordCount(t, h) != 0 {
					t.Fatal("invalid duration lost pending creation or wrote a record", r)
				}
				h.now = h.now.Add(20 * time.Minute)
				r := h.say(h.a, reply, `{"changes":{"time":"16:15","duration_minutes":30}}`, "В 4-15 дня на полчаса")
				if r.Action != domain.ActionCreate || r.Pending || len(r.Events) != 1 || personalRecordCount(t, h) != 1 {
					t.Fatal(r)
				}
				e := r.Events[0]
				if e.Date.String() != "2026-10-22" || e.StartTime.String() != "16:15" || e.Duration != 30*time.Minute || e.ReminderMinutes == nil || *e.ReminderMinutes != 40 || e.Collaborative != knownFriend {
					t.Fatal("time reply changed original fields", e)
				}
				if !knownFriend && !strings.Contains(r.Reply, "Приглашения не отправлены") {
					t.Fatal(r.Reply)
				}
			})
		}
	}
}
