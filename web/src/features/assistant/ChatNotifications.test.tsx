import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { ChatNotificationsProvider, useChatNotifications } from './ChatNotifications';
import type { Exchange } from '../../types/api';

vi.mock('../../app/AuthContext', () => ({ useAuth: () => ({ user: { id: 'alice' } }) }));
function Messages() {
  const { exchanges, markRead } = useChatNotifications();
  return <>{exchanges.map(e => <div key={e.response.request_id}>{e.response.reply}{!e.read && <button onClick={() => void markRead(e.response.request_id)}>Прочитано</button>}</div>)}</>;
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
