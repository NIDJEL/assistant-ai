import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { SocialProvider, useSocial } from './SocialContext';
vi.mock('../../app/DataContext', () => ({ useData: () => ({ refresh: async () => {} }) }));
function Actions() { const social = useSocial(); return <button onClick={() => void social.perform({ action: 'create_meeting' })}>Отправить</button>; }
it('keeps incomplete-calendar notice visible outside the meeting editor until dismissed', async () => {
  const message = 'Встреча создана. Доступность внешних календарей проверена не полностью.';
  vi.stubGlobal('fetch', vi.fn(async (path: string, init?: RequestInit) => new Response(JSON.stringify(init?.method === 'POST' ? { message } : path.endsWith('/people') ? { profile: {}, friends: [], incoming: [], outgoing: [] } : { meetings: [] }), { headers: { 'Content-Type': 'application/json' } })));
  render(<SocialProvider><Actions/></SocialProvider>); const user = userEvent.setup(); await user.click(screen.getByRole('button', { name: 'Отправить' })); expect(await screen.findByRole('status')).toHaveTextContent(message); await user.click(screen.getByRole('button', { name: 'Понятно' })); expect(screen.queryByRole('status')).not.toBeInTheDocument();
});
