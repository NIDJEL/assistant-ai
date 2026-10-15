import type { RhythmEvent } from '../../types/api';

export function notificationKey(event: RhythmEvent) {
  const seconds = event.start_second ? `:${event.start_second}` : '';
  const occurrence = event.occurrence_at ? `:${event.occurrence_at}` : '';
  return `${event.id}:${event.date}:${event.time}${seconds}:${event.reminder_minutes || 0}${occurrence}`;
}
export function notificationSupport() { return typeof window !== 'undefined' && 'Notification' in window && window.isSecureContext; }
export async function registerNotifications() {
  if (!navigator.serviceWorker) return undefined;
  await navigator.serviceWorker.register('/notifications-sw.js');
  const registration = await Promise.race([navigator.serviceWorker.ready, new Promise<never>((_, reject) => { const timer = setTimeout(() => reject(new Error('Notifications unavailable')), 10000); navigator.serviceWorker.ready.finally(() => clearTimeout(timer)); })]);
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
