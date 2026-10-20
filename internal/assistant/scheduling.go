package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/nlu"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/social"
	"github.com/google/uuid"
)

type choiceKey struct{}

func WithChatChoice(ctx context.Context, choice *domain.ChatChoice) context.Context {
	return context.WithValue(ctx, choiceKey{}, choice)
}
func chatChoice(ctx context.Context) *domain.ChatChoice {
	v, _ := ctx.Value(choiceKey{}).(*domain.ChatChoice)
	return v
}
func sameChoice(a, b *domain.ChatChoice) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// Button payloads only reference the server's persisted offer. They never pass
// through the language model and cannot supply arbitrary dates or meeting IDs.
func choiceOperation(p *domain.Pending, choice *domain.ChatChoice) (nlu.Operation, error) {
	op := nlu.Operation{Intent: "choose_time", Confidence: "high"}
	if p == nil || p.Scheduling == nil || choice.Token != p.Scheduling.Token {
		return op, domain.Validation("Этот выбор уже устарел. Запросите новые варианты.")
	}
	a := nlu.SchedulingReply{}
	switch choice.Action {
	case "cancel":
		op.Intent = "cancel"
	case "more":
		a.More = true
	case "new":
		a.NewMeeting = true
	case "confirm":
		a.Confirm = true
	case "meeting":
		if choice.Index < 1 || choice.Index > len(p.Scheduling.Meetings) {
			return op, domain.Validation("Выберите встречу из списка")
		}
		a.Option = choice.Index
	case "slot":
		if choice.Index < 1 || choice.Index > len(p.Scheduling.Slots) || choice.Slot == nil {
			return op, domain.Validation("Выберите предложенное время")
		}
		slot := p.Scheduling.Slots[choice.Index-1]
		if !slot.Start.Equal(choice.Slot.Start) || !slot.End.Equal(choice.Slot.End) {
			return op, domain.Validation("Время не совпадает с предложенным")
		}
		a.Option = choice.Index
	default:
		return op, domain.Validation("Неизвестный выбор")
	}
	op.Arguments, _ = json.Marshal(a)
	if op.Intent == "cancel" {
		op.Arguments = json.RawMessage(`{}`)
	}
	return op, nil
}

func socialContinuation(p *domain.Pending, op nlu.Operation, args any, now time.Time) (nlu.Operation, any, error) {
	if p == nil || len(p.SocialOperation) == 0 {
		return op, args, nil
	}
	var original nlu.Operation
	if json.Unmarshal(p.SocialOperation, &original) != nil {
		return op, args, nil
	}
	base, err := original.Args()
	if err != nil {
		return op, args, err
	}
	c, ok := base.(*nlu.Create)
	if !ok {
		return op, args, nil
	}
	var changes nlu.Changes
	option := 0
	more, newMeeting, confirm := false, false, false
	switch a := args.(type) {
	case *nlu.Continue:
		changes = a.Changes
	case *nlu.SchedulingReply:
		changes = a.Changes
		option = a.Option
		more = a.More
		newMeeting = a.NewMeeting
		confirm = a.Confirm
	case *nlu.Mutation:
		if op.Intent != "select" || a.Target.Type != "ordinal" {
			return op, args, nil
		}
		option = a.Target.Index
	default:
		if op.Intent == "confirm" {
			confirm = true
		} else {
			return op, args, nil
		}
	}
	q := p.Scheduling
	if q == nil {
		q = &domain.SchedulingPending{}
		p.Scheduling = q
	}
	if newMeeting {
		if len(q.Meetings) == 0 && q.Selected == nil {
			return op, args, domain.Validation("Сначала выберите существующую встречу или уточните запрос")
		}
		q.ConfirmedNew = true
		q.Selected = nil
		q.Meetings = nil
	}
	if confirm && q.Selected == nil {
		return op, args, domain.Validation("Сначала выберите встречу")
	}
	if more {
		if len(q.Slots) == 0 {
			return op, args, domain.Validation("Сначала запросите свободное время")
		}
		after := q.Slots[len(q.Slots)-1].End
		q.After = &after
		q.Slots = nil
	}
	if option != 0 {
		count := len(q.Slots)
		if len(q.Meetings) > 0 {
			count = len(q.Meetings)
		}
		if option == -1 {
			option = count
		}
		if option < 1 || option > count {
			return op, args, domain.Validation("Выберите вариант из предложенного списка")
		}
		if len(q.Meetings) > 0 {
			e := q.Meetings[option-1]
			q.Selected = &e
			q.Meetings = nil
		} else {
			slot := q.Slots[option-1]
			if slot.Start.Before(now) {
				return op, args, domain.Validation("Это время уже прошло. Запросите новые варианты.")
			}
			c.Date = &nlu.Temporal{Type: "absolute", Date: domain.Day(slot.Start.In(now.Location())).String()}
			clock := slot.Start.In(now.Location()).Format("15:04")
			c.Time = &clock
			q.After = nil
			q.Slots = nil
		}
	}
	if changes.Date != nil {
		q.Selected = nil
		q.Meetings = nil
		c.Date = changes.Date
		q.After = nil
		q.Slots = nil
	}
	if changes.Time != nil {
		q.Selected = nil
		q.Meetings = nil
		c.Time = changes.Time
		q.After = nil
		q.Slots = nil
	}
	if changes.Title != nil {
		c.Title = *changes.Title
	}
	if changes.ReminderMinutes != nil {
		c.ReminderMinutes = changes.ReminderMinutes
	}
	if changes.ReminderEnabled != nil {
		c.ReminderEnabled = changes.ReminderEnabled
	}
	original.Arguments, _ = json.Marshal(c)
	return original, c, nil
}

