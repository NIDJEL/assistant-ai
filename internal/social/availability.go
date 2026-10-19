package social

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/google/uuid"
)

func validSlot(slot Slot, now time.Time) error {
	d := slot.End.Sub(slot.Start)
	if slot.Start.IsZero() || slot.Start.Before(now) || slot.Start.After(now.AddDate(0, 0, 90)) || d < 5*time.Minute || d > 24*time.Hour || slot.Start.Second() != 0 || slot.End.Second() != 0 {
		return domain.Validation("Выберите будущее время в пределах 90 дней и длительность от 5 минут до суток")
	}
	return nil
}
func anyBusy(a Availability) bool {
	for _, u := range a.Users {
		if u.Status == "busy" {
			return true
		}
	}
	return false
}
func overlaps(a, b Slot) bool { return a.Start.Before(b.End) && b.Start.Before(a.End) }
func MergeBusy(items []Slot) []Slot {
	sort.Slice(items, func(i, j int) bool { return items[i].Start.Before(items[j].Start) })
	result := []Slot{}
	for _, s := range items {
		if !s.End.After(s.Start) {
			continue
		}
		n := len(result)
		if n > 0 && !s.Start.After(result[n-1].End) {
			if s.End.After(result[n-1].End) {
				result[n-1].End = s.End
			}
		} else {
			result = append(result, s)
		}
	}
	return result
}

type calendarBusy struct {
	ID                 string
	Busy               []Slot
	Complete           bool
	Location           *time.Location
	StartHour, EndHour int
}

