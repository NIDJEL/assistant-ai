package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

func (d *Database) CloudConnection(ctx context.Context, p string) (*repository.CloudConnection, error) {
	uid, err := calendarUser(ctx)
	if err != nil {
		return nil, err
	}
	c := repository.CloudConnection{Provider: p}
	err = d.DB.QueryRowContext(ctx, `SELECT subject,login,token_ciphertext,expires_at,status,calendar_id,sync_token FROM cloud_connections WHERE user_id=$1 AND provider=$2`, uid, p).Scan(&c.Subject, &c.Login, &c.Ciphertext, &c.ExpiresAt, &c.Status, &c.CalendarID, &c.SyncToken)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &c, err
}
func (d *Database) SaveCloudConnection(ctx context.Context, c repository.CloudConnection) error {
	uid, err := calendarUser(ctx)
	if err != nil {
		return err
	}
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", uid); err != nil {
		return err
	}
	var subject, status string
	err = tx.QueryRowContext(ctx, `SELECT subject,status FROM cloud_connections WHERE user_id=$1 AND provider=$2 FOR UPDATE`, uid, c.Provider).Scan(&subject, &status)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if subject != "" && subject != c.Subject {
		if status != "disconnected" {
			return domain.Validation("Сначала отключите прежний аккаунт")
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM cloud_event_links WHERE user_id=$1 AND provider=$2`, uid, c.Provider); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM cloud_outbox WHERE user_id=$1 AND provider=$2`, uid, c.Provider); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO cloud_connections(user_id,provider,subject,login,token_ciphertext,expires_at,status,calendar_id,sync_token) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
 ON CONFLICT(user_id,provider) DO UPDATE SET subject=$3,login=$4,token_ciphertext=$5,expires_at=$6,status=$7,calendar_id=$8,sync_token=$9,last_error=''`, uid, c.Provider, c.Subject, c.Login, c.Ciphertext, c.ExpiresAt, c.Status, c.CalendarID, c.SyncToken)
	if err != nil {
		return err
	}
	if status == "disconnected" || subject == "" {
		if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_outbox(user_id,provider,event_id) SELECT user_id,$2,id FROM events WHERE user_id=$1 AND (($2='google' AND $3<>'' AND date IS NOT NULL) OR ($2='zoom' AND (details::jsonb->>'conference_provider'='zoom' OR EXISTS(SELECT 1 FROM cloud_event_links l WHERE l.user_id=$1 AND l.provider='zoom' AND l.event_id=events.id)))) ON CONFLICT(user_id,provider,event_id) DO UPDATE SET version=cloud_outbox.version+1`, uid, c.Provider, c.CalendarID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (d *Database) CloudConnections(ctx context.Context) ([]repository.CalendarConnection, error) {
	uid, err := calendarUser(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := d.DB.QueryContext(ctx, `SELECT provider,status,last_sync,last_error,(SELECT count(*) FROM cloud_outbox o WHERE o.user_id=c.user_id AND o.provider=c.provider) FROM cloud_connections c WHERE user_id=$1 ORDER BY provider`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []repository.CalendarConnection{}
	for rows.Next() {
		var c repository.CalendarConnection
		if err = rows.Scan(&c.Provider, &c.Status, &c.LastSync, &c.LastError, &c.PendingEvents); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}
func (d *Database) CloudUsers(ctx context.Context) ([][2]string, error) {
	rows, err := d.DB.QueryContext(ctx, `SELECT user_id,provider FROM cloud_connections WHERE status IN ('connected','pending','error') ORDER BY provider DESC,user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := [][2]string{}
	for rows.Next() {
		var v [2]string
		if err = rows.Scan(&v[0], &v[1]); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, rows.Err()
}
func (d *Database) WithCloudLock(ctx context.Context, p string, fn func() error) error {
	uid, err := calendarUser(ctx)
	if err != nil {
		return err
	}
	key := uid + ":" + p
	conn, err := d.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var locked bool
	if err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,3))", key).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return &domain.AppError{Code: "integration_busy", Message: "Синхронизация уже выполняется", Status: 409}
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, e := conn.ExecContext(c, "SELECT pg_advisory_unlock(hashtextextended($1,3))", key); e != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	return fn()
}
func (d *Database) CloudStatus(ctx context.Context, p, status, message string, stamp bool) error {
	uid, err := calendarUser(ctx)
	if err != nil {
		return err
	}
	_, err = d.DB.ExecContext(ctx, `UPDATE cloud_connections SET status=$3,last_error=$4,last_sync=CASE WHEN $5 THEN now() ELSE last_sync END WHERE user_id=$1 AND provider=$2`, uid, p, status, message, stamp)
	return err
}
func (d *Database) DisconnectCloud(ctx context.Context, p string) error {
	uid, err := calendarUser(ctx)
	if err != nil {
		return err
	}
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", uid); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cloud_connections SET status='disconnected',token_ciphertext=NULL,expires_at=NULL,last_error='',sync_token='' WHERE user_id=$1 AND provider=$2`, uid, p); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM cloud_outbox WHERE user_id=$1 AND provider=$2`, uid, p); err != nil {
		return err
	}
	return tx.Commit()
}
func (d *Database) SelectCloudCalendar(ctx context.Context, id string) error {
	uid, err := calendarUser(ctx)
	if err != nil {
		return err
	}
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", uid); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cloud_connections SET calendar_id=$2,sync_token='',status='pending',last_error='' WHERE user_id=$1 AND provider='google'`, uid, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_outbox(user_id,provider,event_id) SELECT $1,'google',id FROM events WHERE `+visibleEvents("$1")+` AND date IS NOT NULL ON CONFLICT(user_id,provider,event_id) DO UPDATE SET version=cloud_outbox.version+1`, uid); err != nil {
		return err
	}
	return tx.Commit()
}
func (d *Database) CloudChanges(ctx context.Context, p string) ([]repository.CalendarChange, error) {
	uid, err := calendarUser(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := d.DB.QueryContext(ctx, `SELECT event_id,version FROM cloud_outbox WHERE user_id=$1 AND provider=$2 ORDER BY event_id LIMIT 100`, uid, p)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []repository.CalendarChange{}
	for rows.Next() {
		var v repository.CalendarChange
		if err = rows.Scan(&v.EventID, &v.Version); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, rows.Err()
}
func (d *Database) AckCloudChange(ctx context.Context, p string, c repository.CalendarChange) error {
	uid, err := calendarUser(ctx)
	if err != nil {
		return err
	}
	_, err = d.DB.ExecContext(ctx, `DELETE FROM cloud_outbox WHERE user_id=$1 AND provider=$2 AND event_id=$3 AND version=$4`, uid, p, c.EventID, c.Version)
	return err
}
func (d *Database) HasCloudChange(ctx context.Context, p string, id int64) (bool, error) {
	uid, err := calendarUser(ctx)
	if err != nil {
		return false, err
	}
	var pending bool
	err = d.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_outbox WHERE user_id=$1 AND provider=$2 AND event_id=$3)`, uid, p, id).Scan(&pending)
	return pending, err
}
func (d *Database) CloudLink(ctx context.Context, p, cal string, id int64) (*repository.CloudLink, error) {
	uid, err := calendarUser(ctx)
	if err != nil {
		return nil, err
	}
	v := repository.CloudLink{EventID: id}
	err = d.DB.QueryRowContext(ctx, `SELECT remote_id,etag,remote_updated FROM cloud_event_links WHERE user_id=$1 AND provider=$2 AND calendar_id=$3 AND event_id=$4`, uid, p, cal, id).Scan(&v.RemoteID, &v.ETag, &v.Updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &v, err
}
func saveCloudLink(ctx context.Context, q executor, uid, p, cal string, v repository.CloudLink) error {
	_, err := q.ExecContext(ctx, `INSERT INTO cloud_event_links(user_id,provider,calendar_id,event_id,remote_id,etag,remote_updated) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(user_id,provider,calendar_id,event_id) DO UPDATE SET remote_id=$5,etag=$6,remote_updated=$7`, uid, p, cal, v.EventID, v.RemoteID, v.ETag, v.Updated)
	return err
}
func (d *Database) SaveCloudLink(ctx context.Context, p, cal string, v repository.CloudLink) error {
	uid, err := calendarUser(ctx)
	if err != nil {
		return err
	}
	return saveCloudLink(ctx, d.DB, uid, p, cal, v)
}
func (d *Database) SetCloudSyncToken(ctx context.Context, token string) error {
	uid, err := calendarUser(ctx)
	if err != nil {
		return err
	}
	_, err = d.DB.ExecContext(ctx, `UPDATE cloud_connections SET sync_token=$2 WHERE user_id=$1 AND provider='google'`, uid, token)
	return err
}

// A remote read and its local mutation share the event writer lock. A concurrent
// local edit wins and stays in the durable outbox; a stale page cannot erase it.
func (d *Database) ImportGoogleEvent(ctx context.Context, cal string, link repository.CloudLink, incoming *domain.Event, cancelled bool) error {
	uid, err := calendarUser(ctx)
	if err != nil {
		return err
	}
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(73121013)"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", uid); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "SELECT set_config('gena.cloud_import','google',true)"); err != nil {
		return err
	}
	var id int64
	var etag string
	err = tx.QueryRowContext(ctx, `SELECT event_id,etag FROM cloud_event_links WHERE user_id=$1 AND provider='google' AND calendar_id=$2 AND remote_id=$3`, uid, cal, link.RemoteID).Scan(&id, &etag)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	repo := &Repo{q: tx, userID: uid}
	if id != 0 {
		var dirty bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_outbox WHERE user_id=$1 AND provider='google' AND event_id=$2)`, uid, id).Scan(&dirty); err != nil {
			return err
		}
		if dirty || etag == link.ETag && etag != "" {
			return tx.Commit()
		}
		e, err := repo.Get(ctx, id)
		if err != nil {
			return err
		}
		if e == nil {
			return tx.Commit()
		}
		// An attendee's external calendar cannot mutate the canonical meeting.
		if e.Collaborative && e.OrganizerID != uid {
			link.EventID = id
			if err = saveCloudLink(ctx, tx, uid, "google", cal, link); err != nil {
				return err
			}
			return tx.Commit()
		}
		if cancelled {
			if err = repo.Delete(ctx, id); err != nil {
				return err
			}
		} else if incoming != nil {
			e.Title, e.Description, e.Location, e.Date, e.StartTime, e.Duration = incoming.Title, incoming.Description, incoming.Location, incoming.Date, incoming.StartTime, incoming.Duration
			if incoming.MeetingURL != "" {
				e.MeetingURL = incoming.MeetingURL
			}
			e.StartSecond = incoming.StartSecond
			e.GoogleAllDayDays, e.GoogleDurationSeconds = incoming.GoogleAllDayDays, incoming.GoogleDurationSeconds
			e.GoogleTransparent = incoming.GoogleTransparent
			if e.Kind == domain.EventMeeting && e.StartTime == nil {
				e.Kind = domain.EventTask
				if _, err = tx.ExecContext(ctx, `UPDATE events SET kind='task' WHERE user_id=$1 AND id=$2`, uid, id); err != nil {
					return err
				}
			}
			// Preserve local reminders, tasks and Zoom metadata while importing schedule.
			if _, err = repo.Update(ctx, *e, time.Now()); err != nil {
				return err
			}
		}
	} else if incoming != nil && !cancelled {
		key := "google:" + cal + ":" + link.RemoteID
		incoming.SourceKey = &key
		e, err := repo.Create(ctx, *incoming, time.Now())
		if err != nil {
			return err
		}
		if e == nil {
			if err = tx.QueryRowContext(ctx, `SELECT id FROM events WHERE user_id=$1 AND source_key=$2`, uid, key).Scan(&id); err != nil {
				return err
			}
		} else {
			id = e.ID
		}
	} else {
		return tx.Commit()
	}
	link.EventID = id
	if err = saveCloudLink(ctx, tx, uid, "google", cal, link); err != nil {
		return err
	}
	// Imported schedule changes also reschedule an already linked Zoom meeting.
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_outbox(user_id,provider,event_id) SELECT $1,'zoom',$2 WHERE EXISTS(SELECT 1 FROM cloud_event_links WHERE user_id=$1 AND provider='zoom' AND event_id=$2) AND EXISTS(SELECT 1 FROM cloud_connections WHERE user_id=$1 AND provider='zoom' AND status<>'disconnected') ON CONFLICT(user_id,provider,event_id) DO UPDATE SET version=cloud_outbox.version+1`, uid, id); err != nil {
		return err
	}
	return tx.Commit()
}
