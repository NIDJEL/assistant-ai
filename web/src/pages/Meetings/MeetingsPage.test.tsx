import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { App } from '../../app/App';
import { fixture, mockServer } from '../../test/server';
import type { SharedMeeting } from '../../features/social/types';

const owner = { id: '11111111-1111-4111-8111-111111111111', name: 'Алексей', email: 'alex@example.test', avatar_url: '/alex.png', tags: [] };
const friend = { id: '22222222-2222-4222-8222-222222222222', name: 'Борис', avatar_url: '/boris.png', tags: [] };

it('shows friend photos, Zoom and the organizer as a participant without adding them to the editor', async () => {
  history.pushState({}, '', '/meetings'); const server = mockServer();
  server.events().splice(0, server.events().length, fixture(20, 'meeting', 'Созвон с Борисом', {
    meeting_url: 'https://us04web.zoom.us/j/123456789', organizer_id: owner.id,
    organizer_name: owner.name, organizer_avatar_url: owner.avatar_url,
    participants: [{ name: friend.name, avatar_url: friend.avatar_url }],
  }));
  render(<App/>);
  await screen.findByRole('heading', { name: 'Созвон с Борисом' });
  expect(screen.queryByText('Управляйте встречами и созвонами')).not.toBeInTheDocument();
  expect(screen.getByText('2 участника')).toBeInTheDocument();
  expect(screen.getAllByRole('img', { name: 'Zoom' })).toHaveLength(2);
  expect(screen.queryByText('us04web.zoom.us')).not.toBeInTheDocument();
  expect(document.querySelectorAll('.participant img')).toHaveLength(2);
  expect(document.querySelectorAll('.participant-avatars img')).toHaveLength(2);
  expect(document.querySelector('.participant img')).toHaveAttribute('src', owner.avatar_url);
  const guestPhoto = document.querySelector(`.participant img[src="${friend.avatar_url}"]`)!;
  fireEvent.error(guestPhoto);
  await waitFor(() => expect(document.querySelectorAll('.participant img')).toHaveLength(1));
  expect(document.querySelector('.participants')).toHaveTextContent('Б');
  await userEvent.setup().click(screen.getByRole('button', { name: 'Добавить заметку' }));
  const dialog = screen.getByRole('dialog');
  expect(within(dialog).getByLabelText('Участник 1')).toHaveValue(friend.name);
  expect(within(dialog).queryByLabelText('Участник 2')).not.toBeInTheDocument();
});

function invitation(status: SharedMeeting['members'][number]['status'] = 'pending'): SharedMeeting {
  return { event: fixture(30, 'meeting', 'Совместный созвон', { organizer_id: friend.id }), organizer: friend,
    members: [{ ...owner, status, version: 1 }], proposals: [], version: 1, cancelled: false, timezone: 'Europe/Moscow' };
}

function startInvitation({ linked = false, fail = false, status = 'pending' }: { linked?: boolean; fail?: boolean; status?: SharedMeeting['members'][number]['status'] } = {}) {
  const meeting = invitation(status); history.pushState({}, '', linked ? '/meetings?shared=30&view=all' : '/meetings');
  const server = mockServer(); const requests: unknown[] = [];
  vi.stubGlobal('fetch', async (path: string | URL | Request, init?: RequestInit) => {
    const url = new URL(String(path), 'http://localhost');
    const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
    if (url.pathname === '/api/social/meetings') return json({ meetings: [meeting] });
    if (url.pathname === '/api/social/operations') {
      const op = JSON.parse(String(init?.body)); requests.push(op);
      if (fail) return json({ error: { code: 'availability_conflict', message: 'У вас уже есть планы на это время' } }, 409);
      meeting.members[0].status = op.status;
      if (op.status === 'accepted') server.events().push({ ...meeting.event, organizer_name: friend.name, organizer_avatar_url: friend.avatar_url, participants: [owner] });
      return json({ message: op.status === 'accepted' ? 'Вы подтвердили участие' : 'Вы отказались от участия' });
    }
    return server.handler(path, init);
  });
  render(<App/>); return { meeting, requests, user: userEvent.setup() };
}

it.each(['accepted', 'declined'] as const)('closes the invitation and hides its summary after a successful %s response', async status => {
  const { user, requests } = startInvitation();
  await user.click(await screen.findByRole('button', { name: /Совместный созвон/ }));
  const dialog = screen.getByRole('dialog');
  expect(dialog.querySelectorAll('.shared-members > div')).toHaveLength(2);
  expect(dialog.querySelector('.shared-members > div')).toHaveTextContent('Организатор');
  await user.click(within(dialog).getByRole('button', { name: status === 'accepted' ? 'Приду' : 'Не приду' }));
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  expect(document.querySelector('.shared-meetings')).not.toBeInTheDocument();
  expect(requests).toHaveLength(1);
  expect(requests[0]).toMatchObject({ action: 'respond', status, event_id: 30 });
  expect(document.querySelector('.toast [role="status"]')).toHaveTextContent(status === 'accepted' ? 'Вы подтвердили участие' : 'Вы отказались от участия');
  if (status === 'accepted') expect(document.querySelector('.meeting-timeline')).toHaveTextContent('Совместный созвон');
});

it('clears the invitation deep link after responding and preserves unrelated query parameters', async () => {
  const { user } = startInvitation({ linked: true });
  const dialog = await screen.findByRole('dialog');
  await user.click(within(dialog).getByRole('button', { name: 'Приду' }));
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  expect(location.search).toBe('?view=all');
});

it('keeps the invitation open and pending when saving a response fails', async () => {
  const { user } = startInvitation({ fail: true, linked: true });
  const dialog = await screen.findByRole('dialog');
  await user.click(within(dialog).getByRole('button', { name: 'Приду' }));
  expect(await within(dialog).findByRole('alert')).toHaveTextContent('У вас уже есть планы');
  expect(dialog).toBeInTheDocument();
  expect(document.querySelector('.shared-meeting-summary.is-pending')).toBeInTheDocument();
});

it('does not show previously answered invitations in the shared summary', async () => {
  startInvitation({ status: 'accepted' });
  await screen.findByRole('heading', { level: 1, name: 'Встречи' });
  expect(document.querySelector('.shared-meetings')).not.toBeInTheDocument();
});
