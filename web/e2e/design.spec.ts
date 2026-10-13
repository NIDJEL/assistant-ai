import { test, expect } from '@playwright/test';
import { ready, noOverflow } from './helpers';
import { dayKey, addDays, formatDay } from '../src/features/calendar/dates';
import type { Health, RhythmEvent } from '../src/types/api';

test('widget handle reorder, add/remove, per-page settings and SQLite persistence', async ({ page, request }) => {
  await ready(page, '/calendar');
  const profile = await page.evaluate(() => localStorage.getItem('rhythm.profile.v1'));
  const saved = async () => (await (await request.get(`/api/preferences/${profile}`)).json()).layouts?.['/calendar'];
  const handle = page.getByRole('button', { name: 'Переместить: Календарь', exact: true });
  await handle.focus(); await page.keyboard.press('Space'); await page.keyboard.press('ArrowDown');
  await expect(page.locator('.widget-drag-overlay')).toBeVisible(); await page.keyboard.press('Space');
  await expect.poll(async () => (await saved())?.order).toEqual(['today', 'calendar', 'tasks']);
  await page.reload(); await expect(page.locator('.widget-list > [data-widget]').first()).toHaveAttribute('data-widget', 'today');
  const titleBox = await page.locator('[data-widget="today"] h3').boundingBox();
  const calendarBox = await page.locator('[data-widget="calendar"]').boundingBox();
  if (!titleBox || !calendarBox) throw new Error('Widget boxes missing');
  await page.mouse.move(titleBox.x + 5, titleBox.y + 5); await page.mouse.down(); await page.mouse.move(calendarBox.x + 40, calendarBox.y + 45, { steps: 8 }); await page.mouse.up();
  await expect(page.locator('.widget-drag-overlay')).toHaveCount(0); // The body/title cannot start a drag.
  const gripBox = await page.getByRole('button', { name: 'Переместить: Сегодня', exact: true }).boundingBox();
  if (!gripBox) throw new Error('Handle missing');
  await page.mouse.move(gripBox.x + 8, gripBox.y + 8); await page.mouse.down(); await page.mouse.move(calendarBox.x + 25, calendarBox.y + 40, { steps: 12 });
  await expect(page.locator('.widget-drag-overlay')).toBeVisible(); await page.mouse.up();
  await expect.poll(async () => (await saved())?.order).toEqual(['calendar', 'today', 'tasks']);
  await page.reload(); await expect(page.locator('.widget-list > [data-widget]').first()).toHaveAttribute('data-widget', 'calendar');
  await page.getByRole('button', { name: 'Добавить виджет', exact: true }).click();
  const picker = page.getByRole('dialog', { name: 'Ваши виджеты' });
  await picker.locator('.widget-picker > div').filter({ hasText: 'Задачи' }).getByRole('button', { name: 'Убрать' }).click();
  await picker.locator('.widget-picker > div').filter({ hasText: 'Мой прогресс' }).getByRole('button', { name: 'Добавить' }).click();
  await expect(picker.locator('.widget-picker > div').filter({ hasText: 'Мой прогресс' }).getByRole('button', { name: 'Добавить' })).toHaveCount(0);
  await page.keyboard.press('Escape');
  await expect.poll(async () => (await saved())?.hidden).toContain('tasks');
  await expect.poll(async () => (await saved())?.order).toContain('progress');
  await page.reload(); await expect(page.locator('[data-widget="tasks"]')).toHaveCount(0); await expect(page.locator('[data-widget="progress"]')).toHaveCount(1);
  await page.getByRole('button', { name: 'Настройки: Сегодня', exact: true }).click();
  await page.getByRole('dialog').getByRole('combobox').selectOption('5'); await page.getByRole('button', { name: 'Готово', exact: true }).click();
  await expect.poll(async () => (await saved())?.settings?.today?.limit).toBe(5);
  await page.reload(); await page.getByRole('button', { name: 'Настройки: Сегодня', exact: true }).click(); await expect(page.getByRole('dialog').getByRole('combobox')).toHaveValue('5'); await page.keyboard.press('Escape');
  await ready(page, '/tasks'); await expect(page.locator('[data-widget="tags"]')).toHaveCount(1); // Other page keeps its own layout.
});

