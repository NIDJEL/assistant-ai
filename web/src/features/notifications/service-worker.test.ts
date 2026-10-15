import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';
import { IDBFactory } from 'fake-indexeddb';
const source = readFileSync('public/notifications-sw.js', 'utf8');
function worker(indexedDB = new IDBFactory()) {
  const handlers: Record<string, (event: unknown) => void> = {};
  const showNotification = vi.fn(async () => {}); const openWindow = vi.fn(async () => {});
  const existing = { url: 'https://gena.test/', navigate: vi.fn(async () => {}), focus: vi.fn(async () => {}) };
  const matchAll = vi.fn(async () => [existing]);
  runInNewContext(source, { indexedDB, URL, Date, self: { location: { origin: 'https://gena.test' }, registration: { showNotification }, clients: { claim: async () => {}, matchAll, openWindow }, skipWaiting: async () => {}, addEventListener: (type: string, handler: (event: unknown) => void) => { handlers[type] = handler; } } });
  async function dispatch(type: string, event: object) { let pending = Promise.resolve(); handlers[type]({ ...event, waitUntil: (promise: Promise<void>) => { pending = promise; } }); await pending; }
  const owner = () => dispatch('message', { data: { type: 'GENA_USER', userId: 'alice' }, source: { url: 'https://gena.test/' }, ports: [] });
  return { dispatch, owner, showNotification, existing, matchAll, openWindow, indexedDB };
}
const payload = () => ({ notification_id: 'notice-1', user_id: 'alice', event_id: 42, title: 'Гена', body: 'Снять чайник', url: '/calendar?event=42', kind: 'reminder', timestamp: new Date().toISOString(), expires_at: new Date(Date.now() + 60000).toISOString() });
it('shows a real push payload once across duplicate pushes and worker restarts', async () => {
  const first = worker(); await first.owner(); const p = payload();
  await Promise.all([first.dispatch('push', { data: { json: () => p } }), first.dispatch('push', { data: { json: () => p } })]);
  expect(first.showNotification).toHaveBeenCalledOnce(); expect(first.showNotification).toHaveBeenCalledWith('Гена', expect.objectContaining({ body: 'Снять чайник', data: expect.objectContaining({ eventId: 42 }) }));
  const restarted = worker(first.indexedDB); await restarted.dispatch('push', { data: { json: () => p } }); expect(restarted.showNotification).not.toHaveBeenCalled();
});
it('rejects expired, malformed and another account’s pushes', async () => {
  const w = worker(); await w.owner();
  for (const p of [{}, { ...payload(), user_id: 'bob' }, { ...payload(), expires_at: '2020-01-01' }]) await w.dispatch('push', { data: { json: () => p } });
  await w.dispatch('push', { data: { json: () => { throw new Error(); } } }); expect(w.showNotification).not.toHaveBeenCalled();
});
it('focuses and navigates an existing application window', async () => {
  const w = worker(); const close = vi.fn(); await w.dispatch('notificationclick', { notification: { close, data: { url: '/calendar?event=42' } } });
  expect(close).toHaveBeenCalledOnce(); expect(w.existing.navigate).toHaveBeenCalledWith('https://gena.test/calendar?event=42'); expect(w.existing.focus).toHaveBeenCalledOnce();
});
it('opens the app when closed and refuses external notification targets', async () => {
  const w = worker(); w.matchAll.mockResolvedValue([]);
  await w.dispatch('notificationclick', { notification: { close() {}, data: { url: '/' } } }); expect(w.openWindow).toHaveBeenCalledWith('https://gena.test/');
  w.openWindow.mockClear(); await w.dispatch('notificationclick', { notification: { close() {}, data: { url: 'https://evil.test/' } } }); expect(w.openWindow).not.toHaveBeenCalled();
});
it('stops notifications after logout', async () => { const w = worker(); await w.owner(); await w.dispatch('message', { data: { type: 'GENA_USER', userId: null }, source: { url: 'https://gena.test/' }, ports: [] }); await w.dispatch('push', { data: { json: payload } }); expect(w.showNotification).not.toHaveBeenCalled(); });
