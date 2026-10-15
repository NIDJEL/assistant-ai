package notifications

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type Worker struct {
	Store    Store
	Sender   Sender
	Location *time.Location
	Logger   *slog.Logger
	Interval time.Duration
}

func (w *Worker) Tick(ctx context.Context, now time.Time) error {
	if err := w.schedule(ctx, now); err != nil {
		return err
	}
	return w.deliver(ctx, now, false)
}
func (w *Worker) schedule(ctx context.Context, now time.Time) error {
	n, err := w.Store.ProcessDue(ctx, now, w.Location)
	if err != nil {
		return err
	}
	if n > 0 && w.Logger != nil {
		w.Logger.InfoContext(ctx, "occurrence claimed", "count", n)
	}
	return nil
}
func (w *Worker) deliver(ctx context.Context, now time.Time, realTime bool) error {
	if w.Sender == nil {
		return nil
	}
	// Bound each pass so cancellation and scheduling cannot be starved by a queue.
	for range 100 {
		if err := ctx.Err(); err != nil {
			return err
		}
		if realTime {
			now = time.Now()
		}
		result, err := w.Store.DeliverOne(ctx, now, w.Sender)
		if err != nil {
			return err
		}
		if result == nil {
			break
		}
		if w.Logger != nil {
			w.Logger.InfoContext(ctx, "delivery "+result.Status, "delivery_id", result.ID, "notification_id", result.NotificationID, "subscription_id", result.SubscriptionID, "attempts", result.Attempts, "http_status", result.HTTPStatus)
		}
	}
	return nil
}

func (w *Worker) Run(ctx context.Context) {
	interval := w.Interval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if w.Logger != nil {
		w.Logger.InfoContext(ctx, "notification worker started", "web_push_enabled", w.Sender != nil, "interval", interval.String())
	}
	// Network retries cannot delay scheduling the next reminder or recurrence.
	loop := func(fn func(context.Context, time.Time) error) {
		timer := time.NewTicker(interval)
		defer timer.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			if err := fn(ctx, time.Now()); err != nil && ctx.Err() == nil && w.Logger != nil {
				w.Logger.ErrorContext(ctx, "notification worker error", "error", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
		}
	}
	var wg sync.WaitGroup
	wg.Go(func() { loop(w.schedule) })
	loop(func(ctx context.Context, now time.Time) error { return w.deliver(ctx, now, true) })
	wg.Wait()
}
