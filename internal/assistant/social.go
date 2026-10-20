package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/social"
	"github.com/google/uuid"
)

func (s *Service) applySocial(ctx context.Context, tx repository.Tx, state *domain.State, op nlu.Operation, args any, now time.Time) (bool, domain.Response, error) {
	prior := state.Pending
	continuing := in(op.Intent, "choose_time", "continue", "select", "confirm")
	var continuationErr error
	op, args, continuationErr = socialContinuation(prior, op, args, now)
	if continuationErr != nil {
		return true, answer(continuationErr.Error(), domain.ActionClarify), nil
	}
	if op.Intent == "meeting_zoom" {
		a := args.(*nlu.Social)
		args = &nlu.Create{Friends: a.Friends, FriendTag: a.Tag, Title: a.Query, Date: a.Date, Conference: "zoom"}
		op.Intent = "create_meeting"
		op.Arguments, _ = json.Marshal(args)
	}
	a, isSocial := args.(*nlu.Social)
	create, isCreate := args.(*nlu.Create)
	isCreate = isCreate && op.Intent == "create_meeting"
	if !isSocial && !isCreate {
		return false, domain.Response{}, nil
	}
	session, err := social.In(ctx, tx, s.Calendar.Location)
	if err != nil {
		if isCreate && len(create.Friends) == 0 && create.FriendTag == "" {
			return false, domain.Response{}, nil
		}
		return true, answer(err.Error(), domain.ActionClarify), nil
	}
	ctx = social.WithTx(ctx, tx)
	state.Pending = nil
	question := func(text string) (bool, domain.Response, error) {
		raw, _ := json.Marshal(op)
		state.Pending = &domain.Pending{Command: neutral("simple_llm"), Expectation: "social", At: now, SocialOperation: raw}
		return true, answer(text, domain.ActionClarify), nil
	}
	finish := func(result social.Result, err error) (bool, domain.Response, error) {
		if err != nil {
			var app *domain.AppError
			if errors.As(err, &app) {
				return question(app.Message)
			}
			return true, domain.Response{}, err
		}
		r := answer(result.Message, domain.ActionList)
		if result.EventID > 0 {
			r.Social = &domain.SocialNotice{Kind: "meeting", EventID: result.EventID}
		} else {
			r.Social = &domain.SocialNotice{Kind: "friends"}
		}
		return true, r, nil
	}
	operation := social.Operation{RequestID: uuid.NewString()}
	if isCreate {
		if !continuing {
			prior = nil
		}
		r, e := s.createSocialMeeting(ctx, tx, state, prior, session, op, create, now)
		return true, r, e
	}

	switch op.Intent {
	case "friend_request":
		if e := session.Rate(ctx, "codes", 20); e != nil {
			return finish(social.Result{}, e)
		}
		operation.Action, operation.Code = "request", a.Code
	case "friend_requests":
		p, e := session.People(ctx)
		if e != nil {
			return finish(social.Result{}, e)
		}
		lines := []string{fmt.Sprintf("Входящих заявок: %d. Отправленных: %d.", len(p.Incoming), len(p.Outgoing))}
		for _, r := range p.Incoming {
			lines = append(lines, "Входящая: "+r.Person.Name)
		}
		for _, r := range p.Outgoing {
			lines = append(lines, "Отправлена: "+r.Person.Name)
		}
		return finish(social.Result{Message: strings.Join(lines, "\n")}, nil)
	case "friend_availability":
		if e := session.Rate(ctx, "availability", 120); e != nil {
			return finish(social.Result{}, e)
		}
		ids, e := session.Resolve(ctx, a.Friends, a.Tag)
		if e != nil {
			return question(e.Error())
		}
		if len(ids) == 0 {
			return question("Для кого найти свободное время? Назовите друга или тег.")
		}
		c := neutral("simple_llm")
		if e = simpleDate(&c, a.Date, a.Time, now); e != nil {
			return question("Уточните день и время.")
		}
		start := now.Add(time.Hour).Truncate(time.Minute)
		var periodEnd *domain.Date
		if a.Period != nil {
			d, end, _, e := a.Period.Resolve(now)
			if e != nil {
				return question("Уточните период поиска.")
			}
			periodEnd = end
			start = d.At(domain.TimeOfDay{Hour: 9}, s.Calendar.Location)
			if start.Before(now) {
				start = now.Add(time.Hour).Truncate(time.Minute)
			}
		} else if c.Date != nil {
			clock := domain.TimeOfDay{Hour: 9}
			if c.Time != nil {
				clock = *c.Time
			}
			start = c.Date.At(clock, s.Calendar.Location)
		}
		if start.Before(now) {
			start = now.Truncate(15 * time.Minute).Add(15 * time.Minute)
		}
		duration := 30
		if a.DurationMinutes != nil {
			duration = *a.DurationMinutes
		}
		av, e := session.Availability(ctx, ids, social.Slot{Start: start, End: start.Add(time.Duration(duration) * time.Minute)}, 0, now)
		if e != nil {
			return question(e.Error())
		}
		if periodEnd != nil {
			slots := []social.Slot{}
			for _, slot := range av.Slots {
				if slot.Start.Before(periodEnd.At(domain.TimeOfDay{}, s.Calendar.Location)) {
					slots = append(slots, slot)
				}
			}
			av.Slots = slots
		}
		r, e := s.planningPreview(ctx, state, session, a, ids, av, start, duration, periodEnd, now)
		return true, r, e
	default:
		var date *domain.Date
		if a.Date != nil {
			c := neutral("simple_llm")
			if e := simpleDate(&c, a.Date, nil, now); e != nil {
				return question("Уточните дату исходной встречи.")
			}
			date = c.Date
		}
		names := a.Friends
		if op.Intent == "invite_friends" {
			names = nil
		}
		m, e := session.FindMeeting(ctx, tx, a.Query, names, date)
		if e != nil {
			return question(e.Error())
		}
		operation.EventID, operation.Version = m.Event.ID, m.Version
		switch op.Intent {
		case "invite_friends":
			ids, e := session.Resolve(ctx, a.Friends, a.Tag)
			if e != nil {
				return question(e.Error())
			}
			operation.Action, operation.FriendIDs = "invite", ids
		case "respond_invitation":
			operation.Action, operation.Status = "respond", a.Status
		case "propose_reschedule":
			operation.Action = "propose"
			for _, option := range a.Options {
				c := neutral("simple_llm")
				if e := simpleDate(&c, option.Date, option.Time, now); e != nil || c.Date == nil || c.Time == nil {
					return question("Укажите новый день и время, например: завтра в 16:30.")
				}
				start := c.Date.At(*c.Time, s.Calendar.Location)
				operation.Slots = append(operation.Slots, social.Slot{Start: start, End: start.Add(m.Event.Duration)})
			}
		case "resolve_reschedule":
			operation.Action, operation.Status, operation.SlotIndex = "resolve_proposal", a.Status, max(0, a.Option-1)
			for _, p := range m.Proposals {
				if p.Status == "pending" {
					operation.ID = p.ID
					break
				}
			}
			if operation.ID == "" {
				return question("Нет ожидающего предложения переноса.")
			}
		}
	}
	result, e := session.Execute(ctx, operation, now)
	return finish(result, e)
}