func (s *Service) createSocialMeeting(ctx context.Context, tx repository.Tx, state *domain.State, prior *domain.Pending, session *social.Session, op nlu.Operation, a *nlu.Create, now time.Time) (domain.Response, error) {
	q := &domain.SchedulingPending{}
	if prior != nil && prior.Scheduling != nil {
		q = prior.Scheduling
	}
	question := func(text string) (domain.Response, error) {
		raw, _ := json.Marshal(op)
		state.Pending = &domain.Pending{Command: neutral("simple_llm"), Expectation: "social", At: now, SocialOperation: raw, Scheduling: q}
		return answer(text, domain.ActionClarify), nil
	}
	card := func(text, kind string, complete bool) (domain.Response, error) {
		q.Token = uuid.NewString()
		r, _ := question(text)
		r.Scheduling = &domain.SchedulingCard{Token: q.Token, Kind: kind, Title: a.Title, Slots: q.Slots, Requested: q.Requested, Meetings: q.Meetings, Complete: complete, Timezone: s.Calendar.Location.String(), Today: domain.Day(now).String()}
		return r, nil
	}
	var err error
	c := neutral("simple_llm")
	c.Action = domain.ActionCreate
	c.Kind = domain.EventMeeting
	if err = simpleDate(&c, a.Date, a.Time, now); err != nil {
		return question("Уточните дату и время встречи.")
	}
	ids, err := session.ResolveInvitees(ctx, a.Friends, a.FriendTag, a.AllWithTag)
	if err != nil {
		// An invitation remains accessible after a friendship ends. Resolve it
		// using the actual participants rather than global account search.
		if c.Date != nil && a.FriendTag == "" {
			matches, e := session.MatchingMeetings(ctx, tx, "", a.Friends, nil, c.Date, c.Time)
			if e != nil {
				return domain.Response{}, e
			}
			if len(matches) == 1 {
				return s.useExistingMeeting(ctx, tx, state, &matches[0], a.Conference, now)
			}
		}
		return question(err.Error())
	}
	if q.Selected != nil {
		var m *social.Meeting
		if q.Selected.Collaborative {
			m, err = session.Meeting(ctx, tx, q.Selected.ID)
		} else {
			var e *domain.Event
			e, err = tx.Get(ctx, q.Selected.ID)
			if e != nil {
				m = &social.Meeting{Event: *e, Organizer: social.Person{ID: e.OrganizerID, Name: e.OrganizerName}}
			}
		}
		if err != nil {
			return question(err.Error())
		}
		if m == nil || m.Cancelled || !sameRecord(*q.Selected, m.Event) {
			q.Selected = nil
			return question("Встреча изменилась. Повторите поиск.")
		}
		return s.useExistingMeeting(ctx, tx, state, m, a.Conference, now)
	}
	// Match before asking for a missing clock or attempting external APIs.
	if (c.Date != nil || a.Conference == "zoom") && !q.ConfirmedNew && (len(ids) > 0 || !domain.GenericMeetingTitle(a.Title) || a.Conference == "zoom") {
		query := a.Title
		if len(ids) > 0 && (domain.GenericMeetingTitle(query) || strings.HasPrefix(strings.ToLower(query), "встреча с ")) {
			query = ""
		}
		matches, e := session.MatchingMeetings(ctx, tx, query, nil, ids, c.Date, c.Time)
		if e != nil {
			return domain.Response{}, e
		}
		if len(matches) > 0 {
			if len(matches) == 1 {
				m := matches[0]
				if m.Organizer.ID != session.UserID || a.Conference == "" || m.Event.MeetingURL != "" {
					return s.useExistingMeeting(ctx, tx, state, &m, a.Conference, now)
				}
				q.Selected = &m.Event
				q.Meetings = []domain.Event{m.Event}
				return card(fmt.Sprintf("У вас уже есть %s. Организатор: %s. Добавить Zoom к этой встрече?", meetingDescription(m.Event), m.Organizer.Name), "existing", true)
			}
			q.Meetings = nil
			for _, m := range matches {
				if len(q.Meetings) < 8 {
					q.Meetings = append(q.Meetings, m.Event)
				}
			}
			return card("Нашёл несколько подходящих встреч. Выберите нужную или уточните дату и название.", "meetings", true)
		}
	}
	if domain.GenericMeetingTitle(a.Title) {
		if len(ids) == 0 {
			return question("С кем встреча или по какому поводу?")
		}
	}
	if len(ids) > 0 {
		people, e := session.People(ctx)
		if e != nil {
			return domain.Response{}, e
		}
		names := []string{}
		for _, id := range ids {
			for _, f := range people.Friends {
				if f.ID == id {
					names = append(names, f.Name)
				}
			}
		}
		// Person-only titles always use the actual account names, never a private tag.
		if domain.GenericMeetingTitle(a.Title) || strings.HasPrefix(strings.ToLower(a.Title), "встреча с ") || a.FriendTag != "" {
			a.Title = domain.MeetingDisplayTitle("Встреча", names)
			op.Arguments, _ = json.Marshal(a)
		}
	}
	if c.Date == nil {
		return question("В какой день назначить встречу?")
	}
	if c.Time == nil {
		return question("Во сколько назначить встречу?")
	}
	duration := 60
	if a.DurationMinutes != nil {
		duration = *a.DurationMinutes
	}
	if duration < 5 || duration > 1440 {
		return question("Длительность должна быть от 5 минут до суток.")
	}
	start := c.Date.At(*c.Time, s.Calendar.Location)
	end := start.Add(time.Duration(duration) * time.Minute)
	if q.Requested == nil {
		q.Requested = &domain.PlanningSlot{Start: start, End: end}
	}
	searchStart := start
	if q.After != nil {
		searchStart = *q.After
	}
	if searchStart.Before(now) {
		searchStart = now.Truncate(15 * time.Minute).Add(15 * time.Minute)
	}
	av, err := session.Availability(ctx, ids, social.Slot{Start: searchStart, End: searchStart.Add(end.Sub(start))}, 0, now)
	if err != nil {
		return question(err.Error())
	}
	busy := start.Before(now) || q.After != nil
	for _, u := range av.Users {
		busy = busy || u.Status == "busy"
	}
	if busy {
		q.Slots = nil
		for _, slot := range av.Slots {
			if len(q.Slots) >= 4 {
				break
			}
			q.Slots = append(q.Slots, domain.PlanningSlot{Start: slot.Start, End: slot.End})
		}
		text := "В это время у вас или участников уже есть планы. Нашёл ближайшие свободные варианты."
		if start.Before(now) {
			text = "Это время уже прошло. Нашёл ближайшие свободные варианты."
		}
		if q.After != nil {
			text = "Вот следующие свободные варианты."
		}
		if len(q.Slots) == 0 {
			text = "Общего свободного времени в ближайшие 14 дней не нашёл. Попробуйте другую дату или длительность."
		} else if domain.Day(q.Slots[0].Start.In(s.Calendar.Location)).String() != c.Date.String() {
			text += " На выбранную дату вариантов больше нет; показываю следующие дни."
		} else if domain.Day(q.Slots[len(q.Slots)-1].Start.In(s.Calendar.Location)).String() != c.Date.String() {
			text += " После вариантов на выбранную дату показываю свободное время следующих дней."
		}
		if !av.Complete {
			text += " Доступность внешних календарей проверена не полностью; варианты предварительные."
		}
		return card(text, "slots", av.Complete)
	}
	for _, provider := range []string{a.Calendar, a.Conference} {
		if provider == "" {
			continue
		}
		if provider != "google" && provider != "zoom" {
			return question("Выберите Google Calendar или Zoom.")
		}
		if s.Integrations == nil {
			return question("Подключите " + provider + " в разделе «Интеграции».")
		}
		if err = s.Integrations.Ready(ctx, provider); err != nil {
			return question(err.Error())
		}
	}
	c.Title = a.Title
	c.Duration = end.Sub(start)
	c.CalendarProvider = a.Calendar
	c.ConferenceProvider = a.Conference
	c.ReminderEnabled = a.ReminderEnabled
	c.ReminderMinutes = a.ReminderMinutes
	if len(ids) == 0 {
		state.Pending = nil
		return s.executeValidated(ctx, tx, state, c, now, false)
	}
	event := domain.Event{Title: a.Title, Kind: domain.EventMeeting, Date: c.Date, StartTime: c.Time, Duration: c.Duration, Status: domain.StatusOpen, Priority: domain.PriorityNormal}
	event.CalendarProvider = a.Calendar
	event.ConferenceProvider = a.Conference
	event.ReminderEnabled = a.ReminderEnabled
	event.ReminderMinutes = a.ReminderMinutes
	result, err := session.Execute(ctx, social.Operation{RequestID: uuid.NewString(), Action: "create_meeting", Event: &event, FriendIDs: ids}, now)
	if err != nil {
		return question(err.Error())
	}
	created, err := tx.Get(ctx, result.EventID)
	if err != nil {
		return domain.Response{}, err
	}
	state.Pending = nil
	r := answer(result.Message, domain.ActionCreate)
	r.Social = &domain.SocialNotice{Kind: "meeting", EventID: result.EventID}
	if created != nil {
		r.Events = []domain.Event{*created}
	}
	return r, nil
}

