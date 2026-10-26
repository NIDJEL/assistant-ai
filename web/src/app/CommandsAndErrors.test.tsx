import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { App } from './App';
import { mockServer } from '../test/server';

async function start(path = '/') {
  history.pushState({}, '', path); const server = mockServer(); render(<App/>);
  await screen.findByRole('heading', { name: path === '/integrations' ? 'Интеграции' : 'Чем помочь?' }, { timeout: 5000 });
  return { ...server, user: userEvent.setup() };
}
it('toggles quick commands, inserts editable templates, preserves drafts and never submits automatically', async () => {
  const { user, chats } = await start(); const toggle = screen.getByRole('button', { name: 'Быстрые команды' });
  const input = screen.getByRole('textbox', { name: 'Сообщение ассистенту' });
  for (const [label, value] of [['Напомнить', 'Напомни мне [о чём] [когда]'], ['Добавить в календарь', 'Добавь в календарь [событие] [дата и время]'], ['Создать задачу', 'Создай задачу [что нужно сделать]']]) {
    await user.click(toggle); await user.click(screen.getByRole('button', { name: label }));
    expect(input).toHaveValue(value); expect(input).toHaveFocus(); expect(toggle).toHaveAttribute('aria-expanded', 'false'); expect(chats).toHaveLength(0);
  }
  await user.clear(input); await user.type(input, 'купить молоко');
  await user.click(toggle); await user.click(screen.getByRole('button', { name: 'Напомнить' }));
  expect(input).toHaveValue('Напомни мне купить молоко');
  await user.click(toggle); await user.click(toggle); expect(toggle).toHaveAttribute('aria-expanded', 'false');
  await user.click(toggle); await user.click(screen.getByRole('heading', { name: 'Чем помочь?' }));
  expect(toggle).toHaveAttribute('aria-expanded', 'false'); expect(input).toHaveValue('Напомни мне купить молоко');
  await user.click(toggle); await user.keyboard('{Escape}'); expect(toggle).toHaveFocus(); expect(toggle).toHaveAttribute('aria-expanded', 'false');
});
it('opens integration settings immediately on compact screens and reopens them after closing', async () => {
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() })));
  const { user } = await start('/integrations');
  for (let attempt = 0; attempt < 2; attempt++) {
    await user.click(screen.getAllByRole('button', { name: 'Настроить' })[0]);
    const detail = await screen.findByRole('dialog', { name: 'Интеграция' });
    expect(detail).toHaveTextContent('Яндекс Календарь');
    await user.click(within(detail).getByRole('button', { name: 'Закрыть' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  }
});
it('shows a recoverable HTTP error when authentication receives a proxy failure', async () => {
  history.pushState({}, '', '/');
  let unavailable = true; const server = mockServer(); const handler = server.handler;
  vi.stubGlobal('fetch', vi.fn((path: string | URL | Request, init?: RequestInit) => unavailable && String(path) === '/api/auth/me'
    ? Promise.resolve(new Response('<html>Bad gateway</html>', { status: 502 })) : handler(path, init)));
  render(<App/>);
  expect(await screen.findByRole('heading', { name: 'Гена пока не отвечает' })).toBeInTheDocument();
  unavailable = false; await userEvent.click(screen.getByRole('button', { name: 'Попробовать снова' }));
  expect(await screen.findByRole('heading', { name: 'Чем помочь?' })).toBeInTheDocument();
});
