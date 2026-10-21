package social

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/google/uuid"
)

var codeFormat = regexp.MustCompile(`^[A-Z2-9]{8}$`)

func (s *Session) Profile(ctx context.Context) (Profile, error) {
	var p Profile
	err := s.Q.QueryRowContext(ctx, `SELECT id,display_name,avatar_url,coalesce(email,''),coalesce(friend_code,''),share_friend_email,planning_timezone,planning_start_hour,planning_end_hour FROM users WHERE id=$1`, s.UserID).Scan(&p.ID, &p.Name, &p.AvatarURL, &p.Email, &p.Code, &p.ShareEmail, &p.Timezone, &p.StartHour, &p.EndHour)
	p.Tags = []string{}
	return p, err
}
func (s *Session) People(ctx context.Context) (People, error) {
	p := People{Friends: []Person{}, Incoming: []FriendRequest{}, Outgoing: []FriendRequest{}}
	var err error
	p.Profile, err = s.Profile(ctx)
	if err != nil {
		return p, err
	}
	rows, err := s.Q.QueryContext(ctx, `SELECT u.id,u.display_name,u.avatar_url,CASE WHEN u.share_friend_email THEN coalesce(u.email,'') ELSE '' END,coalesce(t.tags,'[]') FROM friendships f JOIN users u ON u.id=CASE WHEN f.user_low=$1 THEN f.user_high ELSE f.user_low END LEFT JOIN friend_tags t ON t.user_id=$1 AND t.friend_id=u.id WHERE f.user_low=$1 OR f.user_high=$1 ORDER BY lower(u.display_name),u.id`, s.UserID)
	if err != nil {
		return p, err
	}
	for rows.Next() {
		var f Person
		var tags string
		if err = rows.Scan(&f.ID, &f.Name, &f.AvatarURL, &f.Email, &tags); err != nil {
			rows.Close()
			return p, err
		}
		if err = json.Unmarshal([]byte(tags), &f.Tags); err != nil {
			rows.Close()
			return p, err
		}
		p.Friends = append(p.Friends, f)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return p, err
	}
	rows, err = s.Q.QueryContext(ctx, `SELECT r.id,u.id,u.display_name,u.avatar_url,r.sender_id=$1,r.status,r.created_at FROM friend_requests r JOIN users u ON u.id=CASE WHEN r.sender_id=$1 THEN r.recipient_id ELSE r.sender_id END WHERE (r.sender_id=$1 OR r.recipient_id=$1) AND r.status='pending' ORDER BY r.created_at DESC`, s.UserID)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var r FriendRequest
		var outgoing bool
		if err = rows.Scan(&r.ID, &r.Person.ID, &r.Person.Name, &r.Person.AvatarURL, &outgoing, &r.Status, &r.CreatedAt); err != nil {
			return p, err
		}
		r.Person.Tags = []string{}
		if outgoing {
			p.Outgoing = append(p.Outgoing, r)
		} else {
			p.Incoming = append(p.Incoming, r)
		}
	}
	return p, rows.Err()
}
func (s *Session) IsFriend(ctx context.Context, id string) (bool, error) {
	if _, err := uuid.Parse(id); err != nil {
		return false, domain.Validation("Некорректный пользователь")
	}
	var ok bool
	err := s.Q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM friendships WHERE user_low=least($1::uuid,$2::uuid) AND user_high=greatest($1::uuid,$2::uuid))`, s.UserID, id).Scan(&ok)
	return ok, err
}

// Rate is committed separately from mutations so invalid guesses still count.
func (s *Session) Rate(ctx context.Context, bucket string, limit int) error {
	var count int
	err := s.Q.QueryRowContext(ctx, `INSERT INTO social_rate_limits(user_id,bucket,window_start) VALUES($1,$2,date_trunc('hour',now())) ON CONFLICT(user_id,bucket) DO UPDATE SET attempts=CASE WHEN social_rate_limits.window_start=excluded.window_start THEN social_rate_limits.attempts+1 ELSE 1 END,window_start=excluded.window_start RETURNING attempts`, s.UserID, bucket).Scan(&count)
	if err != nil {
		return err
	}
	if count > limit {
		return problem(429, "rate_limited", "Слишком много попыток. Повторите через час.")
	}
	return nil
}
func (s *Session) Execute(ctx context.Context, op Operation, now time.Time) (Result, error) {
	var result Result
	if _, err := uuid.Parse(op.RequestID); err != nil {
		return result, domain.Validation("Нужен request_id в формате UUID")
	}
	hash := sha256.Sum256([]byte(jsonText(op)))
	fingerprint := hex.EncodeToString(hash[:])
	var saved, previous string
	err := s.Q.QueryRowContext(ctx, `SELECT fingerprint,result FROM social_operations WHERE user_id=$1 AND request_id=$2`, s.UserID, op.RequestID).Scan(&previous, &saved)
	if err == nil {
		if previous != fingerprint {
			return result, problem(409, "idempotency_conflict", "Этот request_id уже использован для другого действия")
		}
		err = json.Unmarshal([]byte(saved), &result)
		return result, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}
	switch op.Action {
	case "request":
		result, err = s.request(ctx, op.Code, now)
	case "accept_request", "decline_request", "cancel_request":
		result, err = s.requestResponse(ctx, op, now)
	case "remove_friend":
		if _, e := uuid.Parse(op.UserID); e != nil {
			return result, domain.Validation("Некорректный пользователь")
		}
		_, err = s.Q.ExecContext(ctx, `DELETE FROM friendships WHERE user_low=least($1::uuid,$2::uuid) AND user_high=greatest($1::uuid,$2::uuid)`, s.UserID, op.UserID)
		if err == nil {
			_, err = s.Q.ExecContext(ctx, `DELETE FROM friend_tags WHERE (user_id=$1 AND friend_id=$2) OR (user_id=$2 AND friend_id=$1)`, s.UserID, op.UserID)
		}
		result.Message = "Друг удалён. История встреч сохранена."
	case "tags":
		ok, e := s.IsFriend(ctx, op.UserID)
		if e != nil {
			return result, e
		}
		if !ok {
			return result, notFound()
		}
		tags := []string{}
		seen := map[string]bool{}
		if len(op.Tags) > 12 {
			return result, domain.Validation("У друга может быть не больше 12 тегов")
		}
		for _, tag := range op.Tags {
			tag = strings.TrimSpace(tag)
			key := strings.ToLower(tag)
			if utf8.RuneCountInString(tag) < 1 || utf8.RuneCountInString(tag) > 40 {
				return result, domain.Validation("Тег должен содержать от 1 до 40 символов")
			}
			if !seen[key] {
				tags = append(tags, tag)
				seen[key] = true
			}
		}
		_, err = s.Q.ExecContext(ctx, `INSERT INTO friend_tags(user_id,friend_id,tags) VALUES($1,$2,$3) ON CONFLICT(user_id,friend_id) DO UPDATE SET tags=excluded.tags`, s.UserID, op.UserID, jsonText(tags))
		result.Message = "Теги сохранены. Их видите только вы."
	case "settings":
		p, e := s.Profile(ctx)
		if e != nil {
			return result, e
		}
		if op.ShareEmail != nil {
			p.ShareEmail = *op.ShareEmail
		}
		if op.Timezone != "" {
			if _, e = time.LoadLocation(op.Timezone); e != nil {
				return result, domain.Validation("Неизвестный часовой пояс")
			}
			p.Timezone = op.Timezone
		}
		if op.StartHour != nil {
			p.StartHour = *op.StartHour
		}
		if op.EndHour != nil {
			p.EndHour = *op.EndHour
		}
		if p.StartHour < 0 || p.StartHour >= p.EndHour || p.EndHour > 24 {
			return result, domain.Validation("Проверьте доступные часы")
		}
		_, err = s.Q.ExecContext(ctx, `UPDATE users SET share_friend_email=$2,planning_timezone=$3,planning_start_hour=$4,planning_end_hour=$5 WHERE id=$1`, s.UserID, p.ShareEmail, p.Timezone, p.StartHour, p.EndHour)
		result.Message = "Настройки сохранены"
	case "create_meeting", "invite", "respond", "propose", "resolve_proposal", "cancel_meeting", "cancel_invitation":
		result, err = s.meetingOperation(ctx, op, now)
	case "set_participants":
		result, err = s.setParticipants(ctx, op, now)
	default:
		return result, domain.Validation("Неизвестное действие")
	}
	if err != nil {
		return result, err
	}
	_, err = s.Q.ExecContext(ctx, `INSERT INTO social_operations(user_id,request_id,fingerprint,result) VALUES($1,$2,$3,$4)`, s.UserID, op.RequestID, fingerprint, jsonText(result))
	return result, err
}

// CachedOperation lets an HTTP retry return its committed result before doing
// fresh availability checks (the original time may already be in the past).
func (s *Session) CachedOperation(ctx context.Context, op Operation) (*Result, error) {
	if _, err := uuid.Parse(op.RequestID); err != nil {
		return nil, domain.Validation("Нужен request_id в формате UUID")
	}
	var fingerprint, raw string
	err := s.Q.QueryRowContext(ctx, `SELECT fingerprint,result FROM social_operations WHERE user_id=$1 AND request_id=$2`, s.UserID, op.RequestID).Scan(&fingerprint, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256([]byte(jsonText(op)))
	if fingerprint != hex.EncodeToString(hash[:]) {
		return nil, problem(409, "idempotency_conflict", "Этот request_id уже использован для другого действия")
	}
	var result Result
	err = json.Unmarshal([]byte(raw), &result)
	return &result, err
}
func (s *Session) request(ctx context.Context, code string, now time.Time) (Result, error) {
	var r Result
	code = strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
	if !codeFormat.MatchString(code) {
		return r, domain.Validation("Введите 8 латинских букв или цифр из кода друга")
	}
	code = code[:4] + "-" + code[4:]
	var other string
	err := s.Q.QueryRowContext(ctx, `SELECT id FROM users WHERE friend_code=$1`, code).Scan(&other)
	if errors.Is(err, sql.ErrNoRows) {
		return r, problem(404, "code_unavailable", "Не удалось отправить заявку. Проверьте код у друга.")
	}
	if err != nil {
		return r, err
	}
	if other == s.UserID {
		return r, domain.Validation("Это ваш код. Введите код друга.")
	}
	ok, err := s.IsFriend(ctx, other)
	if err != nil {
		return r, err
	}
	if ok {
		return r, problem(409, "already_friends", "Вы уже друзья")
	}
	var existing string
	err = s.Q.QueryRowContext(ctx, `SELECT id FROM friend_requests WHERE least(sender_id,recipient_id)=least($1::uuid,$2::uuid) AND greatest(sender_id,recipient_id)=greatest($1::uuid,$2::uuid) AND status='pending'`, s.UserID, other).Scan(&existing)
	if err == nil {
		return r, problem(409, "request_exists", "Заявка уже есть. Проверьте входящие и отправленные.")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return r, err
	}
	r.ID = uuid.NewString()
	_, err = s.Q.ExecContext(ctx, `INSERT INTO friend_requests(id,sender_id,recipient_id,created_at) VALUES($1,$2,$3,$4)`, r.ID, s.UserID, other, now)
	if err != nil {
		return r, err
	}
	name, err := s.name(ctx, s.UserID)
	if err != nil {
		return r, err
	}
	err = s.notify(ctx, other, "friend-request:"+r.ID, name+" хочет добавить вас в друзья. Откройте раздел «Друзья».", "friends", 0, now)
	r.Message = "Заявка отправлена"
	return r, err
}
func (s *Session) requestResponse(ctx context.Context, op Operation, now time.Time) (Result, error) {
	r := Result{ID: op.ID}
	if _, err := uuid.Parse(op.ID); err != nil {
		return r, notFound()
	}
	var sender, recipient, status string
	err := s.Q.QueryRowContext(ctx, `SELECT sender_id,recipient_id,status FROM friend_requests WHERE id=$1 AND (sender_id=$2 OR recipient_id=$2)`, op.ID, s.UserID).Scan(&sender, &recipient, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return r, notFound()
	}
	if err != nil {
		return r, err
	}
	desired := "declined"
	if op.Action == "cancel_request" {
		desired = "cancelled"
		if sender != s.UserID {
			return r, notFound()
		}
	} else {
		if recipient != s.UserID {
			return r, notFound()
		}
		if op.Action == "accept_request" {
			desired = "accepted"
		}
	}
	if status == desired {
		r.Message = "Ответ уже сохранён"
		return r, nil
	}
	if status != "pending" {
		return r, problem(409, "request_resolved", "На заявку уже ответили")
	}
	_, err = s.Q.ExecContext(ctx, `UPDATE friend_requests SET status=$2,responded_at=$3 WHERE id=$1`, op.ID, desired, now)
	if err != nil {
		return r, err
	}
	if desired == "accepted" {
		_, err = s.Q.ExecContext(ctx, `INSERT INTO friendships(user_low,user_high) VALUES(least($1::uuid,$2::uuid),greatest($1::uuid,$2::uuid)) ON CONFLICT DO NOTHING`, sender, recipient)
		if err != nil {
			return r, err
		}
		name, e := s.name(ctx, s.UserID)
		if e != nil {
			return r, e
		}
		err = s.notify(ctx, sender, "friend-accepted:"+op.ID, name+" принял(а) заявку. Теперь вы можете планировать общие встречи.", "friends", 0, now)
	}
	r.Message = map[string]string{"accepted": "Теперь вы друзья!", "declined": "Заявка отклонена", "cancelled": "Заявка отменена"}[desired]
	return r, err
}
func (s *Session) name(ctx context.Context, id string) (string, error) {
	var name string
	err := s.Q.QueryRowContext(ctx, `SELECT display_name FROM users WHERE id=$1`, id).Scan(&name)
	return name, err
}
func (s *Session) notify(ctx context.Context, uid, key, text, kind string, eventID int64, now time.Time) error {
	sid := uuid.NewSHA1(uuid.NameSpaceOID, []byte("gena:social:"+uid)).String()
	rid := uuid.NewSHA1(uuid.NameSpaceOID, []byte("gena:social:"+uid+":"+key)).String()
	response := domain.Response{Reply: text, Action: domain.ActionList, Events: []domain.Event{}, SessionID: sid, RequestID: rid, Provider: "social", Timezone: s.Location.String(), Social: &domain.SocialNotice{Kind: kind, EventID: eventID}}
	if kind == "meeting" && eventID > 0 {
		viewer := *s
		viewer.UserID = uid
		m, err := viewer.Meeting(ctx, eventTx(ctx), eventID)
		if err != nil {
			return err
		}
		if domain.GenericMeetingTitle(m.Event.Title) {
			response.Reply = strings.ReplaceAll(response.Reply, "«"+m.Event.Title+"»", "«"+m.Event.Label()+"»")
		}
		response.Events = []domain.Event{m.Event}
	}
	_, err := s.Q.ExecContext(ctx, `INSERT INTO dialogue_sessions(user_id,id,state,updated_at) VALUES($1,$2,'{}',$3) ON CONFLICT DO NOTHING`, uid, sid, now.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	_, err = s.Q.ExecContext(ctx, `INSERT INTO dialogue_exchanges(user_id,session_id,request_id,message,response,created_at,notification) VALUES($1,$2,$3,'',$4,$5,true) ON CONFLICT(user_id,session_id,request_id) DO NOTHING`, uid, sid, rid, jsonText(response), now.Format(time.RFC3339Nano))
	return err
}

// MissingFriendsError distinguishes absent contacts from ambiguous names and
// storage failures. Only ordinary personal meeting creation may recover from it.
type MissingFriendsError struct {
	Names []string
}

func (e *MissingFriendsError) Error() string {
	if len(e.Names) == 1 {
		return "Друг «" + e.Names[0] + "» не найден. Добавьте его по коду в разделе «Друзья»."
	}
	return "Не найдены в друзьях: «" + strings.Join(e.Names, "», «") + "». Добавьте их по коду в разделе «Друзья»."
}

func (e *MissingFriendsError) Unwrap() error { return domain.Validation(e.Error()) }

// Resolve never exposes account search: candidates come only from confirmed friends.
func (s *Session) Resolve(ctx context.Context, names []string, tag string) ([]string, error) {
	p, err := s.People(ctx)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	missing := []string{}
	if len(names) > 30 {
		return nil, domain.Validation("Выберите не больше 30 друзей")
	}
	if tag != "" {
		for _, f := range p.Friends {
			for _, t := range f.Tags {
				if strings.EqualFold(t, tag) {
					ids = append(ids, f.ID)
					break
				}
			}
		}
		if len(ids) == 0 {
			return nil, domain.Validation("Нет друзей с тегом «" + tag + "»")
		}
	}
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			return nil, domain.Validation("Укажите имя участника")
		}
		matches := []Person{}
		exact := []Person{}
		for _, f := range p.Friends {
			if SameName(f.Name, name) {
				exact = append(exact, f)
			}
			matched := NameMatches(f.Name, name)
			for _, t := range f.Tags {
				matched = matched || strings.EqualFold(strings.TrimSpace(t), strings.TrimSpace(name))
			}
			if matched {
				matches = append(matches, f)
			}
		}
		if len(exact) > 0 {
			matches = exact
		}
		if len(matches) == 0 {
			missing = append(missing, name)
			continue
		}
		if len(matches) > 1 {
			labels := []string{}
			for _, f := range matches {
				labels = append(labels, f.Name)
			}
			return nil, domain.Validation("Уточните полное имя: " + strings.Join(labels, ", "))
		}
		ids = append(ids, matches[0].ID)
	}
	if len(missing) > 0 {
		return nil, &MissingFriendsError{Names: missing}
	}
	sort.Strings(ids)
	out := []string{}
	for _, id := range ids {
		if len(out) == 0 || out[len(out)-1] != id {
			out = append(out, id)
		}
	}
	if len(out) > 30 {
		return nil, domain.Validation("Выберите не больше 30 друзей")
	}
	return out, nil
}
func nameWords(s string) []string {
	return strings.Fields(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "ё", "е"))
}

// A tag used for one person must never silently invite an entire group.
func (s *Session) ResolveInvitees(ctx context.Context, names []string, tag string, all bool) ([]string, error) {
	ids, err := s.Resolve(ctx, names, tag)
	if err != nil {
		return nil, err
	}
	if tag != "" && len(ids) > 1 && !all {
		p, err := s.People(ctx)
		if err != nil {
			return nil, err
		}
		labels := []string{}
		for _, f := range p.Friends {
			for _, id := range ids {
				if id == f.ID {
					labels = append(labels, f.Name)
				}
			}
		}
		return nil, domain.Validation("Уточните, с кем встретиться: " + strings.Join(labels, ", "))
	}
	return ids, nil
}
func meetingLabel(e domain.Event) string {
	return fmt.Sprintf("«%s», %s в %s", e.Label(), e.Date, e.StartTime)
}
