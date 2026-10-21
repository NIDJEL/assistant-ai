import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { useState } from 'react';
import { ChatNotificationsProvider, useChatNotifications } from './ChatNotifications';
import type { Exchange } from '../../types/api';

vi.mock('../../app/AuthContext', () => ({ useAuth: () => ({ user: { id: 'alice' } }) }));
function Messages() {
  const { exchanges, markRead } = useChatNotifications();
  const [failed, setFailed] = useState(false);
  const read = (ids: string | string[]) => void markRead(ids).then(() => setFailed(false)).catch(() => setFailed(true));
  return <>{failed && <p role="alert">Нет сети</p>}<button onClick={() => read(exchanges.map(e => e.response.request_id))}>Прочитать все</button>{exchanges.map(e => <div key={e.response.request_id}>{e.response.reply}{!e.read && <button onClick={() => read(e.response.request_id)}>Прочитано</button>}</div>)}</>;
}
function notification(id = 'run-one'): Exchange {
  return { message: '', notification: true, read: false, created_at: '2026-10-18T16:00:00Z', response: { reply: `Встреча ${id}`, request_id: id, session_id: 'notifications', action: 'list', events: [], candidates: null, conflicts: null, provider: 'zoom_webhook', timezone: 'Europe/Moscow', pending: false } };
}
it('receives new persisted messages without reload and acknowledges explicit IDs', async () => {
  let messages: Exchange[] = [];
  const fetch = vi.fn(async (_path, init?: RequestInit) => {
    if (init?.method === 'POST') return new Response(null, { status: 204 });
    return new Response(JSON.stringify({ exchanges: messages }), { headers: { 'Content-Type': 'application/json' } });
  }); vi.stubGlobal('fetch', fetch);
  const view = render(<MemoryRouter initialEntries={['/meetings']}><ChatNotificationsProvider><Messages/></ChatNotificationsProvider></MemoryRouter>);
  await waitFor(() => expect(fetch).toHaveBeenCalledOnce());
  messages = [{ message: '', notification: true, read: false, created_at: '2026-10-18T16:00:00Z', response: { reply: 'Встреча началась', request_id: 'run-one', session_id: 'notifications', action: 'list', events: [], candidates: null, conflicts: null, provider: 'zoom_webhook', timezone: 'Europe/Moscow', pending: false } }];
  await act(async () => { window.dispatchEvent(new Event('focus')); });
  expect(await screen.findByText('Встреча началась')).toBeVisible(); expect(screen.getByRole('link')).toHaveTextContent('Новые сообщения Гены: 1');
  await userEvent.setup().click(screen.getByRole('button', { name: 'Прочитано' }));
  await waitFor(() => expect(screen.queryByRole('link')).toBeNull());
  expect(fetch).toHaveBeenCalledWith('/api/chat/notifications/read', expect.objectContaining({ method: 'POST', body: JSON.stringify({ ids: ['run-one'] }) }));
  view.unmount();
});
it('does not resurrect a read message when an older poll returns late', async () => {
  let finishPoll!: (value: Response) => void; let gets = 0;
  const fetch = vi.fn(async (_path, init?: RequestInit) => {
    if (init?.method === 'POST') return new Response(null, { status: 204 });
    if (++gets === 2) return new Promise<Response>(resolve => { finishPoll = resolve; });
    return new Response(JSON.stringify({ exchanges: [notification()] }));
  }); vi.stubGlobal('fetch', fetch);
  render(<MemoryRouter initialEntries={['/calendar']}><ChatNotificationsProvider><Messages/></ChatNotificationsProvider></MemoryRouter>);
  await screen.findByRole('link');
  await act(async () => { window.dispatchEvent(new Event('focus')); });
  await userEvent.setup().click(screen.getByRole('button', { name: 'Прочитано' }));
  await waitFor(() => expect(screen.queryByRole('link')).toBeNull());
  await act(async () => finishPoll(new Response(JSON.stringify({ exchanges: [notification()] }))));
  expect(screen.queryByRole('link')).toBeNull(); expect(screen.getByText('Встреча run-one')).toBeVisible();
});
it('retains unread messages after a failed acknowledgement and permits retry', async () => {
  let posts = 0;
  const fetch = vi.fn(async (_path, init?: RequestInit) => {
    if (init?.method === 'POST') { if (++posts === 1) throw new TypeError('offline'); return new Response(null, { status: 204 }); }
    return new Response(JSON.stringify({ exchanges: [notification()] }));
  }); vi.stubGlobal('fetch', fetch);
  render(<MemoryRouter initialEntries={['/calendar']}><ChatNotificationsProvider><Messages/></ChatNotificationsProvider></MemoryRouter>);
  expect(await screen.findByRole('link')).toHaveAttribute('href', '/#chat-message-run-one');
  const user = userEvent.setup(); await user.click(screen.getByRole('button', { name: 'Прочитано' }));
  await screen.findByRole('alert'); expect(screen.getByRole('link')).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'Прочитано' }));
  await waitFor(() => expect(screen.queryByRole('link')).toBeNull()); expect(posts).toBe(2);
});
it('batches acknowledgements within the API limit and deduplicates concurrent reads', async () => {
  const posts: string[][] = []; let release!: () => void;
  const gate = new Promise<void>(resolve => { release = resolve; });
  vi.stubGlobal('fetch', vi.fn(async (_path, init?: RequestInit) => {
    if (init?.method === 'POST') { posts.push(JSON.parse(String(init.body)).ids); await gate; return new Response(null, { status: 204 }); }
    return new Response(JSON.stringify({ exchanges: Array.from({ length: 205 }, (_, i) => notification(`id-${i}`)) }));
  }));
  render(<MemoryRouter initialEntries={['/calendar']}><ChatNotificationsProvider><Messages/></ChatNotificationsProvider></MemoryRouter>);
  await screen.findByRole('link'); const user = userEvent.setup();
  await user.click(screen.getByRole('button', { name: 'Прочитать все' }));
  await user.click(screen.getByRole('button', { name: 'Прочитать все' })); expect(posts).toHaveLength(1);
  await act(async () => release()); await waitFor(() => expect(screen.queryByRole('link')).toBeNull());
  expect(posts.map(ids => ids.length)).toEqual([200, 5]); expect(new Set(posts.flat()).size).toBe(205);
});
