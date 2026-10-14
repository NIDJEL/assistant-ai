import { claimNotification, notificationKey } from './notifications';
import { fixture } from '../../test/server';

beforeEach(() => localStorage.clear());
it('deduplicates a recurring replay and distinguishes UTC instants in a DST fold', async () => {
  const event = fixture(8, 'reminder', 'Размяться', { time: '02:30', recurrence: { frequency: 'hour', interval: 1, timezone: 'Europe/Berlin' }, occurrence_at: '2026-10-25T00:30:00Z' });
  expect(await claimNotification('alice', event, new Set())).toBe(true);
  expect(await claimNotification('alice', event, new Set())).toBe(false);
  expect(await claimNotification('alice', { ...event, occurrence_at: '2026-10-25T01:30:00Z' }, new Set())).toBe(true);
});
it('deduplicates across reloads and scopes notification history to the account', async () => {
  const event = fixture(5, 'meeting', 'Пара', { time: '12:50', reminder_minutes: 50 });
  expect(await claimNotification('alice', event, new Set())).toBe(true);
  expect(await claimNotification('alice', event, new Set())).toBe(false);
  expect(await claimNotification('bob', event, new Set())).toBe(true);
  expect(await claimNotification('alice', { ...event, time: '13:50' }, new Set())).toBe(true);
  expect(notificationKey(event)).not.toBe(notificationKey({ ...event, reminder_minutes: 60 }));
  const precise = fixture(6, 'reminder', 'Через 30 секунд', { time: '12:50', start_second: 15 });
  expect(await claimNotification('alice', precise, new Set())).toBe(true);
  expect(await claimNotification('alice', precise, new Set())).toBe(false);
  expect(await claimNotification('alice', { ...precise, start_second: 45 }, new Set())).toBe(true);
});
it('recovers from invalid storage and bounds retained notification history', async () => {
  const storageKey = 'rhythm.notifications.v1.alice';
  localStorage.setItem(storageKey, 'broken json');
  expect(await claimNotification('alice', fixture(1, 'reminder', 'Встреча'), new Set())).toBe(true);
  localStorage.setItem(storageKey, JSON.stringify(Array.from({ length: 250 }, (_, i) => String(i))));
  await claimNotification('alice', fixture(2, 'reminder', 'Пара'), new Set());
  expect(JSON.parse(localStorage.getItem(storageKey)!)).toHaveLength(200);
});
