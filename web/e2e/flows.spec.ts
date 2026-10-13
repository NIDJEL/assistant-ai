import { test, expect } from '@playwright/test';
import { ready, noOverflow, screenshot } from './helpers';
import { dayKey, addDays } from '../src/features/calendar/dates';
import type { Health, RhythmEvent } from '../src/types/api';

test('UI CRUD: task, meeting conflict, move, reminder, archive and delete', async ({ page, request }, info) => {
  const health = await (await request.get('/health')).json() as Health; const today = dayKey(health.now, health.timezone); const token = String(Date.now()); const created: number[] = [];
  async function save() { const response = page.waitForResponse(r => r.url().endsWith('/api/events') && r.request().method() === 'POST'); await page.getByRole('dialog').getByRole('button', { name: 'Сохранить', exact: true }).click(); const result = await response; const event = await result.json() as RhythmEvent; expect(result.status()).toBe(201); created.push(event.id); await expect(page.getByRole('dialog')).toHaveCount(0); return event; }
  try {
    await ready(page, '/tasks'); await page.getByRole('button', { name: 'Новая задача', exact: true }).click();
    await page.getByLabel('Название', { exact: true }).fill(`Проверить интерфейс ${token}`); await page.getByLabel('Приоритет', { exact: true }).selectOption('high'); const task = await save();
    await page.getByRole('textbox', { name: 'Поиск задач' }).fill(task.title); await page.getByRole('checkbox', { name: `Завершить «${task.title}»`, exact: true }).check();
    await expect(page.getByRole('checkbox', { name: `Возобновить «${task.title}»`, exact: true })).toBeChecked();
    await page.getByRole('link', { name: 'Архив', exact: true }).click(); await page.getByRole('textbox', { name: 'Поиск в архиве' }).fill(task.title); await expect(page.getByRole('button', { name: task.title, exact: true })).toBeVisible();
    expect((await (await request.get(`/api/events/${task.id}`)).json()).status).toBe('done');
    await page.getByRole('link', { name: 'Календарь', exact: true }).click(); await page.getByRole('button', { name: 'Новое событие' }).click();
    await page.getByLabel('Название', { exact: true }).fill(`Обсуждение интерфейса ${token}`); await page.getByLabel('Дата', { exact: true }).fill(addDays(today, 1)); await page.getByLabel('Время', { exact: true }).fill('03:12'); await page.getByLabel('Длительность, мин').fill('15'); await page.getByLabel('Место', { exact: true }).fill('Переговорная'); await page.getByLabel('Ссылка встречи').fill('https://example.org/meeting'); const meeting = await save();
    await ready(page, `/calendar?date=${meeting.date}`); await expect(page.getByRole('button', { name: meeting.title, exact: true })).toBeVisible();
    await page.getByRole('link', { name: 'Встречи', exact: true }).click(); await page.getByRole('textbox', { name: 'Поиск встреч' }).fill(meeting.title);
    await expect(page.locator('.meeting-details h2')).toHaveText(meeting.title); await expect(page.getByRole('link', { name: 'Открыть ссылку' })).toHaveAttribute('href', 'https://example.org/meeting');
    await page.getByRole('button', { name: 'Запланировать встречу' }).click(); await page.getByLabel('Название', { exact: true }).fill(`Конфликт интерфейса ${token}`); await page.getByLabel('Дата', { exact: true }).fill(meeting.date!); await page.getByLabel('Время', { exact: true }).fill(meeting.time!); await page.getByLabel('Длительность, мин').fill('15');
    const conflict = page.waitForResponse(r => r.url().endsWith('/api/events') && r.request().method() === 'POST'); await page.getByRole('dialog').getByRole('button', { name: 'Сохранить', exact: true }).click(); expect((await conflict).status()).toBe(409);
    await expect(page.getByRole('heading', { name: 'Пересечение со встречами' })).toBeVisible(); await screenshot(page, 'meeting-conflict', info);
    const forced = page.waitForResponse(r => r.url().endsWith('/api/events') && r.request().method() === 'POST'); await page.getByRole('button', { name: 'Сохранить с пересечением' }).click(); const forcedResponse = await forced; expect(forcedResponse.status()).toBe(201); const forcedEvent = await forcedResponse.json() as RhythmEvent; created.push(forcedEvent.id); await expect(page.getByRole('dialog')).toHaveCount(0);
    await page.getByRole('button', { name: 'Перенести', exact: true }).click(); await page.getByLabel('Время', { exact: true }).fill('04:12'); await page.getByRole('dialog').getByRole('button', { name: 'Сохранить', exact: true }).click(); await expect(page.getByRole('dialog')).toHaveCount(0); expect((await (await request.get(`/api/events/${meeting.id}`)).json()).time).toBe('04:12');
    await page.getByRole('link', { name: 'Напоминания', exact: true }).click(); await page.getByRole('button', { name: 'Новое напоминание' }).click(); await page.getByLabel('Название', { exact: true }).fill(`Проверить напоминание ${token}`); const reminder = await save();
    await page.getByRole('textbox', { name: 'Поиск напоминаний' }).fill(reminder.title); await page.locator('.reminder-title').filter({ hasText: reminder.title }).click(); await page.getByLabel('Время напоминания', { exact: true }).fill('09:45'); await page.getByRole('form', { name: `Редактор: ${reminder.title}` }).getByRole('button', { name: 'Сохранить', exact: true }).click(); await expect(page.locator('.reminder-inline')).toHaveCount(0); expect((await (await request.get(`/api/events/${reminder.id}`)).json()).time).toBe('09:45');
    await page.getByRole('button', { name: `Действия: ${reminder.title}` }).click(); await page.locator('.row-menu').getByRole('button', { name: 'Удалить', exact: true }).click(); await page.getByRole('dialog').getByRole('button', { name: 'Удалить', exact: true }).click(); await expect(page.getByRole('dialog')).toHaveCount(0); expect((await request.get(`/api/events/${reminder.id}`)).status()).toBe(404);
  } finally { for (const id of created) await request.delete(`/api/events/${id}`); }
});

