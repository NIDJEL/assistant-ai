import { useEffect, useRef, useState } from 'react';
import { Bell, X } from 'lucide-react';
import { useAuth } from '../../app/AuthContext';
import { useData } from '../../app/DataContext';
import { useEventActions } from '../../app/EventActionsContext';
import { request } from '../../api/client';
import { Button, IconButton } from '../../components/UI';
import { dateLabel } from '../calendar/dates';
import type { RhythmEvent } from '../../types/api';
import { claimNotification, notificationKey } from './notifications';

export function ReminderNotifications() {
  const { user } = useAuth(); const { health, today, revision, refresh } = useData(); const actions = useEventActions();
  const [notices, setNotices] = useState<RhythmEvent[]>([]); const seen = useRef(new Set<string>());
  const activeUser = useRef(user?.id); activeUser.current = user?.id;
  useEffect(() => {
    if (!health || !user) return;
    const userId = user.id; const controller = new AbortController(); let polling = false;
    async function poll() {
      if (polling || controller.signal.aborted) return; polling = true;
      try {
        if (document.visibilityState === 'hidden') return;
        const data = await request<{ events: RhythmEvent[] }>('/api/reminders/due', { signal: controller.signal });
        let refreshNeeded = false;
        for (const event of data.events) {
          if (controller.signal.aborted) return;
          if (!seen.current.has(notificationKey(event))) refreshNeeded = true;
          if (await claimNotification(userId, event, seen.current)) {
            // A data refresh can restart polling while the storage lock is held.
            // Once claimed, show the notice for this user even if that poll ended.
            if (activeUser.current !== userId) return;
            setNotices(current => [...current, event].slice(-5));
          }
        }
        // The server completes one-shot reminders and advances recurring ones.
        // Sync even if another tab already claimed the visible notification.
        if (refreshNeeded && !controller.signal.aborted) await refresh();
      } catch { /* Transient failures retry on the next poll; no repeated alerts. */ }
      finally { polling = false; }
    }
    const focus = () => { void poll(); };
    void poll(); const timer = setInterval(focus, 30000); window.addEventListener('focus', focus); document.addEventListener('visibilitychange', focus);
    return () => { controller.abort(); clearInterval(timer); window.removeEventListener('focus', focus); document.removeEventListener('visibilitychange', focus); };
  }, [user?.id, health?.timezone, revision, refresh]);
  return <div className="reminder-notices" aria-label="Уведомления о событиях" aria-live="polite">{notices.map(event => <div className="reminder-notice" role="status" key={notificationKey(event)}><Bell size={22}/><div><strong>{event.display_title || event.title}</strong><p>{dateLabel(event.date, today)}, {event.time}</p><Button variant="ghost" onClick={() => actions.edit(event)}>Открыть событие</Button></div><IconButton icon={X} label={`Закрыть уведомление: ${event.display_title || event.title}`} onClick={() => setNotices(current => current.filter(item => notificationKey(item) !== notificationKey(event)))}/></div>)}</div>;
}
