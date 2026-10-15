import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { OnboardingPage } from './OnboardingPage';
import { calendarOAuthURL, startCalendarConnection } from '../../api/integrations';
const { complete } = vi.hoisted(() => ({ complete: vi.fn(async () => {}) }));
vi.mock('../../app/AuthContext', () => ({ useAuth: () => ({ user: { id: 'alice', name: 'Анна', onboarding_completed: false }, completeOnboarding: complete }) }));
vi.mock('../../api/integrations', async original => ({ ...await original<object>(), startCalendarConnection: vi.fn() }));
function mount(path = '/onboarding', status?: string) {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ yandex_configured: true, connections: status ? [{ provider: 'yandex', status }] : [] }))));
  return render(<MemoryRouter initialEntries={[path]}><Routes><Route path="/onboarding" element={<OnboardingPage/>}/><Route path="/" element={<h1>Чат Гены</h1>}/><Route path="/integrations" element={<h1>Интеграции</h1>}/></Routes></MemoryRouter>);
}
beforeEach(() => { complete.mockClear(); vi.mocked(startCalendarConnection).mockClear(); });
it('connect starts real OAuth from onboarding without navigating to Integrations', async () => {
  mount(); const user = userEvent.setup(); await user.click(screen.getByRole('button', { name: 'Начать' })); await user.click(screen.getByRole('button', { name: 'Подключить Яндекс Календарь' }));
  expect(startCalendarConnection).toHaveBeenCalledWith('onboarding'); expect(complete).not.toHaveBeenCalled(); expect(screen.queryByRole('heading', { name: 'Интеграции' })).not.toBeInTheDocument();
  expect(calendarOAuthURL('onboarding')).toBe('/api/auth/yandex/start?calendar=1&source=onboarding');
});
it('skip completes onboarding and opens chat immediately', async () => { mount(); const user = userEvent.setup(); await user.click(screen.getByRole('button', { name: 'Начать' })); await user.click(screen.getByRole('button', { name: 'Сделать позже' })); expect(await screen.findByRole('heading', { name: 'Чат Гены' })).toBeVisible(); expect(complete).toHaveBeenCalledOnce(); });
it.each(['connected', 'pending'])('already %s calendar skips another connection', async status => { mount('/onboarding', status); await screen.findByRole('heading', { name: 'Чат Гены' }); expect(complete).toHaveBeenCalledOnce(); expect(startCalendarConnection).not.toHaveBeenCalled(); });
it('callback failure stays in onboarding with retry and skip', async () => { mount('/onboarding?error=oauth_cancelled'); expect(screen.getByRole('alert')).toHaveTextContent('Не удалось подключить Яндекс Календарь'); await userEvent.click(screen.getByRole('button', { name: 'Попробовать ещё раз' })); expect(startCalendarConnection).toHaveBeenCalledWith('onboarding'); });
it('can skip after callback failure', async () => { mount('/onboarding?error=oauth_invalid'); await userEvent.click(screen.getByRole('button', { name: 'Сделать позже' })); await screen.findByRole('heading', { name: 'Чат Гены' }); });
it('keeps the integration OAuth source distinct and internal', () => { expect(calendarOAuthURL('integrations')).toBe('/api/auth/yandex/start?calendar=1&source=integrations'); });
it('allows retry if completing onboarding fails', async () => { complete.mockRejectedValueOnce(new Error('Нет связи')); mount('/onboarding', 'connected'); await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('Нет связи')); });
