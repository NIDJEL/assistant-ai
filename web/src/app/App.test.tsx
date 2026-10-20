import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { App } from './App';
import { fixture, mockServer } from '../test/server';

async function start(path = '/', options: Parameters<typeof mockServer>[0] = {}) {
  history.pushState({}, '', path); const server = mockServer(options); render(<App/>);
  await screen.findByRole('heading', { level: 1 }); return { ...server, user: userEvent.setup() };
}
it('expands reminder groups independently and retains expansion after an item update', async () => {
  history.pushState({}, '', '/reminders'); const server = mockServer();
  for (let i = 0; i < 9; i++) server.events().push(fixture(100 + i, 'reminder', `Напомнить ${i}`, { date: '2026-10-15', time: i < 3 ? '09:00' : i < 6 ? '15:00' : '20:00' }));
  render(<App/>); await screen.findByRole('heading', { level: 1 }); const user = userEvent.setup();
  const morning = screen.getByRole('region', { name: 'Утро' }); const day = screen.getByRole('region', { name: 'День' }); const evening = screen.getByRole('region', { name: 'Вечер' });
  const count = (region: HTMLElement) => region.querySelectorAll('.reminder-card').length;
  expect([count(morning), count(day), count(evening)]).toEqual([2, 2, 2]);
  await user.click(within(morning).getByRole('button', { name: 'Показать ещё: Утро' }));
  await waitFor(() => expect(count(morning)).toBe(4)); expect([count(day), count(evening)]).toEqual([2, 2]);
  await user.click(within(morning).getAllByRole('switch')[0]); expect([count(morning), count(day), count(evening)]).toEqual([4, 2, 2]);
  await user.click(within(morning).getByRole('button', { name: 'Свернуть: Утро' })); await waitFor(() => expect(count(morning)).toBe(2));
});
it('offers settings only for widgets with an adjustable record count', async () => {
  await start('/calendar'); expect(screen.queryByRole('button', { name: 'Настройки: Календарь' })).not.toBeInTheDocument(); expect(screen.getByRole('button', { name: 'Настройки: Сегодня' })).toBeInTheDocument();
});
function deferred() { let resolve!: () => void; const promise = new Promise<void>(done => { resolve = done; }); return { promise, resolve }; }
it('shows a recurring task rule from structured API data', async () => {
  history.pushState({}, '', '/tasks'); const server = mockServer();
  server.events()[0].recurrence = { frequency: 'day', interval: 1, local_time: '09:00', timezone: 'Europe/Moscow' };
  render(<App/>); await screen.findByRole('heading', { level: 1, name: 'Задачи' });
  expect(document.querySelector('.task-card .task-recurrence')).toHaveTextContent('Каждый день, 09:00');
});
it('navigates all eight pages and preserves explicit integration states', async () => {
  const { user } = await start();
  const nav = screen.getByRole('navigation', { name: 'Основная навигация' });
  for (const name of ['Календарь', 'Задачи', 'Встречи', 'Напоминания', 'Архив', 'Интеграции']) {
    await user.click(within(nav).getByRole('link', { name })); expect(screen.getByRole('heading', { level: 1, name })).toBeInTheDocument();
  }
  expect(screen.queryByText('Скоро')).not.toBeInTheDocument();
  expect(document.querySelectorAll('.integration-state')).toHaveLength(3);
  await user.click(screen.getAllByRole('button', { name: 'Настроить' })[0]); expect(screen.getByRole('complementary', { name: 'Интеграция' })).toHaveTextContent('Разрешите Гене добавлять ваши дела в календарь.'); expect(screen.queryByRole('complementary', { name: 'Виджеты' })).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Назад к виджетам' })); await user.click(screen.getByRole('link', { name: 'Настройки' })); expect(screen.getByRole('heading', { level: 1, name: 'Настройки' })).toBeInTheDocument();
});
it.each(['Send', 'Enter'])('%s sends exactly once while a request is in progress', async mode => {
  const gate = deferred(); const server = await start('/', { chatGate: gate.promise });
  const input = screen.getByRole('textbox', { name: 'Сообщение ассистенту' });
  await server.user.type(input, 'Поставь задачу проверить отчёт');
  if (mode === 'Send') { const button = screen.getByRole('button', { name: 'Отправить сообщение' }); fireEvent.click(button); fireEvent.click(button); }
  else { fireEvent.keyDown(input, { key: 'Enter' }); fireEvent.keyDown(input, { key: 'Enter' }); }
  await waitFor(() => expect(server.chats).toHaveLength(1)); expect(server.chats[0].message).toBe('Поставь задачу проверить отчёт');
  await act(async () => gate.resolve()); await screen.findByText('Готово. Задача сохранена.'); expect(input).toHaveValue('');
});
it('Shift+Enter keeps a line break and does not submit', async () => {
  const { chats, user } = await start(); const input = screen.getByRole('textbox', { name: 'Сообщение ассистенту' });
  await user.type(input, 'Первая строка{Shift>}{Enter}{/Shift}Вторая строка'); expect(input).toHaveValue('Первая строка\nВторая строка'); expect(chats).toHaveLength(0);
});
it('network retry reuses request_id and keeps draft until acknowledgement', async () => {
  const { user, chats } = await start('/', { failFirstChat: true }); const input = screen.getByRole('textbox', { name: 'Сообщение ассистенту' });
  await user.type(input, 'Проверь отчёт'); await user.click(screen.getByRole('button', { name: 'Отправить сообщение' })); await screen.findByRole('alert'); expect(input).toHaveValue('Проверь отчёт');
  await user.click(screen.getByRole('button', { name: 'Отправить сообщение' })); await screen.findByText('Готово. Задача сохранена.');
  expect(chats).toHaveLength(2); expect(chats[0].request_id).toBe(chats[1].request_id);
});
it('late history response does not overwrite a new conversation', async () => {
  const gate = deferred(); const { user } = await start('/', { historyGate: gate.promise });
  await user.type(screen.getByRole('textbox', { name: 'Сообщение ассистенту' }), 'Задача'); await user.click(screen.getByRole('button', { name: 'Отправить сообщение' })); await screen.findByText('Готово. Задача сохранена.');
  await act(async () => gate.resolve()); expect(screen.getByText('Готово. Задача сохранена.')).toBeInTheDocument();
});
it('calendar renders server events and day/week/month navigation', async () => {
  const { user } = await start('/calendar'); expect(within(screen.getByRole('main')).getByText('октябрь 2026')).toBeInTheDocument();
  const views = screen.getByRole('group', { name: 'Вид календаря' }); await user.click(within(views).getByRole('button', { name: 'Неделя' })); expect(screen.getByRole('region', { name: 'Календарь на неделю' })).toBeInTheDocument();
  await user.click(within(views).getByRole('button', { name: 'День' })); expect(within(screen.getByRole('region', { name: 'События выбранного дня' })).getByRole('button', { name: 'Обсуждение проекта' })).toBeInTheDocument();
});
it('task completion calls PATCH and moves the event to completed', async () => {
  const { user, writes } = await start('/tasks'); await user.click(screen.getByRole('checkbox', { name: 'Завершить «Подготовить отчёт»' }));
  await screen.findByRole('checkbox', { name: 'Возобновить «Подготовить отчёт»' }); expect(writes[0]).toMatchObject({ method: 'PATCH', input: { status: 'done' } });
});
it('meeting conflict is displayed and force requires a second explicit action', async () => {
  const { user, writes } = await start('/meetings'); await user.click(screen.getByRole('button', { name: 'Запланировать встречу' }));
  const dialog = screen.getByRole('dialog'); await user.type(within(dialog).getByLabelText('Название'), 'Конфликтная встреча'); await user.click(within(dialog).getByRole('button', { name: 'Сохранить' }));
  await within(dialog).findByText('Пересечение со встречами'); expect(writes).toHaveLength(1); expect(writes[0].input.force).toBe(false);
  await user.click(within(dialog).getByRole('button', { name: 'Сохранить с пересечением' })); await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument()); expect(writes[1].input.force).toBe(true);
});
it('reminder edit persists title and time', async () => {
  const { user, writes } = await start('/reminders'); await user.click(within(screen.getByRole('main')).getByRole('button', { name: 'Позвонить Ивану' }));
  const dialog = screen.getByRole('form', { name: 'Редактор: Позвонить Ивану' }); const title = within(dialog).getByLabelText('Название напоминания'); await user.clear(title); await user.type(title, 'Позвонить Марии'); fireEvent.change(within(dialog).getByLabelText('Время напоминания', { exact: true }), { target: { value: '09:45' } }); await user.click(within(dialog).getByRole('button', { name: 'Сохранить' }));
  await waitFor(() => expect(screen.queryByRole('form', { name: 'Редактор: Позвонить Ивану' })).not.toBeInTheDocument()); expect(writes[0]).toMatchObject({ method: 'PATCH', input: { title: 'Позвонить Марии', time: '09:45' } });
});
it('keeps a reminder menu mounted during a delayed search response', async () => {
  const gate = deferred(); const { user, handler } = await start('/reminders', { searchGate: gate.promise });
  await user.type(screen.getByRole('textbox', { name: 'Поиск напоминаний' }), 'Позвонить Ивану');
  await waitFor(() => expect(handler.mock.calls.some(([path]) => String(path).includes('q='))).toBe(true));
  await user.click(screen.getByRole('button', { name: 'Действия: Позвонить Ивану' }));
  expect(document.querySelector('.row-menu')).toHaveTextContent('Удалить');
  await act(async () => gate.resolve());
  await waitFor(() => expect(document.querySelector('.row-menu')).toHaveTextContent('Удалить'));
});
it('replays newer cached preferences after an immediate refresh instead of restoring stale server values', async () => {
  localStorage.setItem('rhythm.preferences.v1.11111111-1111-4111-8111-111111111111', JSON.stringify({ name: 'Анна', widgets: false, tts: false, layouts: {}, revision: 2 }));
  const { handler } = await start('/settings', { preferences: { name: 'Алексей', widgets: true, revision: 1 } });
  expect(screen.getByLabelText('Ваше имя')).toHaveValue('Алексей'); // Identity name comes from the authenticated user.
  expect(screen.getByRole('button', { name: 'Показать виджеты' })).toBeInTheDocument();
  await waitFor(() => expect(handler.mock.calls.some(([, init]) => init?.method === 'PUT' && JSON.parse(String(init.body)).revision === 2 && JSON.parse(String(init.body)).name === 'Анна')).toBe(true));
});
it('archive search calls the typed client and filters real results', async () => {
  const { user, handler } = await start('/archive'); await user.type(screen.getByRole('textbox', { name: 'Поиск в архиве' }), 'договор');
  await waitFor(() => expect(handler.mock.calls.some(([path]) => String(path).includes('q=%D0%B4%D0%BE%D0%B3%D0%BE%D0%B2%D0%BE%D1%80'))).toBe(true)); expect(screen.getByRole('button', { name: 'Архивный договор' })).toBeInTheDocument();
});
it('ignores another legacy browser user’s dialogue and TTS preference', async () => {
 const legacy = '22222222-2222-4222-8222-222222222222'; localStorage.setItem('rhythm-session', legacy); localStorage.setItem('rhythm-speak', 'true');
 const { handler, user } = await start();
 await waitFor(() => expect(handler.mock.calls.some(([path]) => String(path).startsWith('/api/history/'))).toBe(true));
 expect(handler.mock.calls.some(([path]) => String(path).endsWith(`/api/history/${legacy}`))).toBe(false);
 await user.click(screen.getByRole('link', { name: 'Настройки' })); expect(await screen.findByText('Дмитрий, мужской')).toBeInTheDocument();
 expect(screen.getByText(/Автоматическая озвучка выключена/)).toBeInTheDocument();
});
it('replaces a corrupt stored session UUID instead of repeatedly failing history', async () => {
  localStorage.setItem('rhythm.session.v2.11111111-1111-4111-8111-111111111111', '-'.repeat(36)); await start(); expect(localStorage.getItem('rhythm.session.v2.11111111-1111-4111-8111-111111111111')).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i);
});
it('assistant event cards reflect completion from the server instead of stale history snapshots', async () => {
  const { user } = await start(); await user.type(screen.getByRole('textbox', { name: 'Сообщение ассистенту' }), 'Создай задачу'); await user.click(screen.getByRole('button', { name: 'Отправить сообщение' })); await screen.findByText('Готово. Задача сохранена.');
  await user.click(screen.getByRole('checkbox', { name: 'Завершить «Подготовить отчёт»' })); await screen.findByRole('checkbox', { name: 'Возобновить «Подготовить отчёт»' });
});
