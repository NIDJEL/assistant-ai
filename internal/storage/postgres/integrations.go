package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/google/uuid"
	"time"
)

func (d *Database) Connections(ctx context.Context) ([]repository.CalendarConnection, error) {
	id := repository.UserID(ctx)
	if id == "" {
		return nil, fmt.Errorf("authenticated user scope required")
	}
	rows, err := d.DB.QueryContext(ctx, "SELECT provider,status,last_sync,last_error,(SELECT count(*) FROM calendar_outbox WHERE user_id=$1) FROM calendar_connections WHERE user_id=$1 ORDER BY provider", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []repository.CalendarConnection{}
	for rows.Next() {
		var c repository.CalendarConnection
		if err = rows.Scan(&c.Provider, &c.Status, &c.LastSync, &c.LastError, &c.PendingEvents); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func calendarUser(ctx context.Context) (string, error) {
	id := repository.UserID(ctx)
	if id == "" {
		return "", fmt.Errorf("authenticated user scope required")
	}
	return id, nil
}

func (d *Database) CalendarCredentials(ctx context.Context) (*repository.CalendarCredentials, error) {
	id, err := calendarUser(ctx)
	if err != nil {
		return nil, err
	}
	var c repository.CalendarCredentials
	err = d.DB.QueryRowContext(ctx, "SELECT provider,status,provider_subject,login,token_ciphertext,expires_at,calendar_url FROM calendar_connections WHERE user_id=$1 AND provider='yandex'", id).Scan(&c.Provider, &c.Status, &c.Subject, &c.Login, &c.Ciphertext, &c.ExpiresAt, &c.CalendarURL)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &c, err
}
func (d *Database) SaveCalendarCredentials(ctx context.Context, c repository.CalendarCredentials) error {
	id, err := calendarUser(ctx)
	if err != nil {
		return err
	}
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Share the event writer's lock so a concurrent create cannot fall between
	// the initial backfill and activation of the event trigger.
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", id); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO calendar_connections(id,user_id,provider,status,provider_subject,login,token_ciphertext,expires_at)
 VALUES($1,$2,'yandex','pending',$3,$4,$5,$6) ON CONFLICT(user_id,provider) DO UPDATE SET
 status='pending',provider_subject=$3,login=$4,token_ciphertext=$5,expires_at=$6,last_error='',
 calendar_url=CASE WHEN calendar_connections.provider_subject=$3 THEN calendar_connections.calendar_url ELSE '' END`, uuid.NewString(), id, c.Subject, c.Login, c.Ciphertext, c.ExpiresAt)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO calendar_outbox(user_id,event_id) SELECT user_id,id FROM events WHERE user_id=$1
 ON CONFLICT(user_id,event_id) DO UPDATE SET version=calendar_outbox.version+1`, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (d *Database) SetCalendarTarget(ctx context.Context, target string) error {
	id, err := calendarUser(ctx)
	if err != nil {
		return err
	}
	_, err = d.DB.ExecContext(ctx, "UPDATE calendar_connections SET calendar_url=$2 WHERE user_id=$1 AND provider='yandex'", id, target)
	return err
}
func (d *Database) CalendarChanges(ctx context.Context, limit int) ([]repository.CalendarChange, error) {
	id, err := calendarUser(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := d.DB.QueryContext(ctx, "SELECT event_id,version FROM calendar_outbox WHERE user_id=$1 ORDER BY event_id LIMIT $2", id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	changes := []repository.CalendarChange{}
	for rows.Next() {
		var c repository.CalendarChange
		if err = rows.Scan(&c.EventID, &c.Version); err != nil {
			return nil, err
		}
		changes = append(changes, c)
	}
	return changes, rows.Err()
}
func (d *Database) AcknowledgeCalendarChange(ctx context.Context, c repository.CalendarChange) error {
	id, err := calendarUser(ctx)
	if err != nil {
		return err
	}
	_, err = d.DB.ExecContext(ctx, "DELETE FROM calendar_outbox WHERE user_id=$1 AND event_id=$2 AND version=$3", id, c.EventID, c.Version)
	return err
}
func (d *Database) CalendarSyncStatus(ctx context.Context, status, message string, synced *time.Time) error {
	id, err := calendarUser(ctx)
	if err != nil {
		return err
	}
	_, err = d.DB.ExecContext(ctx, "UPDATE calendar_connections SET status=$2,last_error=$3,last_sync=coalesce($4,last_sync) WHERE user_id=$1 AND provider='yandex'", id, status, message, synced)
	return err
}
func (d *Database) DisconnectCalendar(ctx context.Context) error {
	id, err := calendarUser(ctx)
	if err != nil {
		return err
	}
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", id); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "UPDATE calendar_connections SET status='disconnected',token_ciphertext=NULL,expires_at=NULL,last_error='' WHERE user_id=$1 AND provider='yandex'", id)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "DELETE FROM calendar_outbox WHERE user_id=$1", id)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (d *Database) CalendarUsers(ctx context.Context) ([]string, error) {
	rows, err := d.DB.QueryContext(ctx, "SELECT user_id FROM calendar_connections WHERE provider='yandex' AND status IN ('pending','connected','error') ORDER BY user_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
func (d *Database) WithCalendarLock(ctx context.Context, fn func() error) error {
	id, err := calendarUser(ctx)
	if err != nil {
		return err
	}
	conn, err := d.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var locked bool
	if err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,2))", id).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return &domain.AppError{Code: "calendar_busy", Message: "Синхронизация уже выполняется", Status: 409}
	}
	defer func() {
		unlock, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(unlock, "SELECT pg_advisory_unlock(hashtextextended($1,2))", id); err != nil {
			// Do not return a session holding a lock to the connection pool.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	return fn()
}
