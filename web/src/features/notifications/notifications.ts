import type { RhythmEvent } from '../../types/api';

export function notificationKey(event: RhythmEvent) {
  const seconds = event.start_second ? `:${event.start_second}` : '';
  const occurrence = event.occurrence_at ? `:${event.occurrence_at}` : '';
  return `${event.id}:${event.date}:${event.time}${seconds}:${event.reminder_minutes || 0}${occurrence}`;
}
export function notificationSupport() { return typeof window !== 'undefined' && 'Notification' in window && window.isSecureContext; }
export async function registerNotifications() {
  if (!('serviceWorker' in navigator)) return undefined;
  await navigator.serviceWorker.register('/notifications-sw.js');
  const registration = await navigator.serviceWorker.ready;
  const worker = registration.active;
  if (!worker) throw new Error('Notification worker is unavailable');
  if (worker.state !== 'activated') await new Promise<void>((resolve, reject) => {
    const changed = () => {
      if (worker.state === 'activated' || worker.state === 'redundant') {
        worker.removeEventListener('statechange', changed);
        if (worker.state === 'activated') resolve(); else reject(new Error('Notification worker was replaced'));
      }
    };
    worker.addEventListener('statechange', changed); changed();
  });
  return registration;
}
export async function showSystemNotification(event: RhythmEvent, userId: string, body: string, active: () => boolean = () => true) {
  if (!active() || !notificationSupport() || Notification.permission !== 'granted') return;
  const options: NotificationOptions = { body, tag: `rhythm-${userId}-${notificationKey(event)}`, data: { userId, url: `/calendar?date=${event.date}` } };
  try {
    const registration = await registerNotifications();
    if (!active()) return;
    if (registration) { await registration.showNotification(event.title, options); return; }
  } catch { /* Desktop constructor remains available if service workers fail. */ }
  if (!active()) return;
  try { const notice = new Notification(event.title, options); notice.onclick = () => { window.focus(); window.location.assign(options.data.url); notice.close(); }; }
  catch { /* The in-app notification is still visible on unsupported devices. */ }
}

// Locks serialize simultaneous polling tabs. Storage makes a delivery survive
// reloads while a moved event receives a new key and can notify again.
export async function claimNotification(userId: string, event: RhythmEvent, seen: Set<string>): Promise<boolean> {
  const claim = () => {
    const key = notificationKey(event); if (seen.has(key)) return false;
    const storageKey = `rhythm.notifications.v1.${userId}`;
    let stored: string[] = [];
    try { const value: unknown = JSON.parse(localStorage.getItem(storageKey) || '[]'); if (Array.isArray(value)) stored = value.filter((v): v is string => typeof v === 'string').slice(-200); } catch { /* Storage is optional. */ }
    if (stored.includes(key)) { seen.add(key); return false; }
    seen.add(key);
    try { localStorage.setItem(storageKey, JSON.stringify([...stored, key].slice(-200))); } catch { /* In-memory deduplication still works. */ }
    return true;
  };
  return navigator.locks ? navigator.locks.request(`rhythm.notifications.${userId}`, claim) : claim();
}
