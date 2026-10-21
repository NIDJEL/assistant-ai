import { chooseOption } from './helpers';
import { test, expect } from './fixtures';
import { ready, noOverflow, screenshot } from './helpers';
import { dayKey, addDays } from '../src/features/calendar/dates';
import type { Health, RhythmEvent } from '../src/types/api';

async function settle(page: import('@playwright/test').Page) {
  await page.evaluate(async () => { await Promise.allSettled(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished)); });
}

test('side panels stay at screen edges, hidden widgets do not stretch content and navigation collapse persists', async ({ page }, info) => {
  await page.setViewportSize({ width: 2560, height: 1080 }); await ready(page, '/calendar'); await settle(page);
  const edges = await page.evaluate(() => ({ viewport: document.documentElement.getBoundingClientRect().width, shell: document.querySelector('.app-shell')!.getBoundingClientRect().width, navigation: document.querySelector('.sidebar')!.getBoundingClientRect().left, widgets: document.querySelector('.widgets-panel')!.getBoundingClientRect().right }));
  expect(Math.abs(edges.shell - edges.viewport)).toBeLessThan(1); expect(edges.navigation).toBe(16); expect(edges.widgets).toBe(edges.viewport - 16);
  await screenshot(page, 'wide-calendar', info);
  await page.getByRole('button', { name: 'Свернуть навигацию', exact: true }).click(); await settle(page);
  expect(await page.locator('.sidebar').evaluate(node => node.getBoundingClientRect().width)).toBe(76);
  await expect(page.locator('.sidebar .nav-link span').first()).toBeHidden(); await noOverflow(page);
  await page.reload(); await expect(page.getByRole('button', { name: 'Развернуть навигацию' })).toBeVisible();
  await expect(page.locator('h1')).toHaveText('Календарь'); await expect(page.locator('.widgets-heading')).toBeVisible();
  await screenshot(page, 'collapsed-navigation', info);
  await page.getByRole('button', { name: 'Скрыть виджеты' }).click(); await settle(page);
  expect(await page.locator('.main-panel > .page').evaluate(node => node.getBoundingClientRect().width)).toBeLessThanOrEqual(1280);
  expect(await page.locator('.main-panel > .page').evaluate(node => { const page = node.getBoundingClientRect(); const main = node.parentElement!.getBoundingClientRect(); return Math.abs(page.left + page.width / 2 - main.left - main.width / 2); })).toBeLessThan(1);
  expect(await page.locator('.sidebar').evaluate(node => node.getBoundingClientRect().left)).toBe(16);
  await noOverflow(page); await page.getByRole('button', { name: 'Развернуть навигацию' }).click(); await settle(page);
  await expect(page.locator('.sidebar .nav-link span').first()).toBeVisible(); await noOverflow(page);
  await screenshot(page, 'hidden-widgets', info);
});

test('mobile navigation opens on the left and widget drawer stays on the right', async ({ page }, info) => {
  await page.setViewportSize({ width: 593, height: 932 }); await ready(page, '/calendar');
  await page.getByRole('button', { name: 'Открыть навигацию' }).click(); await settle(page);
  const navigation = page.getByRole('dialog', { name: 'Навигация', exact: true });
  expect(await navigation.evaluate(node => node.getBoundingClientRect().left)).toBeLessThanOrEqual(1);
  await noOverflow(page); await screenshot(page, 'left-navigation', info);
  await page.keyboard.press('Escape'); await expect(navigation).toHaveCount(0);
  await page.getByRole('button', { name: 'Открыть виджеты', exact: true }).click(); await settle(page);
  const widgets = page.getByRole('dialog', { name: 'Виджеты', exact: true });
  expect(await widgets.evaluate(node => node.getBoundingClientRect().right)).toBeGreaterThan(590);
  await noOverflow(page);
});

