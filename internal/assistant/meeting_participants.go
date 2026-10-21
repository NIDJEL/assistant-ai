package assistant

import (
	"context"
	"errors"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/social"
	"github.com/google/uuid"
)

func (s *Service) setMeetingParticipants(ctx context.Context, tx repository.Tx, state *domain.State, a *nlu.MeetingParticipants, now time.Time) (domain.Response, error) {
	pending := state.Pending
	state.Pending = nil
	clarify := func(text string) (domain.Response, error) { return answer(text, domain.ActionClarify), nil }
	c := neutral("simple_llm")
	c.Action, c.Kind = domain.ActionUpdate, domain.EventMeeting
	if err := s.simpleTarget(ctx, tx, state, pending, a.Target, &c, now); err != nil {
		return clarify(err.Error())
	}
	if c.Kind != domain.EventUnknown && c.Kind != domain.EventMeeting {
		return clarify("Участников можно изменить только у встречи.")
	}
	c.Kind = domain.EventMeeting
	items, certain, err := s.mutationCandidates(ctx, tx, state, c)
	if err != nil {
		return domain.Response{}, err
	}
	if len(items) != 1 || !certain {
		return clarify("Уточните название и дату встречи, у которой нужно изменить участников.")
	}
	event := items[0]
	if event.OrganizerID != repository.UserID(ctx) {
		return clarify("Только организатор может изменять участников этой встречи.")
	}
	session, err := social.In(ctx, tx, s.Calendar.Location)
	if err != nil {
		return clarify(err.Error())
	}
	ids, err := session.ResolveInvitees(ctx, a.Friends, a.FriendTag, false)
	if err != nil {
		return clarify(err.Error())
	}
	result, err := session.Execute(social.WithTx(ctx, tx), social.Operation{RequestID: uuid.NewString(), Action: "set_participants", EventID: event.ID, Version: event.MeetingVersion, FriendIDs: ids}, now)
	if err != nil {
		var app *domain.AppError
		if errors.As(err, &app) {
			return clarify(app.Message)
		}
		return domain.Response{}, err
	}
	updated, err := tx.Get(ctx, event.ID)
	if err != nil {
		return domain.Response{}, err
	}
	if updated == nil {
		return domain.Response{}, errors.New("updated meeting unavailable")
	}
	r := answer(result.Message, domain.ActionUpdate)
	r.Events = []domain.Event{*updated}
	r.Social = &domain.SocialNotice{Kind: "meeting", EventID: event.ID}
	return r, nil
}