func meetingDescription(e domain.Event) string {
	return fmt.Sprintf("«%s» %s в %s", e.Label(), e.Date, e.StartTime)
}
func (s *Service) useExistingMeeting(ctx context.Context, tx repository.Tx, state *domain.State, m *social.Meeting, conference string, now time.Time) (domain.Response, error) {
	state.Pending = nil
	r := answer(fmt.Sprintf("У вас уже есть %s. Организатор: %s.", meetingDescription(m.Event), m.Organizer.Name), domain.ActionList)
	r.Social = &domain.SocialNotice{Kind: "meeting", EventID: m.Event.ID}
	r.Events = []domain.Event{m.Event}
	if conference != "zoom" {
		return r, nil
	}
	if m.Event.MeetingURL != "" {
		r.Reply += " Ссылка для звонка уже добавлена."
		return r, nil
	}
	if m.Organizer.ID != repository.UserID(ctx) {
		r.Reply += " Добавить Zoom может только организатор. Обратитесь к нему; перенос можно предложить в приглашении."
		return r, nil
	}
	if s.Integrations == nil {
		r.Reply += " Подключите Zoom в разделе «Интеграции»."
		return r, nil
	}
	if err := s.Integrations.Ready(ctx, "zoom"); err != nil {
		r.Reply += " " + err.Error()
		return r, nil
	}
	e := m.Event
	e.ConferenceProvider = "zoom"
	updated, err := tx.Update(ctx, e, now)
	if err != nil {
		return domain.Response{}, err
	}
	r.Events = []domain.Event{*updated}
	r.Action = domain.ActionUpdate
	r.Reply += " Создаю Zoom для этой встречи."
	return r, nil
}
