import { useState } from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { AuthProvider, useAuth } from './AuthContext';
import { releasePushOnLogout, setPushUser } from '../features/notifications/push';

vi.mock('../features/notifications/push', () => ({ releasePushOnLogout: vi.fn(), setPushUser: vi.fn() }));
const account = { id: 'alice', name: 'Анна', email: 'anna@example.test', avatar_url: '', onboarding_completed: true, providers: ['email'] };
function Session() {
  const { user, loading, logout } = useAuth();
  const [error, setError] = useState('');
  return <><p>{loading ? 'Проверяем сессию' : user ? user.email : 'Вход завершён'}</p>
    <button onClick={() => void logout().catch(e => setError(e.message))}>Выйти</button>
    {error && <p role="alert">{error}</p>}</>;
}
beforeEach(() => {
  vi.mocked(setPushUser).mockResolvedValue();
  vi.mocked(releasePushOnLogout).mockResolvedValue();
});
it.each([false, true])('logs out on the server and clears the account when push cleanup fails: %s', async fails => {
  if (fails) vi.mocked(releasePushOnLogout).mockRejectedValue(new Error('Push unavailable'));
  const fetch = vi.fn(async (path: string) => path === '/api/auth/me'
    ? new Response(JSON.stringify({ user: account })) : new Response(null, { status: 204 }));
  vi.stubGlobal('fetch', fetch);
  render(<AuthProvider><Session/></AuthProvider>);
  await screen.findByText(account.email);
  await userEvent.click(screen.getByRole('button', { name: 'Выйти' }));
  await screen.findByText('Вход завершён');
  expect(releasePushOnLogout).toHaveBeenCalledWith(account.id);
  expect(fetch).toHaveBeenCalledWith('/api/auth/logout', expect.objectContaining({ method: 'POST', credentials: 'same-origin' }));
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
});
it('keeps the account and reports a server logout failure so the user can retry', async () => {
  vi.stubGlobal('fetch', vi.fn(async (path: string) => path === '/api/auth/me'
    ? new Response(JSON.stringify({ user: account }))
    : new Response(JSON.stringify({ error: { message: 'Попробуйте снова' } }), { status: 503 })));
  render(<AuthProvider><Session/></AuthProvider>);
  await screen.findByText(account.email);
  await userEvent.click(screen.getByRole('button', { name: 'Выйти' }));
  await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('Попробуйте снова'));
  expect(screen.getByText(account.email)).toBeInTheDocument();
});
