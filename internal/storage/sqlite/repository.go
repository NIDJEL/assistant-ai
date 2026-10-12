package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

type executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
type Repo struct{ q executor }
type scanner interface{ Scan(...any) error }

const columns = "id,kind,title,date,time,duration_minutes,status,created_at,updated_at,source_key,description,priority,external_calendar_id"

func dateValue(v *domain.Date) any {
	if v == nil {
		return nil
	}
	return v.String()
}
func timeValue(v *domain.TimeOfDay) any {
	if v == nil {
		return nil
	}
	return v.String()
}
func scanEvent(row scanner) (*domain.Event, error) {
	var e domain.Event
	var kind, status, priority, created, updated string
	var day, clock, source, external sql.NullString
	var minutes int
	if err := row.Scan(&e.ID, &kind, &e.Title, &day, &clock, &minutes, &status, &created, &updated, &source, &e.Description, &priority, &external); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var err error
	if e.Kind, err = domain.ParseEventType(kind); err != nil {
		return nil, err
	}
	if e.Status, err = domain.ParseStatus(status); err != nil {
		return nil, err
	}
	if e.Priority, err = domain.ParsePriority(priority); err != nil {
		return nil, err
	}
	if day.Valid {
		if e.Date, err = domain.ParseDate(day.String); err != nil {
			return nil, err
		}
	}
	if clock.Valid {
		if e.StartTime, err = domain.ParseTime(clock.String); err != nil {
			return nil, err
		}
	}
	if e.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return nil, err
	}
	if e.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return nil, err
	}
	if source.Valid {
		e.SourceKey = &source.String
	}
	if external.Valid {
		e.ExternalCalendarID = &external.String
	}
	e.Duration = time.Duration(minutes) * time.Minute
	return &e, nil
}
func (r *Repo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.q.QueryRowContext(ctx, "SELECT count(*) FROM events").Scan(&n)
	return n, err
}
func (r *Repo) Get(ctx context.Context, id int64) (*domain.Event, error) {
	return scanEvent(r.q.QueryRowContext(ctx, "SELECT "+columns+" FROM events WHERE id=?", id))
}
func (r *Repo) Events(ctx context.Context, f domain.Filter) ([]domain.Event, error) {
	where := []string{}
	args := []any{}
	if f.Kind != domain.EventUnknown {
		where = append(where, "kind=?")
		args = append(args, f.Kind.String())
	}
	if f.Status != domain.StatusUnknown {
		where = append(where, "status=?")
		args = append(args, f.Status.String())
	}
	if f.Start != nil {
		where = append(where, "date>=?")
		args = append(args, f.Start.String())
	}
	if f.End != nil {
		where = append(where, "date<?")
		args = append(args, f.End.String())
	}
	if f.Upcoming != nil {
		where = append(where, "(date>=? OR date IS NULL)")
		args = append(args, f.Upcoming.String())
	}
	query := "SELECT " + columns + " FROM events"
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY date IS NULL,date,time,id"
	rows, err := r.q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []domain.Event{}
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, *e)
	}
	return events, rows.Err()
}
func (r *Repo) Create(ctx context.Context, e domain.Event, now time.Time) (*domain.Event, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	stamp := now.Format(time.RFC3339Nano)
	result, err := r.q.ExecContext(ctx, "INSERT INTO events(kind,title,date,time,duration_minutes,status,created_at,updated_at,source_key,description,priority,external_calendar_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(source_key) DO NOTHING", e.Kind.String(), e.Title, dateValue(e.Date), timeValue(e.StartTime), int(e.Duration/time.Minute), e.Status.String(), stamp, stamp, e.SourceKey, e.Description, e.Priority.String(), e.ExternalCalendarID)
	if err != nil {
		return nil, err
	}
	n, err := result.RowsAffected()
	if err != nil || n == 0 {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, id)
}
func (r *Repo) Update(ctx context.Context, e domain.Event, now time.Time) (*domain.Event, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	result, err := r.q.ExecContext(ctx, "UPDATE events SET title=?,date=?,time=?,duration_minutes=?,status=?,description=?,priority=?,external_calendar_id=?,updated_at=? WHERE id=?", e.Title, dateValue(e.Date), timeValue(e.StartTime), int(e.Duration/time.Minute), e.Status.String(), e.Description, e.Priority.String(), e.ExternalCalendarID, now.Format(time.RFC3339Nano), e.ID)
	if err != nil {
		return nil, err
	}
	n, err := result.RowsAffected()
	if err != nil || n == 0 {
		return nil, err
	}
	return r.Get(ctx, e.ID)
}
func (r *Repo) Delete(ctx context.Context, id int64) error {
	_, err := r.q.ExecContext(ctx, "DELETE FROM events WHERE id=?", id)
	return err
}
func (r *Repo) State(ctx context.Context, id string) (domain.State, error) {
	var state domain.State
	var raw string
	err := r.q.QueryRowContext(ctx, "SELECT state FROM sessions WHERE id=?", id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	err = json.Unmarshal([]byte(raw), &state)
	return state, err
}
func (r *Repo) SaveState(ctx context.Context, id string, state domain.State, now time.Time) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = r.q.ExecContext(ctx, "INSERT INTO sessions VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET state=excluded.state,updated_at=excluded.updated_at", id, string(raw), now.Format(time.RFC3339Nano))
	return err
}
func (r *Repo) Cached(ctx context.Context, sid, rid string) (*domain.Exchange, error) {
	var x domain.Exchange
	var raw, stamp string
	err := r.q.QueryRowContext(ctx, "SELECT message,response,created_at FROM exchanges WHERE session_id=? AND request_id=?", sid, rid).Scan(&x.Message, &raw, &stamp)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(raw), &x.Response); err != nil {
		return nil, err
	}
	x.CreatedAt, err = time.Parse(time.RFC3339Nano, stamp)
	return &x, err
}
func (r *Repo) SaveExchange(ctx context.Context, sid, rid, message string, response domain.Response, now time.Time) error {
	raw, err := json.Marshal(response)
	if err != nil {
		return err
	}
	_, err = r.q.ExecContext(ctx, "INSERT INTO exchanges(session_id,request_id,message,response,created_at) VALUES(?,?,?,?,?)", sid, rid, message, string(raw), now.Format(time.RFC3339Nano))
	return err
}
func (r *Repo) History(ctx context.Context, id string, limit int) ([]domain.Exchange, error) {
	rows, err := r.q.QueryContext(ctx, "SELECT message,response,created_at FROM exchanges WHERE session_id=? ORDER BY id DESC LIMIT ?", id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.Exchange{}
	for rows.Next() {
		var x domain.Exchange
		var raw, stamp string
		if err = rows.Scan(&x.Message, &raw, &stamp); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &x.Response); err != nil {
			return nil, err
		}
		if x.CreatedAt, err = time.Parse(time.RFC3339Nano, stamp); err != nil {
			return nil, err
		}
		result = append(result, x)
	}
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result, rows.Err()
}
