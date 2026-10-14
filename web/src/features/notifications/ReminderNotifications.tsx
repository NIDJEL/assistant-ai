import { useEffect, useRef, useState } from 'react';
import { Bell, X } from 'lucide-react';
import { useAuth } from '../../app/AuthContext';
import { useData } from '../../app/DataContext';
import { usePreferences } from '../../app/PreferencesContext';
import { useEventActions } from '../../app/EventActionsContext';
import { request } from '../../api/client';
import { Button, IconButton } from '../../components/UI';
import { dateLabel } from '../calendar/dates';
import type { RhythmEvent } from '../../types/api';
import { claimNotification, notificationKey, notificationSupport, showSystemNotification } from './notifications';

export function ReminderNotifications() {
  const { user } = useAuth(); const { health, today, revision } = useData(); const { preferences } = usePreferences(); const actions = useEventActions();
  const [notices, setNotices] = useState<RhythmEvent[]>([]); const seen = useRef(new Set<string>()); const prefs = useRef(preferences); prefs.current = preferences;
  const mounted = useRef(true);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  useEffect(() => {
    if (!health || !user) return;
    const userId = user.id; const controller = new AbortController(); let polling = false;
    async function poll() {
      if (polling || controller.signal.aborted) return; polling = true;
      try {
        const system = prefs.current.browserNotifications && notificationSupport() && Notification.permission === 'granted';
        if (document.visibilityState === 'hidden' && !system) return;
        const data = await request<{ events: RhythmEvent[] }>('/api/reminders/due', { signal: controller.signal });
        for (const event of data.events) {
          if (controller.signal.aborted) return;
          if (await claimNotification(userId, event, seen.current)) {
            if (controller.signal.aborted) return;
            setNotices(current => [...current, event].slice(-5));
            if (system) void showSystemNotification(event, userId, `Событие ${event.date}, ${event.time}`, () => mounted.current).catch(() => {});
          }
        }
      } catch { /* Transient failures retry on the next poll; no repeated alerts. */ }
      finally { polling = false; }
    }
    const focus = () => { void poll(); };
    void poll(); const timer = setInterval(focus, 30000); window.addEventListener('focus', focus); document.addEventListener('visibilitychange', focus);
    return () => { controller.abort(); clearInterval(timer); window.removeEventListener('focus', focus); document.removeEventListener('visibilitychange', focus); };
  }, [user?.id, health?.timezone, revision]);
  return <div className="reminder-notices" aria-label="Уведомления о событиях" aria-live="polite">{notices.map(event => <div className="reminder-notice" role="status" key={notificationKey(event)}><Bell size={22}/><div><strong>{event.title}</strong><p>{dateLabel(event.date, today)}, {event.time}</p><Button variant="ghost" onClick={() => actions.edit(event)}>Открыть событие</Button></div><IconButton icon={X} label={`Закрыть уведомление: ${event.title}`} onClick={() => setNotices(current => current.filter(item => notificationKey(item) !== notificationKey(event)))}/></div>)}</div>;
}
