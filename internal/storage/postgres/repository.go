package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
)

type executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
type Repo struct {
	q      executor
	userID string
}
type scanner interface{ Scan(...any) error }

const columns = "id,kind,title,date,time,duration_minutes,status,created_at,updated_at,source_key,description,priority,external_calendar_id,location,meeting_url,details,deleted_at"

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
	var day, clock, source, external, deleted sql.NullString
	var details string
	var minutes int
	if err := row.Scan(&e.ID, &kind, &e.Title, &day, &clock, &minutes, &status, &created, &updated, &source, &e.Description, &priority, &external, &e.Location, &e.MeetingURL, &details, &deleted); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(details), &e.EventDetails); err != nil {
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
	if deleted.Valid {
		stamp, err := time.Parse(time.RFC3339Nano, deleted.String)
		if err != nil {
			return nil, err
		}
		e.DeletedAt = &stamp
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
	err := r.q.QueryRowContext(ctx, "SELECT count(*) FROM events WHERE user_id=$1 AND deleted_at IS NULL", r.userID).Scan(&n)
	return n, err
}
func (r *Repo) Get(ctx context.Context, id int64) (*domain.Event, error) {
	return scanEvent(r.q.QueryRowContext(ctx, "SELECT "+columns+" FROM events WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL", id, r.userID))
}
func (r *Repo) GetDeleted(ctx context.Context, id int64) (*domain.Event, error) {
	return scanEvent(r.q.QueryRowContext(ctx, "SELECT "+columns+" FROM events WHERE id=$1 AND user_id=$2 AND deleted_at IS NOT NULL", id, r.userID))
}
func (r *Repo) Restore(ctx context.Context, id int64, now time.Time) (*domain.Event, error) {
	_, err := r.q.ExecContext(ctx, "UPDATE events SET deleted_at=NULL,updated_at=$1 WHERE id=$2 AND user_id=$3 AND deleted_at IS NOT NULL", now.Format(time.RFC3339Nano), id, r.userID)
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, id)
}
func (r *Repo) Events(ctx context.Context, f domain.Filter) ([]domain.Event, error) {
	where := []string{"deleted_at IS NULL", "user_id=?"}
	if f.Deleted {
		where[0] = "deleted_at IS NOT NULL"
	}
	args := []any{r.userID}
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
	rows, err := r.q.QueryContext(ctx, placeholders(query), args...)
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
	details, err := json.Marshal(e.EventDetails)
	if err != nil {
		return nil, err
	}
	stamp := now.Format(time.RFC3339Nano)
	var id int64
	err = r.q.QueryRowContext(ctx, "INSERT INTO events(kind,title,date,time,duration_minutes,status,created_at,updated_at,source_key,description,priority,external_calendar_id,location,meeting_url,details,user_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) ON CONFLICT(user_id,source_key) DO NOTHING RETURNING id", e.Kind.String(), e.Title, dateValue(e.Date), timeValue(e.StartTime), int(e.Duration/time.Minute), e.Status.String(), stamp, stamp, e.SourceKey, e.Description, e.Priority.String(), e.ExternalCalendarID, e.Location, e.MeetingURL, string(details), r.userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, id)
}
func (r *Repo) Update(ctx context.Context, e domain.Event, now time.Time) (*domain.Event, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	details, err := json.Marshal(e.EventDetails)
	if err != nil {
		return nil, err
	}
	result, err := r.q.ExecContext(ctx, "UPDATE events SET title=$1,date=$2,time=$3,duration_minutes=$4,status=$5,description=$6,priority=$7,external_calendar_id=$8,location=$9,meeting_url=$10,details=$11,updated_at=$12 WHERE id=$13 AND user_id=$14 AND deleted_at IS NULL", e.Title, dateValue(e.Date), timeValue(e.StartTime), int(e.Duration/time.Minute), e.Status.String(), e.Description, e.Priority.String(), e.ExternalCalendarID, e.Location, e.MeetingURL, string(details), now.Format(time.RFC3339Nano), e.ID, r.userID)
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
	_, err := r.q.ExecContext(ctx, "UPDATE events SET deleted_at=$1 WHERE id=$2 AND user_id=$3 AND deleted_at IS NULL", time.Now().UTC().Format(time.RFC3339Nano), id, r.userID)
	return err
}
func (r *Repo) State(ctx context.Context, id string) (domain.State, error) {
	var state domain.State
	var raw string
	err := r.q.QueryRowContext(ctx, "SELECT state FROM dialogue_sessions WHERE id=$1 AND user_id=$2", id, r.userID).Scan(&raw)
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
	_, err = r.q.ExecContext(ctx, "INSERT INTO dialogue_sessions(id,state,updated_at,user_id) VALUES($1,$2,$3,$4) ON CONFLICT(user_id,id) DO UPDATE SET state=excluded.state,updated_at=excluded.updated_at", id, string(raw), now.Format(time.RFC3339Nano), r.userID)
	return err
}
func (r *Repo) Cached(ctx context.Context, sid, rid string) (*domain.Exchange, error) {
	var x domain.Exchange
	var raw, stamp string
	err := r.q.QueryRowContext(ctx, "SELECT message,response,created_at FROM dialogue_exchanges WHERE session_id=$1 AND request_id=$2 AND user_id=$3", sid, rid, r.userID).Scan(&x.Message, &raw, &stamp)
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
	_, err = r.q.ExecContext(ctx, "INSERT INTO dialogue_exchanges(session_id,request_id,message,response,created_at,user_id) VALUES($1,$2,$3,$4,$5,$6)", sid, rid, message, string(raw), now.Format(time.RFC3339Nano), r.userID)
	return err
}
func (r *Repo) History(ctx context.Context, id string, limit int) ([]domain.Exchange, error) {
	rows, err := r.q.QueryContext(ctx, "SELECT message,response,created_at FROM dialogue_exchanges WHERE session_id=$1 AND user_id=$2 ORDER BY id DESC LIMIT $3", id, r.userID, limit)
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

func (r *Repo) Preferences(ctx context.Context, id string) (json.RawMessage, error) {
	var raw string
	err := r.q.QueryRowContext(ctx, "SELECT value::text FROM preferences WHERE id=$1 AND user_id=$2", id, r.userID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return json.RawMessage(`{}`), nil
	}
	return json.RawMessage(raw), err
}
func (r *Repo) SavePreferences(ctx context.Context, id string, value json.RawMessage, now time.Time) error {
	stamp := now.Format(time.RFC3339Nano)
	result, err := r.q.ExecContext(ctx, "INSERT INTO preferences(id,value,updated_at,user_id) VALUES($1,$2,$3,$4) ON CONFLICT(user_id,id) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at WHERE coalesce((excluded.value->>'revision')::bigint,0)>=coalesce((preferences.value->>'revision')::bigint,0)", id, string(value), stamp, r.userID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil || n == 0 {
		return err
	}
	_, err = r.q.ExecContext(ctx, "INSERT INTO widget_layouts(user_id,value,updated_at) VALUES($1,coalesce($2::jsonb->'layouts','{}'::jsonb),$3) ON CONFLICT(user_id) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at", r.userID, string(value), stamp)
	return err
}
func placeholders(query string) string {
	var out strings.Builder
	n := 0
	for _, c := range query {
		if c == '?' {
			n++
			fmt.Fprintf(&out, "$%d", n)
		} else {
			out.WriteRune(c)
		}
	}
	return out.String()
}
