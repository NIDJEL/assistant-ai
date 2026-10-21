package social

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

// Validate the entire replacement before changing invitations. The caller's
// calendar transaction commits the event, invitations and notifications together.
func (s *Session) setParticipants(ctx context.Context, op Operation, now time.Time) (Result, error) {
	r := Result{EventID: op.EventID}
	tx := eventTx(ctx)
	if tx == nil {
		return r, errors.New("calendar transaction required")
	}
	event, err := tx.Get(ctx, op.EventID)
	if err != nil {
		return r, err
	}
	if event == nil || event.OrganizerID != s.UserID {
		return r, notFound()
	}
	if event.Kind != domain.EventMeeting || event.Status != domain.StatusOpen || event.DeletedAt != nil {
		return r, domain.Validation("Выберите действующую встречу")
	}
	if len(op.FriendIDs) == 0 {
		return r, domain.Validation("Укажите участников встречи")
	}
	if err = s.checkInvitees(ctx, op.FriendIDs); err != nil {
		return r, err
	}
	var meeting *Meeting
	if event.Collaborative {
		meeting, err = s.Meeting(ctx, tx, event.ID)
		if err != nil {
			return r, err
		}
		if meeting.Cancelled || meeting.Version != op.Version {
			return r, problem(409, "stale_invitation", "Встреча изменилась. Повторите запрос.")
		}
	}
	start, end, valid := event.Interval(s.Location)
	if !valid {
		return r, domain.Validation("Для общей встречи нужны дата и время")
	}
	availability, err := s.Availability(ctx, op.FriendIDs, Slot{Start: start, End: end}, event.ID, now)
	if err != nil {
		return r, err
	}
	if anyBusy(availability) {
		return r, problem(409, "availability_conflict", "Выбранное время занято у участника. Сначала выберите другое время встречи.")
	}
	// Repair titles generated from an unlinked guest, preserving actual subjects.
	if !event.Collaborative {
		legacy := []string{}
		for _, person := range event.Participants {
			legacy = append(legacy, person.Name)
		}
		if len(legacy) > 0 && strings.EqualFold(event.Title, domain.MeetingDisplayTitle("Встреча", legacy)) {
			event.Title = "Встреча"
		}
	}
	event.Participants = nil
	if _, err = tx.Update(ctx, *event, now); err != nil {
		return r, err
	}
	if meeting == nil {
		if _, err = s.Q.ExecContext(ctx, `INSERT INTO collaborative_meetings(event_id,timezone) VALUES($1,$2)`, event.ID, s.Location.String()); err != nil {
			return r, err
		}
		meeting = &Meeting{Event: *event, Version: 1}
	}
	selected := map[string]bool{}
	for _, id := range op.FriendIDs {
		selected[id] = true
	}
	added, removed := len(selected), 0
	for _, member := range meeting.Members {
		if member.Status != "cancelled" && selected[member.ID] {
			added--
		}
		if member.Status != "cancelled" && !selected[member.ID] {
			removed++
			if _, err = s.meetingOperation(ctx, Operation{Action: "cancel_invitation", EventID: event.ID, Version: meeting.Version, UserID: member.ID}, now); err != nil {
				return r, err
			}
		}
	}
	if err = s.invite(ctx, *event, op.FriendIDs, meeting.Version, now); err != nil {
		return r, err
	}
	r.Message = "Участники встречи обновлены."
	if added > 0 {
		r.Message += " Новым участникам отправлены приглашения; ожидаем их ответы."
	}
	if removed > 0 {
		r.Message += " Приглашения исключённых участников отменены."
	}
	if added == 0 && removed == 0 {
		r.Message = "Указанные друзья уже участники этой встречи. Их ответы и Zoom-ссылка сохранены."
	}
	if !availability.Complete {
		r.Message += " Доступность внешних календарей проверена не полностью."
	}
	return r, nil
}
