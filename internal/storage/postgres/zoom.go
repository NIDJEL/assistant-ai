package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/google/uuid"
	"time"
)

func (r *Repo) ZoomState(ctx context.Context, eventID int64, runID string) (string, string, error) {
	var run, state string
	err := r.q.QueryRowContext(ctx, `SELECT z.run_uuid, CASE WHEN z.ended_at IS NOT NULL THEN 'ended' ELSE 'started' END
 FROM zoom_meeting_runs z JOIN cloud_event_links l ON l.user_id=z.user_id AND l.event_id=z.event_id AND l.remote_id=z.meeting_id AND l.provider='zoom'
 WHERE z.user_id=$1 AND z.event_id=$2 AND ($3='' OR z.run_uuid=$3)
 ORDER BY coalesce(z.started_at,z.ended_at) DESC LIMIT 1`, r.userID, eventID, runID).Scan(&run, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	return run, state, err
}

// ReceiveZoomEvent is called only after signature verification. Ownership comes
// from the saved meeting mapping and connected host, never webhook text.
func (d *Database) ReceiveZoomEvent(ctx context.Context, v repository.ZoomEvent, loc *time.Location, now time.Time) error {
	var uid string
	var eid int64
	err := d.DB.QueryRowContext(ctx, `SELECT l.user_id,l.event_id FROM cloud_event_links l
 JOIN cloud_connections c ON c.user_id=l.user_id AND c.provider='zoom' AND c.status<>'disconnected'
 JOIN events e ON e.id=l.event_id AND e.user_id=l.user_id AND e.deleted_at IS NULL AND e.kind='meeting'
 WHERE l.provider='zoom' AND l.remote_id=$1 AND c.subject=$2 LIMIT 1`, v.MeetingID, v.HostID).Scan(&uid, &eid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	ctx = repository.WithUser(ctx, uid)
	return d.Write(ctx, func(tx repository.Tx) error {
		r := tx.(*Repo)
		var linked int
		err := r.q.QueryRowContext(ctx, `SELECT 1 FROM cloud_event_links l JOIN cloud_connections c ON c.user_id=l.user_id AND c.provider='zoom'
   WHERE l.user_id=$1 AND l.event_id=$2 AND l.provider='zoom' AND l.remote_id=$3 AND c.subject=$4 AND c.status<>'disconnected' FOR SHARE OF l,c`, uid, eid, v.MeetingID, v.HostID).Scan(&linked)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		e, err := tx.Get(ctx, eid)
		if err != nil || e == nil {
			return err
		}
		card := domain.ZoomCard(*e, loc)
		if card == nil {
			return nil
		}
		var started, ended any
		if v.Event == "meeting.started" {
			started = v.OccurredAt
		}
		if v.Event == "meeting.ended" {
			ended = v.OccurredAt
			if v.StartedAt != nil {
				started = *v.StartedAt
			}
		}
		_, err = r.q.ExecContext(ctx, `INSERT INTO zoom_meeting_runs(user_id,event_id,meeting_id,run_uuid,started_at,ended_at) VALUES($1,$2,$3,$4,$5,$6)
   ON CONFLICT(user_id,meeting_id,run_uuid) DO UPDATE SET started_at=coalesce(zoom_meeting_runs.started_at,excluded.started_at),ended_at=coalesce(zoom_meeting_runs.ended_at,excluded.ended_at)`, uid, eid, v.MeetingID, v.RunID, started, ended)
		if err != nil {
			return err
		}
		var notified bool
		var end sql.NullTime
		if err = r.q.QueryRowContext(ctx, `SELECT notified,ended_at FROM zoom_meeting_runs WHERE user_id=$1 AND meeting_id=$2 AND run_uuid=$3`, uid, v.MeetingID, v.RunID).Scan(&notified, &end); err != nil {
			return err
		}
		if v.Event != "meeting.started" || notified || end.Valid {
			return nil
		}
		card.Status, card.RunID = "started", v.RunID
		sid := uuid.NewSHA1(uuid.NameSpaceOID, []byte("gena:zoom-notifications:"+uid)).String()
		rid := uuid.NewSHA1(uuid.NameSpaceOID, []byte("gena:zoom-start:"+uid+":"+v.MeetingID+":"+v.RunID)).String()
		response := domain.Response{Reply: fmt.Sprintf("Ваша встреча „%s“ началась. Можете присоединиться прямо сейчас.", e.Title), Action: domain.ActionList, Events: []domain.Event{*e}, ZoomMeetings: []domain.ZoomMeeting{*card}, SessionID: sid, RequestID: rid, Provider: "zoom_webhook", Timezone: loc.String()}
		if err = tx.SaveState(ctx, sid, domain.State{}, now); err != nil {
			return err
		}
		if err = tx.SaveExchange(ctx, sid, rid, "", response, now); err != nil {
			return err
		}
		if _, err = r.q.ExecContext(ctx, `UPDATE dialogue_exchanges SET notification=true WHERE user_id=$1 AND session_id=$2 AND request_id=$3`, uid, sid, rid); err != nil {
			return err
		}
		_, err = r.q.ExecContext(ctx, `UPDATE zoom_meeting_runs SET notified=true WHERE user_id=$1 AND meeting_id=$2 AND run_uuid=$3`, uid, v.MeetingID, v.RunID)
		return err
	})
}

func (d *Database) ChatNotifications(ctx context.Context) ([]domain.Exchange, error) {
	uid := repository.UserID(ctx)
	if uid == "" {
		return nil, errors.New("authenticated user scope required")
	}
	rows, err := d.DB.QueryContext(ctx, `SELECT response,created_at,read_at IS NOT NULL FROM dialogue_exchanges WHERE user_id=$1 AND notification AND
 (read_at IS NULL OR id IN (SELECT id FROM dialogue_exchanges WHERE user_id=$1 AND notification ORDER BY id DESC LIMIT 100)) ORDER BY id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []domain.Exchange{}
	for rows.Next() {
		x := domain.Exchange{Notification: true}
		var raw, stamp string
		if err = rows.Scan(&raw, &stamp, &x.Read); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &x.Response); err != nil {
			return nil, err
		}
		if x.CreatedAt, err = time.Parse(time.RFC3339Nano, stamp); err != nil {
			return nil, err
		}
		list = append(list, x)
	}
	return list, rows.Err()
}
func (d *Database) ReadChatNotifications(ctx context.Context, ids []string, now time.Time) error {
	uid := repository.UserID(ctx)
	if uid == "" {
		return errors.New("authenticated user scope required")
	}
	for _, id := range ids {
		if _, err := d.DB.ExecContext(ctx, `UPDATE dialogue_exchanges SET read_at=$3 WHERE user_id=$1 AND request_id=$2 AND notification AND read_at IS NULL`, uid, id, now); err != nil {
			return err
		}
	}
	return nil
}
