import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { App } from './App';
function response(body: unknown, status = 200) { return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }); }
function server(handler?: (path: string, init: RequestInit) => Promise<Response> | Response) {
 const fetch = vi.fn(async (path: string, init: RequestInit = {}) => { if (handler) { const value = await handler(path, init); if (value) return value; } return response({ error: { code: 'unauthorized', message: 'Войдите, чтобы продолжить работу' } }, 401); }); vi.stubGlobal('fetch', fetch); return fetch;
}
it('keeps the dashboard unmounted while checking the session, then redirects to auth', async () => {
 history.pushState({}, '', '/tasks'); let done!: (value: Response) => void; const pending = new Promise<Response>(resolve => { done = resolve; }); server(() => pending); render(<App/>);
 expect(document.querySelector('.app-shell')).not.toBeInTheDocument(); expect(document.querySelector('.auth-splash')).toBeInTheDocument();
 await act(async () => done(response({ error: { message: 'Войдите' } }, 401))); await screen.findByRole('heading', { name: 'Добро пожаловать! Я Гена' }); expect(location.pathname).toBe('/auth');
 expect(screen.getAllByText('Гена')).toHaveLength(2); expect(screen.getByRole('button', { name: 'Войти через Яндекс' })).toBeInTheDocument(); expect(screen.queryByRole('button', { name: /VK/ })).not.toBeInTheDocument();
});
it('switches forms in one auth card, validates confirmation and toggles password visibility', async () => {
 history.pushState({}, '', '/auth'); server(); const user = userEvent.setup(); render(<App/>); await screen.findByRole('heading', { name: 'Добро пожаловать! Я Гена' });
 await user.click(screen.getByRole('tab', { name: 'Создать аккаунт' })); await screen.findByLabelText('Повторите пароль'); expect(screen.getByRole('button', { name: 'Продолжить с Яндексом' })).toBeInTheDocument();
 await user.type(screen.getByLabelText('Имя'), 'Анна'); await user.type(screen.getByLabelText('Почта'), 'anna@example.test'); await user.type(screen.getByLabelText('Пароль', { exact: true }), 'password 12345'); await user.type(screen.getByLabelText('Повторите пароль'), 'password wrong'); await user.click(screen.getByRole('button', { name: 'Создать аккаунт' }));
 expect(screen.getByRole('alert')).toHaveTextContent('Пароли не совпадают'); await user.click(screen.getByRole('button', { name: 'Показать пароль' })); expect(screen.getByLabelText('Пароль', { exact: true })).toHaveAttribute('type', 'text');
 await user.click(screen.getByRole('tab', { name: 'Войти' })); await waitFor(() => expect(screen.queryByLabelText('Повторите пароль')).not.toBeInTheDocument()); expect(document.querySelectorAll('.auth-container')).toHaveLength(1);
});
it('disables duplicate login submissions and presents generic inline errors', async () => {
 history.pushState({}, '', '/auth'); let release!: () => void; const pending = new Promise<void>(resolve => { release = resolve; }); const fetch = server(async path => { if (path.endsWith('/login')) { await pending; return response({ error: { message: 'Неверная почта или пароль' } }, 401); } return response({}, 401); }); const user = userEvent.setup(); render(<App/>); await screen.findByRole('heading', { name: 'Добро пожаловать! Я Гена' });
 await user.type(screen.getByLabelText('Почта'), 'anna@example.test'); await user.type(screen.getByLabelText('Пароль', { exact: true }), 'password 12345'); await user.click(screen.getByRole('button', { name: 'Войти' })); expect(screen.getByRole('button', { name: 'Входим…' })).toBeDisabled(); expect(fetch.mock.calls.filter(([path]) => path.endsWith('/login'))).toHaveLength(1);
 await act(async () => release()); expect(await screen.findByRole('alert')).toHaveTextContent('Неверная почта или пароль');
});
it('first login offers calendar separately and persists completion', async () => {
 history.pushState({}, '', '/onboarding'); const user = { id: '11111111-1111-4111-8111-111111111111', name: 'Анна', email: 'anna@example.test', providers: ['yandex'], avatar_url: '', onboarding_completed: false };
 const fetch = server(path => response({ user: { ...user, onboarding_completed: path.endsWith('/onboarding') } })); const keyboard = userEvent.setup(); render(<App/>); await screen.findByRole('button', { name: 'Начать' }); await keyboard.click(screen.getByRole('button', { name: 'Начать' })); expect(screen.getByRole('heading', { name: 'Подключить Яндекс Календарь?' })).toBeInTheDocument();
 await keyboard.click(screen.getByRole('button', { name: 'Сделать позже' })); await keyboard.click(screen.getByRole('button', { name: 'Перейти к Гене' })); await waitFor(() => expect(fetch.mock.calls.some(([path]) => path === '/api/auth/onboarding')).toBe(true)); expect(fetch.mock.calls.some(([path]) => path.includes('/api/integrations'))).toBe(false);
});
