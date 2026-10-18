package assistant

import (
	"context"
	"errors"
	"testing"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

type cloudFixture struct {
	fail  bool
	ready []string
	syncs int
}

func (f *cloudFixture) Ready(_ context.Context, p string) error {
	f.ready = append(f.ready, p)
	if f.fail {
		return errors.New("Подключите сервис")
	}
	return nil
}
func (f *cloudFixture) SyncEvent(context.Context, int64, bool, bool) (*domain.Event, error) {
	f.syncs++
	return nil, nil
}
func TestSimpleCloudFlagsSurviveMissingTime(t *testing.T) {
	s, p, now := simpleService(t)
	external := &cloudFixture{}
	s.Integrations = external
	operationFixture(p, "create_meeting", `{"title":"С командой","date":{"type":"relative_day","offset":1},"calendar":"google","conference":"zoom"}`)
	r := say(t, s, "Создай Zoom встречу с командой завтра и добавь в Google Calendar", now)
	if !r.Pending {
		t.Fatal("missing time should ask", r)
	}
	operationFixture(p, "continue", `{"changes":{"time":"17:00"}}`)
	r = say(t, s, "В 17:00", now)
	if r.Pending || len(r.Events) != 1 || r.Events[0].ConferenceProvider != "zoom" || r.Events[0].CalendarProvider != "google" || external.syncs != 1 {
		t.Fatal("integration request lost across pending", r)
	}
}
func TestSimpleCloudDisconnectedDoesNotCreateLocalMeeting(t *testing.T) {
	s, p, now := simpleService(t)
	s.Integrations = &cloudFixture{fail: true}
	operationFixture(p, "create_meeting", `{"title":"С командой","date":{"type":"relative_day","offset":1},"time":"17:00","conference":"zoom"}`)
	r := say(t, s, "Создай Zoom встречу завтра в 17", now)
	if r.Action != domain.ActionClarify || len(r.Events) != 0 {
		t.Fatal(r)
	}
}
