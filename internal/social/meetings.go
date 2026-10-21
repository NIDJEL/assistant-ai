package social

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/google/uuid"
)

func (s *Session) Meetings(ctx context.Context, tx repository.Tx) ([]Meeting, error) {
	rows, err := s.Q.QueryContext(ctx, `SELECT cm.event_id FROM collaborative_meetings cm JOIN events e ON e.id=cm.event_id WHERE e.user_id=$1 OR EXISTS(SELECT 1 FROM meeting_participants p WHERE p.event_id=e.id AND p.user_id=$1) ORDER BY e.date DESC,e.time DESC`, s.UserID)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	result := []Meeting{}
	for _, id := range ids {
		m, e := s.Meeting(ctx, tx, id)
		if e != nil {
			return nil, e
		}
		result = append(result, *m)
	}
	return result, nil
}
func (s *Session) Meeting(ctx context.Context, tx repository.Tx, id int64) (*Meeting, error) {
	m := &Meeting{Members: []Member{}, Proposals: []Proposal{}}
	var snapshot string
	if err := s.Q.QueryRowContext(ctx, `SELECT cancelled_snapshot FROM meeting_participants WHERE event_id=$1 AND user_id=$2 AND status='cancelled' AND cancelled_snapshot IS NOT NULL`, id, s.UserID).Scan(&snapshot); err == nil {
		if err = json.Unmarshal([]byte(snapshot), m); err != nil {
			return nil, err
		}
		m.Event.EventDetails = domain.EventDetails{}
		m.displayTitle(s.UserID)
		return m, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var raw string
	err := s.Q.QueryRowContext(ctx, `SELECT cm.version,cm.cancelled,cm.timezone,e.user_id,u.display_name,u.avatar_url,CASE WHEN u.share_friend_email THEN coalesce(u.email,'') ELSE '' END,(CASE WHEN e.user_id=$2 THEN e.details::jsonb ELSE '{}'::jsonb END) || jsonb_build_object('id',e.id,'kind',e.kind,'title',e.title,'date',e.date,'time',e.time,'duration_minutes',e.duration_minutes,'status',e.status,'description',e.description,'location',e.location,'meeting_url',e.meeting_url,'priority',e.priority,'created_at',e.created_at,'updated_at',e.updated_at,'organizer_id',e.user_id,'collaborative',true,'meeting_version',cm.version) FROM collaborative_meetings cm JOIN events e ON e.id=cm.event_id JOIN users u ON u.id=e.user_id WHERE e.id=$1 AND (e.user_id=$2 OR EXISTS(SELECT 1 FROM meeting_participants p WHERE p.event_id=e.id AND p.user_id=$2))`, id, s.UserID).Scan(&m.Version, &m.Cancelled, &m.Timezone, &m.Organizer.ID, &m.Organizer.Name, &m.Organizer.AvatarURL, &m.Organizer.Email, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, notFound()
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(raw), &m.Event); err != nil {
		return nil, err
	}
	m.Organizer.Tags = []string{}
	rows, err := s.Q.QueryContext(ctx, `SELECT u.id,u.display_name,u.avatar_url,p.status,p.version FROM meeting_participants p JOIN users u ON u.id=p.user_id WHERE p.event_id=$1 ORDER BY p.invited_at,u.id`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var member Member
		member.Tags = []string{}
		if err = rows.Scan(&member.ID, &member.Name, &member.AvatarURL, &member.Status, &member.Version); err != nil {
			rows.Close()
			return nil, err
		}
		m.Members = append(m.Members, member)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = s.Q.QueryContext(ctx, `SELECT p.id,p.proposer_id,u.display_name,p.version,p.slots,p.status FROM reschedule_proposals p JOIN users u ON u.id=p.proposer_id WHERE p.event_id=$1 ORDER BY p.created_at DESC LIMIT 30`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p Proposal
		var slots string
		if err = rows.Scan(&p.ID, &p.ProposerID, &p.Name, &p.Version, &slots, &p.Status); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(slots), &p.Slots); err != nil {
			return nil, err
		}
		m.Proposals = append(m.Proposals, p)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	m.displayTitle(s.UserID)
	if domain.ValidZoomJoinURL(m.Event.MeetingURL) {
		m.Event.ConferenceProvider = "zoom"
	}
	return m, nil
}

func (m *Meeting) displayTitle(viewer string) {
	m.Event.OrganizerName = m.Organizer.Name
	m.Event.OrganizerAvatarURL = m.Organizer.AvatarURL
	names := []string{}
	if m.Organizer.ID != viewer {
		names = append(names, m.Organizer.Name)
	} else {
		for _, p := range m.Members {
			if p.Status != "cancelled" {
				names = append(names, p.Name)
			}
		}
	}
	m.Event.DisplayTitle = domain.MeetingDisplayTitle(m.Event.Title, names)
}
func (s *Session) meetingOperation(ctx context.Context, op Operation, now time.Time) (Result, error) {
	r := Result{EventID: op.EventID}
	tx := eventTx(ctx)
	if tx == nil {
		return r, errors.New("calendar transaction required")
	}
	if op.Action == "create_meeting" {
		if op.Event == nil {
			return r, domain.Validation("Укажите встречу")
		}
		e := *op.Event
		e.Kind = domain.EventMeeting
		e.Status = domain.StatusOpen
		if e.Priority == domain.PriorityUnknown {
			e.Priority = domain.PriorityNormal
		}
		if e.Duration == 0 {
			e.Duration = time.Hour
		}
		e.Recurrence = nil
		e.ID = 0
		e.SourceKey = nil
		e.ExternalCalendarID = nil
		if err := e.Validate(); err != nil {
			return r, domain.Validation(err.Error())
		}
		if len(op.FriendIDs) == 0 {
			return r, domain.Validation("Выберите друзей")
		}
		if err := s.checkInvitees(ctx, op.FriendIDs); err != nil {
			return r, err
		}
		start, end, _ := e.Interval(s.Location)
		a, err := s.Availability(ctx, op.FriendIDs, Slot{start, end}, 0, now)
		if err != nil {
			return r, err
		}
		if anyBusy(a) {
			return r, problem(409, "availability_conflict", "Выбранное время занято. Проверьте доступность и выберите свободный интервал.")
		}
		e.Participants = nil
		created, err := tx.Create(ctx, e, now)
		if err != nil {
			return r, err
		}
		r.EventID = created.ID
		_, err = s.Q.ExecContext(ctx, `INSERT INTO collaborative_meetings(event_id,timezone) VALUES($1,$2)`, created.ID, s.Location.String())
		if err != nil {
			return r, err
		}
		if err = s.invite(ctx, *created, op.FriendIDs, 1, now); err != nil {
			return r, err
		}
		r.Message = "Встреча создана. Друзьям отправлены приглашения; ожидаем их ответы."
		if !a.Complete {
			r.Message += " Доступность внешних календарей проверена не полностью."
		}
		return r, nil
	}
	if op.Action == "invite" {
		e, e2 := tx.Get(ctx, op.EventID)
		if e2 != nil {
			return r, e2
		}
		if e == nil || e.OrganizerID != s.UserID {
			return r, notFound()
		}
		if !e.Collaborative {
			if err := s.checkInvitees(ctx, op.FriendIDs); err != nil {
				return r, err
			}
			_, err := s.Q.ExecContext(ctx, `INSERT INTO collaborative_meetings(event_id,timezone) VALUES($1,$2) ON CONFLICT DO NOTHING`, e.ID, s.Location.String())
			if err != nil {
				return r, err
			}
		}
	}
	m, err := s.Meeting(ctx, tx, op.EventID)
	if err != nil {
		return r, err
	}
	if m.Cancelled {
		return r, problem(409, "meeting_cancelled", "Встреча отменена")
	}
	organizer := m.Organizer.ID == s.UserID
	if op.Version != m.Version {
		return r, problem(409, "stale_invitation", "Встреча изменилась. Обновите приглашение и ответьте ещё раз.")
	}
	switch op.Action {
	case "invite":
		if !organizer {
			return r, notFound()
		}
		if len(op.FriendIDs) == 0 {
			return r, domain.Validation("Выберите друзей")
		}
		if err = s.checkInvitees(ctx, op.FriendIDs); err != nil {
			return r, err
		}
		start, end, _ := m.Event.Interval(s.Location)
		a, e := s.Availability(ctx, op.FriendIDs, Slot{start, end}, m.Event.ID, now)
		if e != nil {
			return r, e
		}
		if anyBusy(a) {
			return r, problem(409, "availability_conflict", "Друг занят. Выберите другое время.")
		}
		err = s.invite(ctx, m.Event, op.FriendIDs, m.Version, now)
		r.Message = "Приглашения отправлены"
		if !a.Complete {
			r.Message += " Доступность внешних календарей проверена не полностью."
		}
	case "respond":
		if op.UserID != "" && op.UserID != s.UserID {
			return r, problem(403, "own_response_only", "Можно отвечать только на собственное приглашение")
		}
		if organizer || !containsStatus(op.Status, "accepted", "declined") {
			return r, domain.Validation("Выберите «Приду» или «Не приду»")
		}
		var previous string
		var revision int
		err = s.Q.QueryRowContext(ctx, `SELECT status,response_revision FROM meeting_participants WHERE event_id=$1 AND user_id=$2`, m.Event.ID, s.UserID).Scan(&previous, &revision)
		if err != nil {
			return r, notFound()
		}
		if previous == "cancelled" {
			return r, notFound()
		}
		if previous == op.Status {
			r.Message = "Ответ уже сохранён"
			return r, nil
		}
		if op.Status == "accepted" {
			start, end, _ := m.Event.Interval(s.Location)
			a, e := s.Availability(ctx, []string{}, Slot{start, end}, m.Event.ID, now)
			if e != nil {
				return r, e
			}
			if anyBusy(a) {
				return r, problem(409, "availability_conflict", "У вас уже есть планы на это время. Предложите перенос.")
			}
		}
		revision++
		_, err = s.Q.ExecContext(ctx, `UPDATE meeting_participants SET status=$3,responded_at=$4,response_revision=$5 WHERE event_id=$1 AND user_id=$2`, m.Event.ID, s.UserID, op.Status, now, revision)
		if err != nil {
			return r, err
		}
		_, err = s.Q.ExecContext(ctx, `INSERT INTO invitation_responses(event_id,user_id,version,revision,status,created_at) VALUES($1,$2,$3,$4,$5,$6)`, m.Event.ID, s.UserID, m.Version, revision, op.Status, now)
		if err != nil {
			return r, err
		}
		name, e := s.name(ctx, s.UserID)
		if e != nil {
			return r, e
		}
		verb := "подтвердил(а) участие во встрече "
		if op.Status == "declined" {
			verb = "отклонил(а) приглашение на встречу "
		}
		err = s.notify(ctx, m.Organizer.ID, fmt.Sprintf("response:%d:%s:%d:%d", m.Event.ID, s.UserID, m.Version, revision), name+" "+verb+meetingLabel(m.Event), "meeting", m.Event.ID, now)
		r.Message = map[string]string{"accepted": "Вы подтвердили участие. Встреча добавлена в ваш календарь.", "declined": "Вы отказались от участия"}[op.Status]
	case "propose":
		if !organizer {
			allowed := false
			for _, p := range m.Members {
				if p.ID == s.UserID && p.Status != "cancelled" {
					allowed = true
				}
			}
			if !allowed {
				return r, notFound()
			}
		}
		if len(op.Slots) < 1 || len(op.Slots) > 5 {
			return r, domain.Validation("Предложите от 1 до 5 вариантов")
		}
		for _, slot := range op.Slots {
			if err = validSlot(slot, now); err != nil {
				return r, err
			}
			if slot.End.Sub(slot.Start) != m.Event.Duration {
				return r, domain.Validation("Длительность слота должна совпадать со встречей")
			}
		}
		r.ID = uuid.NewString()
		_, err = s.Q.ExecContext(ctx, `INSERT INTO reschedule_proposals(id,event_id,proposer_id,version,slots,created_at) VALUES($1,$2,$3,$4,$5,$6)`, r.ID, m.Event.ID, s.UserID, m.Version, jsonText(op.Slots), now)
		if err != nil {
			return r, err
		}
		name, e := s.name(ctx, s.UserID)
		if e != nil {
			return r, e
		}
		for _, uid := range meetingUsers(*m) {
			if uid != s.UserID {
				if err = s.notify(ctx, uid, "proposal:"+r.ID, name+" предлагает другое время для встречи «"+m.Event.Title+"». Сравните варианты в разделе «Встречи».", "meeting", m.Event.ID, now); err != nil {
					return r, err
				}
			}
		}
		r.Message = "Варианты времени отправлены. Исходное время сохранено до решения организатора."
	case "resolve_proposal":
		if !organizer {
			return r, notFound()
		}
		if _, e := uuid.Parse(op.ID); e != nil {
			return r, notFound()
		}
		var proposal *Proposal
		for i := range m.Proposals {
			if m.Proposals[i].ID == op.ID {
				proposal = &m.Proposals[i]
			}
		}
		if proposal == nil {
			return r, notFound()
		}
		if proposal.Status != "pending" || proposal.Version != m.Version {
			return r, problem(409, "stale_proposal", "Предложение уже обработано или устарело")
		}
		if op.Status == "declined" {
			_, err = s.Q.ExecContext(ctx, `UPDATE reschedule_proposals SET status='declined',resolved_at=$2 WHERE id=$1`, op.ID, now)
			if err == nil {
				err = s.notify(ctx, proposal.ProposerID, "proposal-declined:"+op.ID, "Предложение переноса встречи «"+m.Event.Title+"» отклонено. Исходное время сохранено.", "meeting", m.Event.ID, now)
			}
			r.Message = "Предложение отклонено. Исходное время сохранено."
			break
		}
		if op.Status != "accepted" || op.SlotIndex < 0 || op.SlotIndex >= len(proposal.Slots) {
			return r, domain.Validation("Выберите предложенный интервал")
		}
		slot := proposal.Slots[op.SlotIndex]
		if err = validSlot(slot, now); err != nil {
			return r, err
		}
		a, e := s.AvailabilityForMeeting(ctx, *m, slot, now)
		if e != nil {
			return r, e
		}
		if anyBusy(a) {
			return r, problem(409, "availability_conflict", "Кто-то из участников уже занят. Проверьте новые свободные интервалы.")
		}
		e2 := m.Event
		local := slot.Start.In(s.Location)
		e2.Date = domain.Day(local)
		e2.StartTime = &domain.TimeOfDay{Hour: local.Hour(), Minute: local.Minute()}
		e2.StartSecond = 0
		if _, err = tx.Update(ctx, e2, now); err != nil {
			return r, err
		}
		_, err = s.Q.ExecContext(ctx, `UPDATE reschedule_proposals SET status='accepted',resolved_at=$2 WHERE id=$1`, op.ID, now)
		r.Message = "Встреча перенесена. Все участники уведомлены и должны подтвердить новое время."
		if !a.Complete {
			r.Message += " Внешняя доступность проверена не полностью."
		}
	case "cancel_meeting":
		if !organizer {
			return r, notFound()
		}
		err = tx.Delete(ctx, m.Event.ID)
		r.Message = "Встреча отменена. Участники уведомлены."
	case "cancel_invitation":
		if !organizer {
			return r, notFound()
		}
		if _, e := uuid.Parse(op.UserID); e != nil {
			return r, notFound()
		}
		for i := range m.Members {
			if m.Members[i].ID == op.UserID {
				m.Members[i].Status = "cancelled"
			}
		}
		frozen := *m
		frozen.Cancelled = true
		// The organizer's editor view contains personal tags/reminders and legacy
		// participant metadata. History for the revoked guest retains public fields.
		frozen.Event.EventDetails = domain.EventDetails{}
		res, e := s.Q.ExecContext(ctx, `UPDATE meeting_participants SET status='cancelled',responded_at=$3,cancelled_snapshot=$4 WHERE event_id=$1 AND user_id=$2 AND status<>'cancelled'`, m.Event.ID, op.UserID, now, jsonText(frozen))
		if e != nil {
			return r, e
		}
		n, _ := res.RowsAffected()
		if n > 0 {
			err = s.notify(ctx, op.UserID, fmt.Sprintf("invite-cancel:%d:%s:%d", m.Event.ID, op.UserID, m.Version), "Организатор отменил ваше приглашение на встречу «"+m.Event.Title+"».", "meeting", m.Event.ID, now)
		}
		r.Message = "Приглашение отменено"
	}
	return r, err
}
func containsStatus(v string, values ...string) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}
func meetingUsers(m Meeting) []string {
	ids := []string{m.Organizer.ID}
	for _, p := range m.Members {
		if p.Status != "cancelled" {
			ids = append(ids, p.ID)
		}
	}
	return ids
}
func (s *Session) checkInvitees(ctx context.Context, ids []string) error {
	if len(ids) > 30 {
		return domain.Validation("Можно пригласить до 30 друзей")
	}
	for _, id := range ids {
		ok, err := s.IsFriend(ctx, id)
		if err != nil {
			return err
		}
		if !ok {
			return problem(403, "friend_required", "Приглашать можно только подтверждённых друзей")
		}
	}
	return nil
}
func (s *Session) invite(ctx context.Context, e domain.Event, ids []string, version int, now time.Time) error {
	for _, id := range ids {
		var generation int
		err := s.Q.QueryRowContext(ctx, `INSERT INTO meeting_participants(event_id,user_id,version,invited_at) VALUES($1,$2,$3,$4) ON CONFLICT(event_id,user_id) DO UPDATE SET status='pending',version=$3,invited_at=$4,responded_at=NULL,cancelled_snapshot=NULL,invitation_generation=meeting_participants.invitation_generation+1 WHERE meeting_participants.status='cancelled' RETURNING invitation_generation`, e.ID, id, version, now).Scan(&generation)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if generation > 0 {
			if err = s.notify(ctx, id, fmt.Sprintf("invitation:%d:%s:%d:%d", e.ID, id, version, generation), "Вас приглашают на встречу "+meetingLabel(e)+". Выберите «Приду», «Не приду» или предложите другое время.", "meeting", e.ID, now); err != nil {
				return err
			}
		}
	}
	return nil
}

type txContextKey struct{}

func WithTx(ctx context.Context, tx repository.Tx) context.Context {
	return context.WithValue(ctx, txContextKey{}, tx)
}
func eventTx(ctx context.Context) repository.Tx {
	t, _ := ctx.Value(txContextKey{}).(repository.Tx)
	return t
}

// BeforeChange runs for every writer: UI, assistant and imported Google changes.
func (s *Session) BeforeChange(ctx context.Context, tx repository.Tx, e domain.Event, deleting bool, now time.Time) error {
	var exists bool
	if err := s.Q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM collaborative_meetings WHERE event_id=$1)`, e.ID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return nil
	}
	m, err := s.Meeting(ctx, tx, e.ID)
	if err != nil {
		return err
	}
	if m.Organizer.ID != s.UserID {
		return problem(403, "organizer_required", "Только организатор может изменять эту встречу")
	}
	changed := deleting || m.Event.Date.String() != e.Date.String() || m.Event.StartTime.String() != e.StartTime.String() || m.Event.Duration != e.Duration
	if !changed {
		if m.Event.Title != e.Title || m.Event.Description != e.Description || m.Event.Location != e.Location || m.Event.MeetingURL != e.MeetingURL {
			for _, p := range m.Members {
				if p.Status != "cancelled" {
					if err = s.notify(ctx, p.ID, fmt.Sprintf("meeting-details:%d:%d", e.ID, now.UnixNano()), "Организатор обновил детали встречи «"+e.Title+"».", "meeting", e.ID, now); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if !deleting {
		start, end, ok := e.Interval(s.Location)
		if !ok {
			return domain.Validation("Для общей встречи нужны дата и время")
		}
		a, err := s.AvailabilityForMeeting(ctx, *m, Slot{start, end}, now)
		if err != nil {
			return err
		}
		if anyBusy(a) {
			return problem(409, "availability_conflict", "Новое время занято у участника. Проверьте доступность.")
		}
	}
	version := m.Version + 1
	_, err = s.Q.ExecContext(ctx, `UPDATE collaborative_meetings SET version=$2,cancelled=$3 WHERE event_id=$1`, e.ID, version, deleting)
	if err != nil {
		return err
	}
	status := "pending"
	if deleting {
		status = "cancelled"
	}
	_, err = s.Q.ExecContext(ctx, `UPDATE meeting_participants SET status=$2,version=$3,responded_at=NULL,response_revision=0 WHERE event_id=$1 AND status<>'cancelled'`, e.ID, status, version)
	if err != nil {
		return err
	}
	_, err = s.Q.ExecContext(ctx, `UPDATE reschedule_proposals SET status='superseded',resolved_at=$2 WHERE event_id=$1 AND status='pending'`, e.ID, now)
	if err != nil {
		return err
	}
	text := "Встреча " + meetingLabel(e) + " перенесена. Подтвердите участие в новом времени."
	if deleting {
		text = "Встреча «" + e.Title + "» отменена организатором."
	}
	for _, p := range m.Members {
		if p.Status != "cancelled" {
			if err = s.notify(ctx, p.ID, fmt.Sprintf("meeting-change:%d:%d", e.ID, version), text, "meeting", e.ID, now); err != nil {
				return err
			}
		}
	}
	return nil
}
func (s *Session) FindMeeting(ctx context.Context, tx repository.Tx, query string, names []string, date *domain.Date) (*Meeting, error) {
	matches, err := s.MatchingMeetings(ctx, tx, query, names, nil, date, nil)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, domain.Validation("Приглашение не найдено. Уточните название и дату.")
	}
	if len(matches) > 1 {
		return nil, domain.Validation("Найдено несколько встреч. Уточните название и дату в разделе «Встречи».")
	}
	return &matches[0], nil
}

func (s *Session) MatchingMeetings(ctx context.Context, tx repository.Tx, query string, names, ids []string, date *domain.Date, clock *domain.TimeOfDay) ([]Meeting, error) {
	items, err := s.Meetings(ctx, tx)
	if err != nil {
		return nil, err
	}
	// Include owned, non-collaborative meetings as well as received invitations.
	own, err := tx.Events(ctx, domain.Filter{Kind: domain.EventMeeting, Start: date})
	if err != nil {
		return nil, err
	}
	for _, e := range own {
		if !e.Collaborative && e.OrganizerID == s.UserID {
			items = append(items, Meeting{Event: e, Organizer: Person{ID: s.UserID, Name: e.OrganizerName}})
		}
	}
	matches := []Meeting{}
	for _, m := range items {
		if m.Cancelled || m.Event.DeletedAt != nil || m.Event.Status != domain.StatusOpen || date != nil && m.Event.Date.String() != date.String() || clock != nil && m.Event.StartTime.String() != clock.String() {
			continue
		}
		if !domain.GenericMeetingTitle(query) && !strings.Contains(strings.ToLower(m.Event.Label()), strings.ToLower(query)) {
			continue
		}
		all := true
		for _, name := range names {
			found := NameMatches(m.Organizer.Name, name)
			for _, p := range m.Members {
				found = found || p.Status != "cancelled" && NameMatches(p.Name, name)
			}
			all = all && found
		}
		for _, id := range ids {
			found := m.Organizer.ID == id
			for _, p := range m.Members {
				found = found || p.ID == id && p.Status != "cancelled"
			}
			all = all && found
		}
		for _, p := range m.Members {
			if p.ID == s.UserID && p.Status == "cancelled" {
				all = false
			}
		}
		if all {
			matches = append(matches, m)
		}
	}
	return matches, nil
}
