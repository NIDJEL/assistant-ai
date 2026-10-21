import { test, expect } from '@playwright/test';
import { legalAcceptance } from './legal-acceptance';
import { ready, noOverflow, screenshot } from './helpers';

test.beforeEach(async ({ context }, info) => {
  const response = await context.request.post('/api/auth/register', { data: { ...legalAcceptance, name: 'Проверка интерфейса', email: `ui-${Date.now()}-${info.testId.replace(/\W/g, '').slice(-12)}@example.test`, password: 'ui test password 12345', password_confirmation: 'ui test password 12345' } });
  expect(response.status()).toBe(201); expect((await context.request.post('/api/auth/onboarding', { data: {} })).status()).toBe(200);
});
test('reference calendar keeps compact event pills and opens all remaining events for the chosen day', async ({ page, context }, info) => {
  const selected = '2040-10-20'; const other = '2040-10-21';
  await page.route('**/health', async route => { const response = await route.fetch(); await route.fulfill({ response, json: { ...await response.json(), now: `${selected}T12:00:00+03:00` } }); });
  for (let i = 0; i < 8; i++) {
    const created = await context.request.post('/api/events', { data: { kind: i % 3 === 0 ? 'task' : i % 3 === 1 ? 'meeting' : 'reminder', title: `Проверить подробный план следующего этапа проекта ${i}`, date: i < 5 ? selected : other, time: `1${i}:00`, duration_minutes: 30, priority: i === 2 ? 'high' : 'normal', force: true } });
    expect(created.status()).toBe(201);
  }
  await ready(page, `/calendar?date=${selected}`);
  const chosen = page.locator('.calendar-cell.selected'); const next = page.getByRole('button', { name: '21 октября, событий: 3', exact: true });
  await expect(chosen.locator('.cell-event')).toHaveCount(2); await expect(chosen.locator('.cell-more')).toHaveText('+ ещё 3');
  await expect(next.locator('.cell-event')).toHaveCount(1); await expect(next.locator('.cell-more')).toHaveText('+ ещё 2');
  await expect(chosen.locator('.day-number')).toHaveClass(/today/); await expect(chosen.locator('.cell-event-title').first()).toHaveCSS('white-space', 'nowrap');
  await expect(chosen.locator('.cell-event').first()).toHaveAttribute('title', /Проверить подробный план следующего этапа проекта/);
  const initialHeight = (await page.locator('.month-calendar').boundingBox())!.height;
  await next.locator('.cell-more').click(); await expect(page).toHaveURL(new RegExp(`date=${other}`)); await expect(page.locator('.selected-day .day-event')).toHaveCount(3); await expect(page.locator('.selected-day')).toBeInViewport();
  expect((await page.locator('.month-calendar').boundingBox())!.height).toBe(initialHeight);
  for (const width of [1440, 1080, 390]) {
    await page.setViewportSize({ width, height: 900 }); await ready(page, `/calendar?date=${selected}`); await noOverflow(page);
    const cell = (await chosen.boundingBox())!;
    if (width > 760) { const more = (await chosen.locator('.cell-more').boundingBox())!; expect(more.y + more.height).toBeLessThanOrEqual(cell.y + cell.height); }
    else await expect(chosen.locator('.cell-mobile-count')).toHaveText('5 соб.');
    await page.locator('.month-calendar').screenshot({ path: info.outputPath(`calendar-reference-${width}.png`) });
  }
});
for (const width of [1440, 390]) test(`custom date picker in a modal, keyboard, outside click and ISO save at ${width}px`, async ({ page }, info) => {
  await page.setViewportSize({ width, height: 900 }); await ready(page, '/calendar');
  await page.getByRole('button', { name: 'Новое событие' }).click(); const editor = page.getByRole('dialog', { name: 'Новая встреча' });
  await editor.getByLabel('Название', { exact: true }).fill('Проверка календаря'); await editor.getByLabel('Дата', { exact: true }).fill('2040-02-28');
  await editor.getByRole('button', { name: 'Выбрать дату' }).click(); const picker = page.getByRole('dialog', { name: 'Выбор даты' });
  await expect(picker.locator('[data-date="2040-02-28"]')).toBeFocused(); await page.keyboard.press('ArrowRight'); await expect(picker.locator('[data-date="2040-02-29"]')).toBeFocused();
  await page.keyboard.press('Enter'); await expect(picker).toHaveCount(0); await expect(editor.getByLabel('Дата', { exact: true })).toHaveValue('29.02.2040');
  await editor.getByRole('button', { name: 'Выбрать дату' }).click(); await expect(picker).toBeVisible(); await screenshot(page, `date-picker-${width}`, info);
  const bounds = (await picker.boundingBox())!; expect(bounds.x).toBeGreaterThanOrEqual(10); expect(bounds.x + bounds.width).toBeLessThanOrEqual(width - 10); expect(bounds.y + bounds.height).toBeLessThanOrEqual(890);
  await picker.getByRole('button', { name: 'Следующий период выбора даты' }).click(); await page.keyboard.press('Tab'); await page.keyboard.press('Tab'); await expect(picker.locator('[data-date="2040-03-29"]')).toBeFocused();
  await page.keyboard.press('Escape'); await expect(picker).toHaveCount(0); await expect(editor).toBeVisible(); await expect(editor.getByLabel('Дата', { exact: true })).toBeFocused();
  await editor.getByRole('button', { name: 'Выбрать дату' }).click(); await expect(picker).toBeVisible(); await page.mouse.click(2, 2); await expect(picker).toHaveCount(0); await expect(editor).toBeVisible();
  await editor.getByLabel('Время', { exact: true }).fill('12:35');
  const note = editor.getByRole('textbox', { name: 'Заметка', exact: true }); const before = (await note.boundingBox())!.height;
  await note.fill(Array(12).fill('Строка заметки').join('\n'));
  expect((await note.boundingBox())!.height).toBeGreaterThan(before); await expect(note).toHaveCSS('resize', 'none');
  const saved = page.waitForResponse(r => r.url().endsWith('/api/events') && r.request().method() === 'POST'); await editor.getByRole('button', { name: 'Сохранить', exact: true }).click(); const response = await saved; expect(response.status()).toBe(201); expect(await response.json()).toMatchObject({ date: '2040-02-29', time: '12:35' }); await expect(editor).toHaveCount(0); await noOverflow(page);
});
test('friend checkbox is intrinsic, widget capabilities and date states are distinct', async ({ page }, info) => {
  await page.route('**/api/social/people', async route => {
    const response = await route.fetch(); const body = await response.json(); body.friends = [{ id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', name: 'Друг для проверки', tags: ['Коллега'], email: '', avatar_url: '' }]; await route.fulfill({ response, json: body });
  });
  await ready(page, '/calendar'); await expect(page.getByRole('button', { name: 'Настройки: Календарь' })).toHaveCount(0); await expect(page.getByRole('button', { name: 'Настройки: Сегодня' })).toBeVisible();
  const today = page.locator('.calendar-cell[aria-current="date"]'); const label = await today.getAttribute('aria-label');
  await page.locator('.calendar-cell:not([aria-current="date"]):not(.outside)').first().click(); await expect(today).not.toHaveClass(/selected/); await expect(today.locator('.day-number')).toHaveClass(/today/); await expect(page.locator('.calendar-cell.selected .day-number')).not.toHaveClass(/today/); expect(await today.getAttribute('aria-label')).toBe(label);
  await expect(page.locator('.widgets-panel .mini-month-heading')).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)');
  await page.getByRole('button', { name: 'Новое событие' }).click(); const editor = page.getByRole('dialog'); const friend = editor.getByRole('checkbox', { name: /Друг для проверки/ });
  await friend.check(); const size = (await friend.boundingBox())!; expect(size.width).toBeLessThanOrEqual(22); expect(size.height).toBeLessThanOrEqual(22); await expect(editor.locator('.friend-picker-list label')).toHaveCSS('display', 'flex'); await screenshot(page, 'friend-picker', info); await noOverflow(page);
  await page.keyboard.press('Escape'); await ready(page, '/friends'); await expect(page.getByRole('button', { name: /Настройки доступности/ })).toHaveCount(0); await expect(page.getByRole('button', { name: 'Скопировать код дружбы' })).toBeVisible();
});
test('chat event cards and calendar actions use dark surfaces', async ({ page, context }, info) => {
  const created = await context.request.post('/api/events', { data: { kind: 'meeting', title: 'Встреча с дизайнером', description: '', date: '2040-03-01', time: '13:00', duration_minutes: 30, priority: 'normal', status: 'open', force: true } }); expect(created.status()).toBe(201); const event = await created.json();
  await page.route('**/api/chat', async route => { const input = route.request().postDataJSON(); await route.fulfill({ json: { reply: 'Встреча сохранена.', action: 'create', events: [event], candidates: null, conflicts: null, pending: false, provider: 'rules', timezone: 'Europe/Moscow', session_id: input.session_id, request_id: input.request_id } }); });
  await ready(page); await page.getByRole('textbox', { name: 'Сообщение ассистенту' }).fill('Запланируй встречу'); await page.getByRole('button', { name: 'Отправить сообщение' }).click();
  await expect(page.locator('.chat-event-card')).toBeVisible();
  for (const selector of ['.chat-event-card', '.calendar-link']) {
    const color = await page.locator(selector).evaluate(element => getComputedStyle(element).backgroundColor.match(/\d+/g)!.slice(0, 3).map(Number)); expect(Math.max(...color)).toBeLessThan(90);
  }
  expect((await page.locator('.calendar-link').boundingBox())!.width).toBeLessThan(300); await screenshot(page, 'dark-chat-card', info);
});