test('native dialog focus trap, ESC and focus restoration', async ({ page }) => {
  await ready(page, '/tasks'); const button = page.getByRole('button', { name: 'Новая задача', exact: true }); await button.click();
  await expect(page.getByLabel('Название', { exact: true })).toBeFocused();
  for (let i = 0; i < 15; i++) { await page.keyboard.press('Tab'); expect(await page.evaluate(() => !!document.activeElement?.closest('dialog'))).toBe(true); }
  await page.keyboard.press('Escape'); await expect(page.getByRole('dialog')).toHaveCount(0); await expect(button).toBeFocused();
});
test('calendar handles Go civil-date boundaries and invalid route queries', async ({ page }) => {
  await ready(page, '/calendar?date=9999-12-31'); await expect(page.getByRole('button', { name: 'Следующий период', exact: true })).toBeDisabled(); await noOverflow(page);
  await ready(page, '/calendar?date=0001-01-01'); await expect(page.getByRole('button', { name: 'Предыдущий период', exact: true })).toBeDisabled();
  await ready(page, '/calendar?date=0000-01-01'); await expect(page.locator('h1')).toHaveText('Календарь'); await expect(page.getByRole('button', { name: 'Новое событие' })).toBeVisible();
});

test('widget visibility and Settings persist in this browser without an account', async ({ page }) => {
  await ready(page); await page.getByRole('button', { name: 'Скрыть виджеты' }).click(); await expect(page.getByRole('complementary', { name: 'Виджеты', exact: true })).toHaveCount(0);
  await page.reload(); await expect(page.getByRole('button', { name: 'Показать виджеты' })).toBeVisible(); await page.getByRole('button', { name: 'Показать виджеты' }).click(); await expect(page.getByRole('complementary', { name: 'Виджеты', exact: true })).toBeVisible();
  await page.getByRole('link', { name: 'Настройки', exact: true }).click(); await page.getByLabel('Ваше имя').fill('Анна'); await page.getByRole('button', { name: 'Сохранить имя' }).click(); await page.reload(); await expect(page.getByLabel('Ваше имя')).toHaveValue('Анна');
});

test('responsive routes, drawers and desktop screenshots for all pages', async ({ page }, info) => {
  const pages = [{ path: '/', title: 'Чем помочь?', shot: 'today' }, { path: '/calendar', title: 'Календарь', shot: 'calendar' }, { path: '/tasks', title: 'Задачи', shot: 'tasks' }, { path: '/meetings', title: 'Встречи', shot: 'meetings' }, { path: '/reminders', title: 'Напоминания', shot: 'reminders' }, { path: '/archive', title: 'Архив', shot: 'archive' }, { path: '/integrations', title: 'Интеграции', shot: 'integrations' }, { path: '/settings', title: 'Настройки', shot: 'settings' }];
  for (const viewport of [{ width: 1920, height: 1080 }, { width: 1600, height: 900 }, { width: 1440, height: 900 }, { width: 1280, height: 800 }, { width: 834, height: 1112 }, { width: 390, height: 844 }]) {
    await page.setViewportSize(viewport);
    for (const item of pages) {
      await ready(page, item.path); await expect(page.locator('h1')).toHaveText(item.title); await page.reload(); await expect(page.locator('h1')).toHaveText(item.title); await noOverflow(page);
      if (viewport.width === 1440) {
        if (item.path === '/') { await page.getByRole('button', { name: 'Что у меня на неделю?' }).click(); await page.getByRole('button', { name: 'Отправить сообщение' }).click(); await expect(page.locator('.assistant-message .reply-text')).toBeVisible(); }
        await screenshot(page, item.shot, info);
      }
    }
    if (viewport.width < 1180) { await page.getByRole('button', { name: 'Открыть виджеты' }).click(); await expect(page.getByRole('dialog', { name: 'Виджеты' })).toBeVisible(); await noOverflow(page); await page.keyboard.press('Escape'); }
    if (viewport.width === 390) { await page.getByRole('button', { name: 'Открыть навигацию' }).click(); await page.getByRole('dialog').getByRole('link', { name: 'Сегодня', exact: true }).click(); await expect(page.locator('h1')).toHaveText('Чем помочь?'); await noOverflow(page); await screenshot(page, 'mobile', info); }
  }
});
