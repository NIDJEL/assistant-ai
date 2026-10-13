import type { ChatInput, ChatResponse, EventInput, EventPatch, RhythmEvent } from '../types/api';
export const today = '2026-10-13';
export function fixture(id: number, kind: RhythmEvent['kind'], title: string, overrides: Partial<RhythmEvent> = {}): RhythmEvent {
  return { id, kind, title, description: '', date: today, time: kind === 'task' ? null : '10:00', duration_minutes: 60, priority: 'normal', status: 'open', location: '', meeting_url: '', created_at: '2026-10-13T10:00:00+03:00', updated_at: '2026-10-13T10:00:00+03:00', source_key: null, external_calendar_id: null, ...overrides };
}
export const initial = [fixture(1, 'task', 'Подготовить отчёт'), fixture(2, 'meeting', 'Обсуждение проекта'), fixture(3, 'reminder', 'Позвонить Ивану', { time: '08:00' }), fixture(4, 'task', 'Архивный договор', { date: '2026-10-01', status: 'done' })];
export function mockServer(options: { chatGate?: Promise<void>; historyGate?: Promise<void>; searchGate?: Promise<void>; preferences?: Record<string, unknown>; failFirstChat?: boolean } = {}) {
  let events = initial.map(e => ({ ...e })); const chats: ChatInput[] = []; const writes: { method: string; input: EventInput | EventPatch }[] = [];
  const handler = vi.fn(async (path: string | URL | Request, init: RequestInit = {}) => {
    const url = new URL(String(path), 'http://localhost'); const method = init.method || 'GET';
    const json = (data: unknown, status = 200) => new Response(JSON.stringify(data), { status, headers: { 'Content-Type': 'application/json' } });
    if (url.pathname === '/api/auth/me') return json({ user: { id: '11111111-1111-4111-8111-111111111111', name: 'Алексей', email: 'alex@example.test', avatar_url: '', onboarding_completed: true, providers: ['email'] } });
    if (url.pathname === '/health') return json({ backend: 'go', status: 'ok', now: '2026-10-13T12:30:00+03:00', timezone: 'Europe/Moscow', events: events.filter(event => !event.deleted_at).length, nlp_provider: 'rules', schema_version: 5 });
    if (url.pathname.startsWith('/api/history/')) { await options.historyGate; return json({ exchanges: [], state: {} }); }
    if (url.pathname.startsWith('/api/preferences/')) { if (method === 'PUT') return json(JSON.parse(String(init.body))); return json(options.preferences || {}); }
    if (url.pathname === '/api/chat') {
      const input = JSON.parse(String(init.body)) as ChatInput; chats.push(input); await options.chatGate;
      if (options.failFirstChat && chats.length === 1) throw new TypeError('offline');
      const response: ChatResponse = { reply: 'Готово. Задача сохранена.', action: 'create', events: [events[0]], candidates: null, conflicts: null, pending: false, provider: 'rules', timezone: 'Europe/Moscow', session_id: input.session_id, request_id: input.request_id };
      return json(response);
    }
    if (url.pathname === '/api/events' && method === 'GET') {
      const q = url.searchParams.get('q')?.toLowerCase() || ''; const matched = events.filter(event => !!event.deleted_at === (url.searchParams.get('deleted') === 'true') && (!q || event.title.toLowerCase().includes(q)));
      if (q) await options.searchGate;
      const offset = Number(url.searchParams.get('offset') || 0); return json({ events: matched.slice(offset, offset + 200), total: matched.length, limit: 200, offset });
    }
    if (url.pathname === '/api/events' && method === 'POST') {
      const input = JSON.parse(String(init.body)) as EventInput; writes.push({ method, input });
      if (input.title === 'Конфликтная встреча' && !input.force) return json({ error: { code: 'meeting_conflict', message: 'Время пересекается с другой встречей' }, conflicts: [events[1]] }, 409);
      const event = fixture(Math.max(...events.map(event => event.id)) + 1, input.kind, input.title, input); events.push(event); return json(event, 201);
    }
    if (url.pathname.endsWith('/restore') && method === 'POST') { const id = Number(url.pathname.split('/').at(-2)); const event = events.find(event => event.id === id && event.deleted_at); if (!event) return json({ error: { code: 'not_found', message: 'Не найдено' } }, 404); delete event.deleted_at; return json(event); }
    const id = Number(url.pathname.split('/').at(-1));
    if (method === 'PATCH') { const input = JSON.parse(String(init.body)); writes.push({ method, input }); events = events.map(event => event.id === id ? { ...event, ...input } : event); return json(events.find(event => event.id === id)); }
    if (method === 'DELETE') { events = events.map(event => event.id === id ? { ...event, deleted_at: new Date().toISOString() } : event); return new Response(null, { status: 204 }); }
    throw new Error(`Unexpected request ${method} ${url.pathname}`);
  });
  vi.stubGlobal('fetch', handler); return { handler, chats, writes, events: () => events };
}