func (s *Session) busy(ctx context.Context, id string, from, to time.Time, exclude int64) (calendarBusy, error) {
	b := calendarBusy{ID: id, Busy: []Slot{}, Complete: true}
	var zone string
	err := s.Q.QueryRowContext(ctx, `SELECT planning_timezone,planning_start_hour,planning_end_hour FROM users WHERE id=$1`, id).Scan(&zone, &b.StartHour, &b.EndHour)
	if err != nil {
		return b, err
	}
	b.Location, err = time.LoadLocation(zone)
	if err != nil {
		return b, err
	}
	rows, err := s.Q.QueryContext(ctx, `SELECT date,time,duration_minutes,details FROM events e WHERE e.id<>$2 AND e.deleted_at IS NULL AND e.status='open' AND e.date IS NOT NULL AND (e.user_id=$1 OR EXISTS(SELECT 1 FROM meeting_participants p JOIN collaborative_meetings cm ON cm.event_id=p.event_id WHERE p.event_id=e.id AND p.user_id=$1 AND p.status='accepted' AND NOT cm.cancelled)) AND (e.kind='meeting' OR e.details::jsonb->>'calendar_provider' IN ('google','yandex')) AND (e.date<=$3 OR e.details::jsonb ? 'recurrence')`, id, exclude, to.In(s.Location).Format("2006-01-02"))
	if err != nil {
		return b, err
	}
	for rows.Next() {
		var day string
		var clock *string
		var minutes int
		var details string
		if err = rows.Scan(&day, &clock, &minutes, &details); err != nil {
			rows.Close()
			return b, err
		}
		date, e := domain.ParseDate(day)
		if e != nil {
			rows.Close()
			return b, e
		}
		var d domain.EventDetails
		if e = json.Unmarshal([]byte(details), &d); e != nil {
			rows.Close()
			return b, e
		}
		if d.GoogleTransparent {
			continue
		}
		if clock == nil {
			days := max(1, d.GoogleAllDayDays)
			start := date.At(domain.TimeOfDay{}, s.Location)
			slot := Slot{start, start.AddDate(0, 0, days)}
			if overlaps(slot, Slot{from, to}) {
				b.Busy = append(b.Busy, slot)
			}
			continue
		}
		t, e := domain.ParseTime(*clock)
		if e != nil {
			rows.Close()
			return b, e
		}
		start := date.At(*t, s.Location).Add(time.Duration(d.StartSecond) * time.Second)
		duration := time.Duration(minutes) * time.Minute
		if d.GoogleDurationSeconds > 0 {
			duration = time.Duration(d.GoogleDurationSeconds) * time.Second
		}
		if d.Recurrence != nil {
			next, e := d.Recurrence.Next(from.Add(-duration))
			for n := 0; e == nil && next.Before(to) && n < 5000; n++ {
				b.Busy = append(b.Busy, Slot{next, next.Add(duration)})
				next, e = d.Recurrence.Next(next)
			}
		} else if overlaps(Slot{start, start.Add(duration)}, Slot{from, to}) {
			b.Busy = append(b.Busy, Slot{start, start.Add(duration)})
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return b, err
	}
	// A connected external calendar is complete only after a recent successful
	// free/busy read covering the requested horizon. Outbound sync is not evidence.
	rows, err = s.Q.QueryContext(ctx, `SELECT provider,status FROM calendar_connections WHERE user_id=$1 AND provider='yandex' AND status<>'disconnected' UNION SELECT provider,status FROM cloud_connections WHERE user_id=$1 AND provider='google' AND status<>'disconnected'`, id)
	if err != nil {
		return b, err
	}
	providers := map[string]string{}
	for rows.Next() {
		var p, status string
		if err = rows.Scan(&p, &status); err != nil {
			rows.Close()
			return b, err
		}
		providers[p] = status
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return b, err
	}
	for provider, status := range providers {
		if status != "connected" {
			b.Complete = false
		}
		var raw string
		var complete bool
		err = s.Q.QueryRowContext(ctx, `SELECT busy,complete FROM availability_snapshots WHERE user_id=$1 AND provider=$2 AND starts_at<=$3 AND ends_at>=$4 AND checked_at>now()-interval '5 minutes'`, id, provider, from, to).Scan(&raw, &complete)
		if err != nil || !complete {
			b.Complete = false
			continue
		}
		var slots []BusySlot
		if json.Unmarshal([]byte(raw), &slots) != nil {
			b.Complete = false
			continue
		}
		for _, slot := range slots {
			if exclude == 0 || slot.EventID != exclude {
				b.Busy = append(b.Busy, slot.Slot)
			}
		}
	}
	b.Busy = MergeBusy(b.Busy)
	return b, nil
}
func (s *Session) Availability(ctx context.Context, ids []string, slot Slot, eventID int64, now time.Time) (Availability, error) {
	if err := validSlot(slot, now); err != nil {
		return Availability{}, err
	}
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return Availability{}, domain.Validation("Некорректный участник")
		}
		if id != s.UserID {
			ok, err := s.IsFriend(ctx, id)
			if err != nil {
				return Availability{}, err
			}
			if !ok {
				allowed := false
				if eventID > 0 {
					err = s.Q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM events e JOIN meeting_participants p ON p.event_id=e.id WHERE e.id=$1 AND ((e.user_id=$2 AND p.user_id=$3) OR (e.user_id=$3 AND p.user_id=$2)))`, eventID, s.UserID, id).Scan(&allowed)
					if err != nil {
						return Availability{}, err
					}
				}
				if !allowed {
					return Availability{}, problem(403, "friend_required", "Доступность доступна только для друзей и участников вашей встречи")
				}
			}
		}
	}
	return s.availability(ctx, append([]string{s.UserID}, ids...), slot, eventID, now)
}
func (s *Session) AvailabilityForMeeting(ctx context.Context, m Meeting, slot Slot, now time.Time) (Availability, error) {
	if m.Cancelled {
		return Availability{}, notFound()
	}
	if err := validSlot(slot, now); err != nil {
		return Availability{}, err
	}
	ids := []string{m.Organizer.ID}
	for _, p := range m.Members {
		if p.Status != "cancelled" {
			ids = append(ids, p.ID)
		}
	}
	return s.availability(ctx, ids, slot, m.Event.ID, now)
}
func (s *Session) availability(ctx context.Context, ids []string, slot Slot, exclude int64, now time.Time) (Availability, error) {
	result := Availability{Users: []AvailabilityUser{}, Slots: []Slot{}, Complete: true}
	calendars := []calendarBusy{}
	seen := map[string]bool{}
	// Search a bounded 14-day horizon with 15-minute steps and each member's zone.
	horizon := slot.Start.AddDate(0, 0, 14)
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		b, err := s.busy(ctx, id, slot.Start.Add(-24*time.Hour), horizon, exclude)
		if err != nil {
			return result, err
		}
		status := "free"
		if !b.Complete {
			status = "unknown"
		}
		for _, busy := range b.Busy {
			if overlaps(slot, busy) {
				status = "busy"
				break
			}
		}
		result.Users = append(result.Users, AvailabilityUser{id, status, b.Complete})
		result.Complete = result.Complete && b.Complete
		calendars = append(calendars, b)
	}
	start := slot.Start
	step := 15 * time.Minute
	start = start.Truncate(step)
	if start.Before(slot.Start) {
		start = start.Add(step)
	}
	for candidate := start; candidate.Add(slot.End.Sub(slot.Start)).Before(horizon) && len(result.Slots) < 6; candidate = candidate.Add(step) {
		c := Slot{candidate, candidate.Add(slot.End.Sub(slot.Start))}
		if c.Start.Before(now) {
			continue
		}
		free := true
		for _, b := range calendars {
			localStart, localEnd := c.Start.In(b.Location), c.End.In(b.Location)
			workStart := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), b.StartHour, 0, 0, 0, b.Location)
			workEnd := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), b.EndHour, 0, 0, 0, b.Location)
			if localStart.Before(workStart) || localEnd.After(workEnd) {
				free = false
				break
			}
			for _, busy := range b.Busy {
				if overlaps(c, busy) {
					free = false
					break
				}
			}
			if !free {
				break
			}
		}
		if free {
			result.Slots = append(result.Slots, c)
			candidate = c.End.Add(-step)
		}
	}
	return result, nil
}
