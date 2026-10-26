package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

// Polling is read-only and scoped by authenticated owner. No delivery/sync is
// attempted here; existing workers remain the only background executors.
func (d *Database) ExecutionStatuses(ctx context.Context, ids []int64, now time.Time) ([]domain.ExecutionStatus, error) {
	uid, err := calendarUser(ctx)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) > 100 {
		ids = ids[:100]
	}
	result := []domain.ExecutionStatus{}
	err = d.Read(ctx, func(tx repository.Tx) error {
		r := tx.(*Repo)
		rows, err := r.q.QueryContext(ctx, `SELECT e.id,c.provider,CASE WHEN c.status='disconnected' OR (c.provider='google' AND c.calendar_id='') THEN 'disabled'
 WHEN c.status NOT IN ('connected','syncing') THEN 'failed'
 WHEN EXISTS(SELECT 1 FROM cloud_outbox o WHERE o.user_id=e.user_id AND o.event_id=e.id AND o.provider=c.provider) THEN CASE WHEN c.last_error<>'' THEN 'retry' ELSE 'pending' END
 WHEN e.date IS NULL THEN 'local_only'
 WHEN EXISTS(SELECT 1 FROM cloud_event_links l WHERE l.user_id=e.user_id AND l.event_id=e.id AND l.provider=c.provider) THEN 'synced' ELSE 'pending' END
 FROM events e JOIN cloud_connections c ON c.user_id=e.user_id
 WHERE e.user_id=$1 AND e.id=ANY($2::bigint[]) AND (c.provider='google' OR e.details::jsonb->>'conference_provider'='zoom' OR EXISTS(SELECT 1 FROM cloud_event_links l WHERE l.user_id=e.user_id AND l.event_id=e.id AND l.provider=c.provider))
 UNION ALL SELECT e.id,'yandex',CASE WHEN c.status='disconnected' THEN 'disabled' WHEN c.status NOT IN ('connected','syncing') THEN 'failed'
 WHEN EXISTS(SELECT 1 FROM calendar_outbox o WHERE o.user_id=e.user_id AND o.event_id=e.id) THEN CASE WHEN c.last_error<>'' THEN 'retry' ELSE 'pending' END WHEN e.date IS NULL THEN 'local_only' WHEN c.last_sync >= COALESCE(e.deleted_at,e.updated_at)::timestamptz THEN 'synced' ELSE 'pending' END
 FROM events e JOIN calendar_connections c ON c.user_id=e.user_id AND c.provider='yandex' WHERE e.user_id=$1 AND e.id=ANY($2::bigint[])`, uid, ids)
		if err != nil {
			return err
		}
		for rows.Next() {
			var s domain.ExecutionStatus
			if err = rows.Scan(&s.EventID, &s.Provider, &s.State); err != nil {
				rows.Close()
				return err
			}
			s.Message = executionMessage(s.Provider, s.State)
			result = append(result, s)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		rows, err = r.q.QueryContext(ctx, `SELECT e.id,coalesce(n.sent,0),coalesce(n.pending,0),coalesce(n.failed,0),coalesce(n.attempts,0),n.next_attempt,
 EXISTS(SELECT 1 FROM push_subscriptions p WHERE p.user_id=e.user_id AND p.disabled_at IS NULL)
 FROM events e LEFT JOIN LATERAL (
 SELECT count(*) FILTER(WHERE d.status='success') sent,count(*) FILTER(WHERE d.status IN ('pending','retry')) pending,
 count(*) FILTER(WHERE d.status IN ('failed','expired','dead')) failed,max(d.attempts) attempts,min(d.next_attempt_at) FILTER(WHERE d.status IN ('pending','retry')) next_attempt
 FROM notification_deliveries d WHERE d.user_id=e.user_id AND d.notification_id=(SELECT o.id FROM notification_occurrences o WHERE o.user_id=e.user_id AND o.event_id=e.id ORDER BY o.created_at DESC LIMIT 1)
 ) n ON true WHERE e.user_id=$1 AND e.id=ANY($2::bigint[]) AND e.date IS NOT NULL AND e.time IS NOT NULL AND e.deleted_at IS NULL
 AND (e.details::jsonb->>'reminder_enabled') IS DISTINCT FROM 'false' AND (e.kind='reminder' OR e.details::jsonb->>'reminder_minutes' IS NOT NULL OR e.details::jsonb->>'reminder_enabled'='true')`, uid, ids)
		if err != nil {
			return err
		}
		for rows.Next() {
			var s domain.ExecutionStatus
			var failed int
			var next sql.NullTime
			var enabled bool
			s.Provider = "web_push"
			if err = rows.Scan(&s.EventID, &s.AcceptedDevices, &s.PendingDevices, &failed, &s.Attempts, &next, &enabled); err != nil {
				rows.Close()
				return err
			}
			s.State = "scheduled"
			switch {
			case s.PendingDevices > 0 && s.Attempts > 0:
				s.State = "retry"
			case s.PendingDevices > 0:
				s.State = "pending"
			case s.AcceptedDevices > 0:
				s.State = "accepted"
			case failed > 0:
				s.State = "failed"
			case !enabled:
				s.State = "disabled"
			}
			if next.Valid {
				s.NextAttemptAt = &next.Time
			}
			s.Message = executionMessage(s.Provider, s.State)
			if s.AcceptedDevices > 0 {
				s.Message += fmt.Sprintf(" Принято для устройств: %d.", s.AcceptedDevices)
			}
			result = append(result, s)
		}
		err = rows.Err()
		rows.Close()
		return err
	})
	return result, err
}

func executionMessage(provider, state string) string {
	if provider == "web_push" {
		switch state {
		case "scheduled":
			return "Напоминание сохранено и ожидает времени отправки."
		case "accepted":
			return "Push-сервис принял последнее уведомление. Прочтение на устройстве не подтверждено."
		case "disabled":
			return "Уведомления на устройства не подключены."
		case "pending":
			return "Уведомление ожидает доставки."
		case "retry":
			return "Доставка уведомления будет повторена."
		default:
			return "Не удалось доставить уведомление. Проверьте настройки уведомлений."
		}
	}
	switch state {
	case "local_only":
		return "Сохранено в Гене. Для отправки в календарь нужна дата."
	case "synced":
		return "Изменение подтверждено внешним сервисом."
	case "pending":
		return "Сохранено в Гене. Отправка во внешний сервис ожидается."
	case "retry":
		return "Сохранено в Гене. Синхронизация будет повторена."
	case "disabled":
		return "Подключение внешнего сервиса отключено."
	default:
		return "Сохранено в Гене. Для синхронизации требуется проверить подключение."
	}
}