test('expanded task, meeting selection, selected calendar day, inline reminder and integration detail use real API', async ({ page, request }) => {
  const health = await (await request.get('/health')).json() as Health; const today = dayKey(health.now, health.timezone); const next = addDays(today, 1); const token = Date.now(); const ids: number[] = [];
  const create = async (body: object) => { const response = await request.post('/api/events', { data: body }); expect(response.status()).toBe(201); const value = await response.json() as RhythmEvent; ids.push(value.id); return value; };
  try {
    const task = await create({ kind: 'task', title: `Контроль качества ${token}`, date: today, description: 'Проверить итог перед публикацией.', priority: 'high', tags: ['Контроль'], subtasks: [{ id: 'a', title: 'Проверить цифры', done: false }, { id: 'b', title: 'Записать вывод', done: true }] });
    await ready(page, '/tasks'); await page.getByRole('textbox', { name: 'Поиск задач' }).fill(task.title);
    await expect(page.locator('.task-expanded')).toContainText('Проверить итог'); await page.getByRole('checkbox', { name: 'Проверить цифры', exact: true }).check();
    await expect.poll(async () => (await (await request.get(`/api/events/${task.id}`)).json()).subtasks[0].done).toBe(true);
    await page.reload(); await page.getByRole('textbox', { name: 'Поиск задач' }).fill(task.title); await expect(page.getByRole('checkbox', { name: 'Проверить цифры', exact: true })).toBeChecked();
    const one = await create({ kind: 'meeting', title: `Первое обсуждение ${token}`, date: today, time: '05:30', duration_minutes: 30, force: true, participants: [{ name: 'Тестовый участник' }] });
    const two = await create({ kind: 'meeting', title: `Второе обсуждение ${token}`, date: next, time: '05:30', duration_minutes: 45, force: true, location: 'Переговорная', description: 'Обсудить результаты проверки.', participants: [{ name: 'Второй участник' }] });
    await ready(page, '/meetings'); await page.getByRole('textbox', { name: 'Поиск встреч' }).fill(`обсуждение ${token}`); await page.locator('.meeting-tile').filter({ hasText: two.title }).click();
    await expect(page.locator('.meeting-details h2')).toHaveText(two.title); await expect(page.locator('.meeting-details')).toContainText('Второй участник'); await expect(page.locator('.meeting-details')).toContainText('45 мин');
    await page.locator('.meeting-tile').filter({ hasText: one.title }).click(); await expect(page.locator('.meeting-details')).toContainText('Тестовый участник');
    await ready(page, '/calendar'); await page.locator(`.calendar-cell[aria-label^="${formatDay(next)},"]`).click();
    await expect(page.getByRole('region', { name: 'События выбранного дня' })).toContainText(two.title);
    const reminder = await create({ kind: 'reminder', title: `Напомнить о проверке ${token}`, date: today, time: '09:00' });
    await ready(page, '/reminders'); await page.getByRole('textbox', { name: 'Поиск напоминаний' }).fill(reminder.title); await page.locator('.reminder-title').filter({ hasText: reminder.title }).click();
    const inline = page.getByRole('form', { name: `Редактор: ${reminder.title}` }); await inline.getByLabel('Время напоминания').fill('09:45'); await inline.getByRole('button', { name: 'Сохранить', exact: true }).click();
    await expect.poll(async () => (await (await request.get(`/api/events/${reminder.id}`)).json()).time).toBe('09:45');
    await page.getByRole('switch', { name: `Включить «${reminder.title}»` }).uncheck(); await expect.poll(async () => (await (await request.get(`/api/events/${reminder.id}`)).json()).reminder_enabled).toBe(false);
    await ready(page, '/integrations'); await page.getByRole('button', { name: 'Настроить', exact: true }).click();
    await expect(page.getByRole('complementary', { name: 'Виджеты', exact: true })).toHaveCount(0); const detail = page.getByRole('complementary', { name: 'Интеграция', exact: true }); await expect(detail).toContainText('Не подключено'); await expect(detail.getByRole('button', { name: 'Синхронизировать сейчас' })).toBeDisabled();
    await page.reload(); await expect(detail).toBeVisible(); await noOverflow(page); await page.getByRole('button', { name: 'Назад к виджетам' }).click(); await expect(page.getByRole('complementary', { name: 'Виджеты', exact: true })).toBeVisible();
  } finally { for (const id of ids) await request.delete(`/api/events/${id}`); }
});
