import type { Page } from '@playwright/test';
import type { RhythmEvent } from '../src/types/api';

export async function mockSchedule(page: Page) {
  const user = { id: '11111111-1111-4111-8111-111111111111', name: 'Алексей Тест', email: 'alex@example.test', avatar_url: '', onboarding_completed: true, providers: ['email'] };
  const event = (id: number, kind: RhythmEvent['kind'], title: string, date: string, time: string): RhythmEvent => ({ id, kind, title, date, time, description: '', duration_minutes: 60, priority: 'normal', status: 'open', location: '', meeting_url: '', created_at: '2026-10-21T10:00:00+03:00', updated_at: '2026-10-21T10:00:00+03:00', source_key: null, external_calendar_id: null });
  const events = Array.from({ length: 35 }, (_, i) => event(i + 1, 'meeting', `Встреча ${i + 1}`, new Date(Date.UTC(2026, 9, 21 + i)).toISOString().slice(0, 10), '10:00'));
  const times = ['08:00', '08:30', '09:00', '15:45', '13:30', '14:00', '20:15', '18:00', '19:00'];
  const dates = ['2026-10-21', '2027-01-27', '2026-12-19', '2026-11-20', '2027-01-18', '2027-02-10', '2027-01-09', '2027-02-12', '2026-12-20'];
  times.forEach((time, i) => events.push({ ...event(100 + i, 'reminder', `Напоминание ${i + 1} с подробным названием`, dates[i], time), priority: i % 3 === 0 ? 'high' : 'normal', reminder_enabled: true, ...(i === 0 ? { recurrence: { frequency: 'day', interval: 1, local_time: time, timezone: 'Europe/Moscow' } as const } : {}) }));
  let preferences = { name: user.name, widgets: false, navCollapsed: true, motion: 'system', revision: 1 };
  events.push(event(201, 'task', 'Обычная задача', '2026-10-21', ''));
  events.push({ ...event(202, 'task', 'Важная задача', '2026-10-21', ''), priority: 'high' });
  await page.route(url => url.pathname === '/health' || url.pathname.startsWith('/api/'), async route => {
    const url = new URL(route.request().url()); let body: unknown = {};
    if (url.pathname === '/api/auth/me') body = { user };
    else if (url.pathname.startsWith('/api/preferences/')) { if (route.request().method() === 'PUT') preferences = { ...preferences, ...route.request().postDataJSON() }; body = preferences; }
    else if (url.pathname === '/health') body = { backend: 'go', status: 'ok', now: '2026-10-21T12:30:00+03:00', timezone: 'Europe/Moscow', events: events.length, nlp_provider: 'rules', schema_version: 5 };
    else if (url.pathname === '/api/events') { const q = (url.searchParams.get('q') || '').toLowerCase(); const found = events.filter(e => e.title.toLowerCase().includes(q)); body = { events: found, total: found.length, limit: 200, offset: 0 }; }
    else if (url.pathname === '/api/social/people') body = { profile: { ...user, code: 'ABCD-EFGH', share_email: false, timezone: 'Europe/Moscow', start_hour: 9, end_hour: 21, tags: [] }, friends: [], incoming: [], outgoing: [] };
    else if (url.pathname === '/api/social/meetings') body = { meetings: [] };
    else if (url.pathname === '/api/integrations') body = { connections: ['yandex', 'google', 'zoom'].map(provider => ({ provider, status: 'connected', last_sync: null, last_error: '', pending_events: 0 })), yandex_configured: true, google_configured: true, zoom_configured: true };
    else if (url.pathname === '/api/voice/voices') body = { voices: [{ id: 'ru-RU-DmitryNeural', name: 'Гена, мужской' }], default_voice: 'ru-RU-DmitryNeural' };
    else if (url.pathname === '/api/integrations/google/calendars') body = { calendars: Array.from({ length: 45 }, (_, index) => ({ id: `calendar-${index}`, summary: `Календарь ${index + 1}: планы и встречи нашей команды`, primary: index === 0 })), selected: 'calendar-0' };
    else if (url.pathname === '/api/reminders/due') body = { events: [] };
    else if (url.pathname.includes('/history/') || url.pathname === '/api/chat/notifications') body = { exchanges: [], state: {} };
    else if (url.pathname === '/api/notifications/push/config') body = { enabled: false, public_key: '' };
    else if (url.pathname === '/api/notifications/push/subscriptions') body = { subscriptions: [] };
    await route.fulfill({ json: body });
  });
  return events;
}