test('calendar circles remain round and content changes leave the header and toolbar in place', async ({ page }) => {
  await ready(page, '/calendar');
  await page.evaluate(() => {
    const original = document.startViewTransition.bind(document); Object.assign(window, { shellTransitions: 0 });
    document.startViewTransition = update => { (window as unknown as { shellTransitions: number }).shellTransitions++; return original(update); };
  });
  const position = () => page.evaluate(() => { const header = document.querySelector('.page-header')!.getBoundingClientRect(); const toolbar = document.querySelector('.toolbar')!.getBoundingClientRect(); return [header.x, header.y, toolbar.x, toolbar.y, document.querySelector('.main-panel')!.clientWidth]; });
  const before = await position();
  const assertPosition = async () => { const after = await position(); after.forEach((value, i) => expect(Math.abs(value - before[i])).toBeLessThan(1)); };
  await page.locator('.calendar-cell:not(.selected):not(.outside)').nth(4).click(); await assertPosition();
  await page.getByRole('button', { name: 'Следующий период', exact: true }).click(); await assertPosition();
  await page.getByRole('button', { name: 'Неделя', exact: true }).click(); await assertPosition();
  await page.getByRole('button', { name: 'День', exact: true }).click(); await assertPosition();
  await page.getByRole('button', { name: 'Месяц', exact: true }).click(); await assertPosition();
  expect(await page.evaluate(() => (window as unknown as { shellTransitions: number }).shellTransitions)).toBe(0);
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 900 });
    const circle = await page.locator('.calendar-cell.selected .day-number').boundingBox(); if (!circle) throw new Error('Selected date missing'); expect(Math.abs(circle.width - circle.height)).toBeLessThan(.5);
  }
  await noOverflow(page);
});

test('completed reminder collapses, explains its disappearance and can be undone', async ({ page, request }, info) => {
  const health = await (await request.get('/health')).json() as Health; const today = dayKey(health.now, health.timezone); const token = Date.now(); const ids: number[] = [];
  try {
    for (const time of ['08:01', '08:02', '08:03']) { const response = await request.post('/api/events', { data: { kind: 'reminder', title: `Плавное напоминание ${token} ${time}`, date: addDays(today, 1), time } }); expect(response.status()).toBe(201); ids.push((await response.json()).id); }
    await ready(page, '/reminders'); await page.getByRole('textbox', { name: 'Поиск напоминаний' }).fill(`Плавное напоминание ${token}`);
    const title = `Плавное напоминание ${token} 08:01`; const card = page.locator('.reminder-card').filter({ hasText: title });
    await card.locator('.reminder-title').click(); await expect(card.getByRole('form')).toBeVisible(); await expect(card.locator('.reminder-title')).toBeVisible();
    await page.evaluate(() => {
      const original = Element.prototype.animate; const exits: string[] = []; Object.assign(window, { reminderExits: exits });
      Element.prototype.animate = function(frames, options) { if (Array.isArray(frames) && frames.at(-1)?.height === '0px' && this.querySelector('.reminder-card')) exits.push(this.textContent || ''); return original.call(this, frames, options); };
    });
    await card.getByRole('checkbox', { name: `Завершить «${title}»`, exact: true }).check();
    await expect(page.locator('.action-notice')).toContainText('перенесена в архив');
    expect(await page.evaluate(title => (window as unknown as { reminderExits: string[] }).reminderExits.some(value => value.includes(title)), title)).toBe(true);
    await screenshot(page, 'reminder-completed', info);
    await page.locator('.action-notice').getByRole('button', { name: 'Отменить', exact: true }).click();
    await expect.poll(async () => (await (await request.get(`/api/events/${ids[0]}`)).json()).status).toBe('open');
    await expect(card.locator('.reminder-title')).toBeVisible(); await noOverflow(page);
  } finally { for (const id of ids) await request.delete(`/api/events/${id}`); }
});

