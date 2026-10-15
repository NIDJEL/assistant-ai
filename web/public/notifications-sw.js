self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', event => event.waitUntil(self.clients.claim()));

function database() {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open('gena-notifications', 1);
    request.onupgradeneeded = () => request.result.createObjectStore('state');
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
}
async function setUser(userId) {
  const db = await database();
  try { await new Promise((resolve, reject) => {
    const tx = db.transaction('state', 'readwrite');
    tx.objectStore('state').put(userId, 'user');
    tx.oncomplete = resolve; tx.onerror = () => reject(tx.error); tx.onabort = () => reject(tx.error);
  }); } finally { db.close(); }
}
// One read/write transaction serializes simultaneous pushes across SW restarts.
async function claim(payload) {
  const db = await database();
  try { return await new Promise((resolve, reject) => {
    const tx = db.transaction('state', 'readwrite'); const store = tx.objectStore('state'); let accepted = false;
    const owner = store.get('user');
    owner.onsuccess = () => {
      if (owner.result !== payload.user_id) return;
      const previous = store.get(`notice:${payload.notification_id}`);
      previous.onsuccess = () => { if (!previous.result) { accepted = true; store.put(payload.expires_at, `notice:${payload.notification_id}`); } };
    };
    tx.oncomplete = () => resolve(accepted); tx.onerror = () => reject(tx.error); tx.onabort = () => reject(tx.error);
  }); } finally { db.close(); }
}
self.addEventListener('message', event => {
  if (event.data?.type !== 'GENA_USER' || !event.source?.url || new URL(event.source.url).origin !== self.location.origin) return;
  const userId = typeof event.data.userId === 'string' ? event.data.userId : null;
  event.waitUntil(setUser(userId).then(() => event.ports[0]?.postMessage('ok')));
});
self.addEventListener('push', event => {
  event.waitUntil((async () => {
    let payload;
    try { payload = event.data?.json(); } catch { return; }
    if (!payload || typeof payload.notification_id !== 'string' || typeof payload.user_id !== 'string' || typeof payload.body !== 'string') return;
    if (!Number.isFinite(Date.parse(payload.expires_at)) || Date.parse(payload.expires_at) <= Date.now()) return;
    if (!await claim(payload)) return;
    await self.registration.showNotification('Гена', {
      body: payload.body, tag: `gena-${payload.notification_id}`, renotify: false,
      icon: '/icons/gena-192.png', badge: '/icons/gena-192.png',
      timestamp: Date.parse(payload.timestamp),
      data: { notificationId: payload.notification_id, userId: payload.user_id, eventId: payload.event_id, url: payload.url, kind: payload.kind },
    });
  })());
});
self.addEventListener('pushsubscriptionchange', event => {
  // Renew only after an explicit action if the browser or session requires it.
  event.waitUntil(self.clients.matchAll({ type: 'window' }).then(windows => {
    for (const client of windows) client.postMessage({ type: 'GENA_PUSH_INVALID' });
  }));
});
self.addEventListener('notificationclick', event => {
  event.notification.close();
  const target = new URL(event.notification.data?.url || '/', self.location.origin);
  if (target.origin !== self.location.origin || !['/', '/calendar', '/settings'].includes(target.pathname)) return;
  event.waitUntil((async () => {
    const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
    const existing = windows.find(client => new URL(client.url).origin === target.origin);
    if (existing) { await existing.navigate(target.href); await existing.focus(); }
    else await self.clients.openWindow(target.href);
  })());
});