// Prepare external availability outside the write transaction. Resolution here
// is read-only; execution resolves and authorizes the names again under the lock.
func (s *Service) prepareSimplePlanning(ctx context.Context, op nlu.Operation, now time.Time, after *time.Time) {
	args, err := op.Args()
	if err != nil {
		return
	}
	ids := []string{}
	requested := false
	start := now.Add(time.Hour).Truncate(time.Minute)
	err = s.Store.Read(ctx, func(tx repository.Tx) error {
		session, e := social.In(ctx, tx, s.Calendar.Location)
		if e != nil {
			return e
		}
		switch a := args.(type) {
		case *nlu.Create:
			if op.Intent != "create_meeting" {
				return nil
			}
			requested = true
			ids, e = session.Resolve(ctx, a.Friends, a.FriendTag)
			if e != nil {
				return e
			}
			c := neutral("simple_llm")
			if simpleDate(&c, a.Date, a.Time, now) == nil && c.Date != nil {
				clock := domain.TimeOfDay{Hour: 9}
				if c.Time != nil {
					clock = *c.Time
				}
				start = c.Date.At(clock, s.Calendar.Location)
			}
		case *nlu.Social:
			if op.Intent == "friend_request" || op.Intent == "friend_requests" {
				return nil
			}
			requested = true
			ids, e = session.Resolve(ctx, a.Friends, a.Tag)
			if e != nil {
				return e
			}
			if a.Date != nil || a.Period != nil {
				t := a.Date
				if t == nil {
					t = a.Period
				}
				date, _, _, err := t.Resolve(now)
				if err == nil {
					clock := domain.TimeOfDay{Hour: 9}
					if a.Time != nil {
						if c, e := domain.ParseTime(*a.Time); e == nil {
							clock = *c
						}
					}
					start = date.At(clock, s.Calendar.Location)
					if start.Before(now) {
						start = now.Add(time.Hour).Truncate(time.Minute)
					}
				}
			}
			if op.Intent != "friend_availability" {
				m, e := session.FindMeeting(ctx, tx, a.Query, nil, nil)
				if e != nil {
					return e
				}
				ids = append(ids, m.Organizer.ID)
				for _, p := range m.Members {
					if p.Status != "cancelled" {
						ids = append(ids, p.ID)
					}
				}
				if op.Intent == "resolve_reschedule" {
					for _, p := range m.Proposals {
						if p.Status == "pending" && len(p.Slots) > 0 {
							start = p.Slots[min(max(0, a.Option-1), len(p.Slots)-1)].Start
							break
						}
					}
				} else {
					start, _, _ = m.Event.Interval(s.Calendar.Location)
				}
			}
		}
		return nil
	})
	if after != nil {
		start = *after
	}
	if err == nil && requested {
		s.RefreshPlanning(ctx, append([]string{repository.UserID(ctx)}, ids...), start.Add(-24*time.Hour), start.AddDate(0, 0, 14))
	}
}
