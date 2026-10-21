package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/social"
	"github.com/go-chi/chi/v5"
)

func (a *API) socialRoutes(r chi.Router) {
	r.Get("/api/social/people", a.socialPeople)
	r.Get("/api/social/meetings", a.socialMeetings)
	r.Post("/api/social/operations", a.socialOperation)
	r.Post("/api/social/availability", a.socialAvailability)
}
func (a *API) socialPeople(w http.ResponseWriter, r *http.Request) {
	var result social.People
	err := a.Calendar.Store.Read(r.Context(), func(tx repository.Tx) error {
		s, err := social.In(r.Context(), tx, a.Calendar.Location)
		if err != nil {
			return err
		}
		result, err = s.People(r.Context())
		return err
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, result)
}
func (a *API) socialMeetings(w http.ResponseWriter, r *http.Request) {
	var result []social.Meeting
	err := a.Calendar.Store.Read(r.Context(), func(tx repository.Tx) error {
		s, err := social.In(r.Context(), tx, a.Calendar.Location)
		if err != nil {
			return err
		}
		result, err = s.Meetings(r.Context(), tx)
		return err
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, map[string]any{"meetings": result})
}
func (a *API) socialRate(ctx context.Context, bucket string, limit int) error {
	var limited error
	err := a.Calendar.Store.Write(ctx, func(tx repository.Tx) error {
		s, e := social.In(ctx, tx, a.Calendar.Location)
		if e != nil {
			return e
		}
		e = s.Rate(ctx, bucket, limit)
		var app *domain.AppError
		if errors.As(e, &app) && app.Status == 429 {
			limited = e
			return nil
		}
		return e
	})
	if err != nil {
		return err
	}
	return limited
}
func (a *API) socialOperation(w http.ResponseWriter, r *http.Request) {
	var op social.Operation
	if err := decode(w, r, &op); err != nil {
		a.fail(w, r, err)
		return
	}
	var cached *social.Result
	cachedErr := a.Calendar.Store.Read(r.Context(), func(tx repository.Tx) error {
		s, e := social.In(r.Context(), tx, a.Calendar.Location)
		if e != nil {
			return e
		}
		cached, e = s.CachedOperation(r.Context(), op)
		return e
	})
	if cachedErr != nil {
		a.fail(w, r, cachedErr)
		return
	}
	if cached != nil {
		write(w, 200, cached)
		return
	}
	limit := 240
	bucket := "operations"
	if op.Action == "request" {
		limit = 20
		bucket = "codes"
	}
	if err := a.socialRate(r.Context(), bucket, limit); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.preparePlanningOperation(r.Context(), op); err != nil {
		a.fail(w, r, err)
		return
	}
	var result social.Result
	err := a.Calendar.Store.Write(r.Context(), func(tx repository.Tx) error {
		s, e := social.In(r.Context(), tx, a.Calendar.Location)
		if e != nil {
			return e
		}
		result, e = s.Execute(social.WithTx(r.Context(), tx), op, a.Clock.Now())
		return e
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, result)
}
func (a *API) socialAvailability(w http.ResponseWriter, r *http.Request) {
	var input struct {
		FriendIDs []string    `json:"friend_ids"`
		Slot      social.Slot `json:"slot"`
		EventID   int64       `json:"event_id"`
	}
	if err := decode(w, r, &input); err != nil {
		a.fail(w, r, err)
		return
	}
	if len(input.FriendIDs) > 30 {
		a.fail(w, r, domain.Validation("Выберите до 30 друзей"))
		return
	}
	if err := a.socialRate(r.Context(), "availability", 120); err != nil {
		a.fail(w, r, err)
		return
	}
	ids := input.FriendIDs
	check := func(tx repository.Tx) (social.Availability, error) {
		s, err := social.In(r.Context(), tx, a.Calendar.Location)
		if err != nil {
			return social.Availability{}, err
		}
		if input.EventID > 0 {
			m, e := s.Meeting(r.Context(), tx, input.EventID)
			if e != nil {
				return social.Availability{}, e
			}
			ids = []string{m.Organizer.ID}
			for _, p := range m.Members {
				if p.Status != "cancelled" {
					ids = append(ids, p.ID)
				}
			}
			return s.AvailabilityForMeeting(r.Context(), *m, input.Slot, a.Clock.Now())
		}
		return s.Availability(r.Context(), ids, input.Slot, 0, a.Clock.Now())
	}
	err := a.Calendar.Store.Read(r.Context(), func(tx repository.Tx) error { _, err := check(tx); return err })
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.refreshPlanning(r.Context(), append([]string{repository.UserID(r.Context())}, ids...), input.Slot.Start.Add(-24*time.Hour), input.Slot.Start.AddDate(0, 0, 14))
	var result social.Availability
	err = a.Calendar.Store.Read(r.Context(), func(tx repository.Tx) error { var err error; result, err = check(tx); return err })
	if err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 200, result)
}
func (a *API) preparePlanningOperation(ctx context.Context, op social.Operation) error {
	if op.Action != "create_meeting" && op.Action != "invite" && op.Action != "set_participants" && op.Action != "respond" && op.Action != "resolve_proposal" {
		return nil
	}
	ids := []string{}
	var slot social.Slot
	err := a.Calendar.Store.Read(ctx, func(tx repository.Tx) error {
		s, e := social.In(ctx, tx, a.Calendar.Location)
		if e != nil {
			return e
		}
		if op.Action == "create_meeting" {
			if op.Event == nil {
				return domain.Validation("Укажите встречу")
			}
			start, end, ok := op.Event.Interval(a.Calendar.Location)
			if !ok {
				return domain.Validation("Укажите дату и время")
			}
			slot = social.Slot{Start: start, End: end}
			ids = op.FriendIDs
			_, e = s.Availability(ctx, ids, slot, 0, a.Clock.Now())
			return e
		}
		m, e := s.Meeting(ctx, tx, op.EventID)
		if e != nil {
			if op.Action == "invite" || op.Action == "set_participants" {
				event, getErr := tx.Get(ctx, op.EventID)
				if getErr != nil {
					return getErr
				}
				if event != nil && event.OrganizerID == repository.UserID(ctx) {
					for _, uid := range op.FriendIDs {
						ok, e := s.IsFriend(ctx, uid)
						if e != nil {
							return e
						}
						if !ok {
							return &domain.AppError{Status: 403, Code: "friend_required", Message: "Приглашать можно только подтверждённых друзей"}
						}
					}
					start, end, _ := event.Interval(a.Calendar.Location)
					slot = social.Slot{Start: start, End: end}
					ids = op.FriendIDs
					return nil
				}
			}
			return e
		}
		start, end, _ := m.Event.Interval(a.Calendar.Location)
		slot = social.Slot{Start: start, End: end}
		ids = append(ids, m.Organizer.ID)
		for _, p := range m.Members {
			if p.Status != "cancelled" {
				ids = append(ids, p.ID)
			}
		}
		ids = append(ids, op.FriendIDs...)
		for _, uid := range op.FriendIDs {
			ok, e := s.IsFriend(ctx, uid)
			if e != nil {
				return e
			}
			if !ok {
				return &domain.AppError{Status: 403, Code: "friend_required", Message: "Приглашать можно только подтверждённых друзей"}
			}
		}
		if op.Action == "resolve_proposal" && op.Status == "accepted" {
			for _, p := range m.Proposals {
				if p.ID == op.ID && op.SlotIndex >= 0 && op.SlotIndex < len(p.Slots) {
					slot = p.Slots[op.SlotIndex]
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	a.refreshPlanning(ctx, append([]string{repository.UserID(ctx)}, ids...), slot.Start.Add(-24*time.Hour), slot.Start.AddDate(0, 0, 14))
	return nil
}

// External HTTP is outside domain transactions. Refresh uses only IDs previously
// authorized by a friendship or the user's meeting, with bounded concurrency.
func (a *API) refreshPlanning(ctx context.Context, ids []string, from, to time.Time) {
	ctx, cancelAll := context.WithTimeout(ctx, 12*time.Second)
	defer cancelAll()
	seen := map[string]bool{}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, uid := range ids {
		if seen[uid] {
			continue
		}
		seen[uid] = true
		wg.Add(1)
		go func(uid string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			job, cancel := context.WithTimeout(repository.WithUser(ctx, uid), 12*time.Second)
			defer cancel()
			for _, provider := range []string{"google", "yandex"} {
				connected := false
				_ = a.Calendar.Store.Read(job, func(tx repository.Tx) error {
					s, e := social.In(job, tx, a.Calendar.Location)
					if e != nil {
						return e
					}
					if provider == "google" {
						e = s.Q.QueryRowContext(job, `SELECT EXISTS(SELECT 1 FROM cloud_connections WHERE user_id=$1 AND provider='google' AND status<>'disconnected')`, uid).Scan(&connected)
					} else {
						e = s.Q.QueryRowContext(job, `SELECT EXISTS(SELECT 1 FROM calendar_connections WHERE user_id=$1 AND provider='yandex' AND status<>'disconnected')`, uid).Scan(&connected)
					}
					if e != nil {
						return e
					}
					return nil
				})
				if !connected {
					continue
				}
				var busy []social.BusySlot
				var e error
				if provider == "google" && a.Cloud != nil {
					busy, e = a.Cloud.PlanningBusy(job, from, to)
				} else if provider == "yandex" && a.CalendarSync != nil {
					busy, e = a.CalendarSync.PlanningBusy(job, from, to)
				} else {
					e = errors.New("provider unavailable")
				}
				saveCtx, saveCancel := context.WithTimeout(context.WithoutCancel(job), 2*time.Second)
				_ = a.Calendar.Store.Write(saveCtx, func(tx repository.Tx) error {
					s, err := social.In(saveCtx, tx, a.Calendar.Location)
					if err != nil {
						return err
					}
					raw := []social.BusySlot{}
					if e == nil {
						raw = busy
					}
					_, err = s.Q.ExecContext(saveCtx, `INSERT INTO availability_snapshots(user_id,provider,starts_at,ends_at,busy,complete,checked_at) VALUES($1,$2,$3,$4,$5,$6,now()) ON CONFLICT(user_id,provider) DO UPDATE SET starts_at=$3,ends_at=$4,busy=$5,complete=$6,checked_at=now()`, uid, provider, from, to, jsonString(raw), e == nil)
					return err
				})
				saveCancel()
			}
		}(uid)
	}
	wg.Wait()
}
func (a *API) prepareSharedPatch(ctx context.Context, id int64, patch calendar.Patch) error {
	if patch.Date == nil && patch.Time == nil && patch.Duration == nil {
		return nil
	}
	var ids []string
	var start time.Time
	err := a.Calendar.Store.Read(ctx, func(tx repository.Tx) error {
		e, err := tx.Get(ctx, id)
		if err != nil {
			return err
		}
		if e == nil || !e.Collaborative {
			return nil
		}
		if e.OrganizerID != repository.UserID(ctx) {
			return &domain.AppError{Status: 403, Code: "organizer_required", Message: "Только организатор может изменять встречу"}
		}
		if patch.Date != nil {
			e.Date = *patch.Date
		}
		if patch.Time != nil {
			e.StartTime = *patch.Time
		}
		if patch.Duration != nil {
			e.Duration = *patch.Duration
		}
		if err = e.Validate(); err != nil {
			return domain.Validation(err.Error())
		}
		s, err := social.In(ctx, tx, a.Calendar.Location)
		if err != nil {
			return err
		}
		m, err := s.Meeting(ctx, tx, id)
		if err != nil {
			return err
		}
		var end time.Time
		start, end, _ = e.Interval(a.Calendar.Location)
		if _, err = s.AvailabilityForMeeting(ctx, *m, social.Slot{Start: start, End: end}, a.Clock.Now()); err != nil {
			return err
		}
		ids = append(ids, m.Organizer.ID)
		for _, p := range m.Members {
			if p.Status != "cancelled" {
				ids = append(ids, p.ID)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(ids) > 0 {
		a.refreshPlanning(ctx, ids, start.Add(-24*time.Hour), start.AddDate(0, 0, 14))
	}
	return nil
}
func jsonString(v any) string { raw, _ := json.Marshal(v); return string(raw) }