test('task, meeting and integration filters keep the shell and page heading stable', async ({ page }) => {
  for (const path of ['/tasks', '/meetings', '/integrations']) {
    await ready(page, path);
    const before = await page.locator('.page-header').evaluate(node => { const rect = node.getBoundingClientRect(); return [rect.x, rect.y, rect.width]; });
    await page.evaluate(() => { Object.assign(window, { unexpectedSnapshots: 0 }); const original = document.startViewTransition.bind(document); document.startViewTransition = update => { (window as unknown as { unexpectedSnapshots: number }).unexpectedSnapshots++; return original(update); }; });
    if (path === '/tasks') {
      const filters = page.getByRole('group', { name: 'Фильтр задач' }); await filters.getByRole('button', { name: /^Сегодня/ }).click(); await filters.getByRole('button', { name: /^Скоро/ }).click(); await filters.getByRole('button', { name: /^Все/ }).click();
    } else if (path === '/meetings') {
      await chooseOption(page.getByRole('combobox', { name: 'Период встреч' }), 'Сегодня'); await page.getByRole('button', { name: 'Онлайн', exact: true }).click(); await page.getByRole('button', { name: 'Офлайн', exact: true }).click();
    } else {
      const filters = page.getByRole('group', { name: 'Фильтр интеграций' }); await filters.getByRole('button', { name: 'Подключено', exact: true }).click(); await filters.getByRole('button', { name: 'Доступно', exact: true }).click();
    }
    const after = await page.locator('.page-header').evaluate(node => { const rect = node.getBoundingClientRect(); return [rect.x, rect.y, rect.width]; });
    after.forEach((value, index) => expect(Math.abs(value - before[index])).toBeLessThan(1));
    expect(await page.evaluate(() => (window as unknown as { unexpectedSnapshots: number }).unexpectedSnapshots)).toBe(0); await noOverflow(page);
  }
});

test('deleted records survive refresh and restore from archive with the same id and metadata', async ({ page, request }, info) => {
  const health = await (await request.get('/health')).json() as Health; const date = addDays(dayKey(health.now, health.timezone), 1); const title = `Вернуть удалённую задачу ${Date.now()}`;
  const response = await request.post('/api/events', { data: { kind: 'task', title, date, priority: 'high', tags: ['Контроль'], subtasks: [{ id: 'a', title: 'Сохранить детали', done: true }] } }); expect(response.status()).toBe(201); const event = await response.json() as RhythmEvent;
  try {
    await ready(page, '/tasks'); await page.getByRole('textbox', { name: 'Поиск задач' }).fill(title);
    const taskTitle = page.locator('.task-title').filter({ hasText: title }); await expect(taskTitle).toBeVisible();
    if (await taskTitle.getAttribute('aria-expanded') === 'true') await taskTitle.click();
    await page.getByRole('button', { name: `Действия: ${title}`, exact: true }).click(); await page.locator('.row-menu').getByRole('button', { name: 'Удалить', exact: true }).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Удалить', exact: true }).click(); await expect(page.getByRole('dialog')).toHaveCount(0);
    expect((await request.get(`/api/events/${event.id}`)).status()).toBe(404);
    await page.locator('.action-notice').getByRole('link', { name: 'Открыть архив' }).click();
    await page.getByRole('textbox', { name: 'Поиск в архиве' }).fill(title); const restore = page.getByRole('button', { name: `Восстановить «${title}»`, exact: true }); await expect(restore).toBeVisible();
    await page.reload(); await page.getByRole('textbox', { name: 'Поиск в архиве' }).fill(title); await expect(restore).toBeVisible(); await expect(page.locator('.archive-results')).toHaveAttribute('aria-busy', 'false'); await expect(page.locator('.archive-results .motion-collapse[data-open="false"]')).toHaveCount(0); await screenshot(page, 'archive-restore', info);
    await restore.click(); await expect(page.locator('.restore-notice')).toContainText(title); await expect(restore).toHaveCount(0);
    const restored = await (await request.get(`/api/events/${event.id}`)).json() as RhythmEvent;
    expect(restored.id).toBe(event.id); expect(restored.tags).toEqual(event.tags); expect(restored.subtasks).toEqual(event.subtasks); expect(restored.date).toBe(date); expect(restored.status).toBe('open'); expect(restored.deleted_at).toBeUndefined(); await noOverflow(page);
  } finally { await request.delete(`/api/events/${event.id}`); }
});
