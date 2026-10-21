import { test, expect, type Page } from '@playwright/test';
import type { RhythmEvent } from '../src/types/api';

async function mockSchedule(page: Page) {
  const user = { id: '11111111-1111-4111-8111-111111111111', name: 'Алексей Тест', email: 'alex@example.test', avatar_url: '', onboarding_completed: true, providers: ['email'] };
  const event = (id: number, kind: RhythmEvent['kind'], title: string, date: string, time: string): RhythmEvent => ({ id, kind, title, date, time, description: '', duration_minutes: 60, priority: 'normal', status: 'open', location: '', meeting_url: '', created_at: '2026-10-21T10:00:00+03:00', updated_at: '2026-10-21T10:00:00+03:00', source_key: null, external_calendar_id: null });
  const events = Array.from({ length: 35 }, (_, i) => event(i + 1, 'meeting', `Встреча ${i + 1}`, new Date(Date.UTC(2026, 9, 21 + i)).toISOString().slice(0, 10), '10:00'));
  const times = ['08:00', '08:30', '09:00', '15:45', '13:30', '14:00', '20:15', '18:00', '19:00'];
  const dates = ['2026-10-21', '2027-01-27', '2026-12-19', '2026-11-20', '2027-01-18', '2027-02-10', '2027-01-09', '2027-02-12', '2026-12-20'];
  times.forEach((time, i) => events.push({ ...event(100 + i, 'reminder', `Напоминание ${i + 1} с подробным названием`, dates[i], time), priority: i % 3 === 0 ? 'high' : 'normal', reminder_enabled: true, ...(i === 0 ? { recurrence: { frequency: 'day', interval: 1, local_time: time, timezone: 'Europe/Moscow' } as const } : {}) }));
  await page.route(url => url.pathname === '/health' || url.pathname.startsWith('/api/'), async route => {
    const url = new URL(route.request().url()); let body: unknown = {};
    if (url.pathname === '/api/auth/me') body = { user };
    else if (url.pathname.startsWith('/api/preferences/')) body = { name: user.name, widgets: false, navCollapsed: true, motion: 'system', revision: 1 };
    else if (url.pathname === '/health') body = { backend: 'go', status: 'ok', now: '2026-10-21T12:30:00+03:00', timezone: 'Europe/Moscow', events: events.length, nlp_provider: 'rules', schema_version: 5 };
    else if (url.pathname === '/api/events') { const q = (url.searchParams.get('q') || '').toLowerCase(); const found = events.filter(e => e.title.toLowerCase().includes(q)); body = { events: found, total: found.length, limit: 200, offset: 0 }; }
    else if (url.pathname === '/api/social/people') body = { profile: { ...user, code: 'ABCD-EFGH', share_email: false, timezone: 'Europe/Moscow', start_hour: 9, end_hour: 21, tags: [] }, friends: [], incoming: [], outgoing: [] };
    else if (url.pathname === '/api/social/meetings') body = { meetings: [] };
    else if (url.pathname === '/api/integrations') body = { connections: ['yandex', 'google', 'zoom'].map(provider => ({ provider, status: 'connected', last_sync: null, last_error: '', pending_events: 0 })), yandex_configured: true, google_configured: true, zoom_configured: true };
    else if (url.pathname === '/api/reminders/due') body = { events: [] };
    else if (url.pathname.includes('/history/') || url.pathname === '/api/chat/notifications') body = { exchanges: [], state: {} };
    else if (url.pathname === '/api/notifications/push/config') body = { enabled: false, public_key: '' };
    else if (url.pathname === '/api/notifications/push/subscriptions') body = { subscriptions: [] };
    await route.fulfill({ json: body });
  });
  return events;
}

test('meeting list scrolls to its final entry without scrolling the page', async ({ page }, info) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  await mockSchedule(page); await page.goto('/meetings');
  const list = page.getByRole('region', { name: 'Список встреч' });
  await expect(list.locator('.meeting-tile')).toHaveCount(30);
  for (const width of [1440, 1024, 390, 320]) {
    await page.setViewportSize({ width, height: 900 });
    await expect.poll(() => list.evaluate(node => node.clientHeight > 0 && node.scrollHeight > node.clientHeight)).toBe(true);
    expect(await page.evaluate(() => document.documentElement.scrollHeight <= innerHeight + 1)).toBe(true);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await list.evaluate(node => { node.scrollTop = 0; }); await list.hover(); await page.mouse.wheel(0, 700);
    await expect.poll(() => list.evaluate(node => node.scrollTop)).toBeGreaterThan(0);
    expect(await page.evaluate(() => scrollY)).toBe(0);
    await list.evaluate(node => { node.scrollTop = node.scrollHeight; }); await list.hover(); await page.mouse.wheel(0, 700);
    expect(await page.evaluate(() => scrollY)).toBe(0);
  }
  await page.setViewportSize({ width: 1440, height: 900 });
  await list.getByRole('button', { name: 'Показать ещё', exact: true }).click();
  await expect(list.locator('.meeting-tile')).toHaveCount(35);
  const final = list.getByRole('button', { name: /Встреча 35/ }); await final.scrollIntoViewIfNeeded();
  const bounds = (await list.boundingBox())!; const last = (await final.boundingBox())!;
  expect(last.y + last.height).toBeLessThanOrEqual(bounds.y + bounds.height);
  await final.click(); await expect(page.locator('.meeting-details h2')).toHaveText('Встреча 35');
  expect(await page.evaluate(() => scrollY)).toBe(0);
  await page.locator('.meetings-page').screenshot({ path: info.outputPath('meetings-scroll-desktop.png') });
  await page.setViewportSize({ width: 390, height: 900 });
  await page.locator('.meetings-page').screenshot({ path: info.outputPath('meetings-scroll-mobile.png') });
  expect(errors).toEqual([]);
});

test('reminder toolbar, dates and group expansion stay aligned at every width', async ({ page }, info) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  await mockSchedule(page); await page.goto('/reminders');
  await expect(page.getByText('Ничего не забывайте')).toHaveCount(0);
  const filters = page.getByRole('group', { name: 'Фильтр напоминаний' });
  await expect(filters).toHaveAttribute('data-ready', 'true');
  await expect(page.locator('.reminder-card')).toHaveCount(6);
  expect((await page.locator('.reminders-page > .page-header').boundingBox())!.height).toBeLessThan(85);
  expect(await filters.locator('button').first().evaluate(node => getComputedStyle(node).fontSize)).toBe('14px');
  const search = (await page.locator('.reminders-toolbar .search-box').boundingBox())!;
  expect(search.width).toBeLessThan((await page.locator('.reminders-toolbar').boundingBox())!.width * .7);
  const animated = await filters.evaluate(async root => {
    const pill = root.querySelector('.segment-indicator')!;
    pill.getBoundingClientRect(); (root.querySelectorAll('button')[2] as HTMLButtonElement).click();
    await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    return pill.getAnimations().length > 0;
  });
  expect(animated).toBe(true); await expect(page.locator('.reminder-card')).toHaveCount(1);
  await filters.getByRole('button', { name: 'Сегодня', exact: true }).click(); await expect(page.locator('.reminder-card')).toHaveCount(1);
  await filters.getByRole('button', { name: 'Важные', exact: true }).click(); await expect(page.locator('.reminder-card')).toHaveCount(3);
  await filters.getByRole('button', { name: 'Все', exact: true }).click(); await expect(page.locator('.reminder-card')).toHaveCount(6);
  const morning = page.getByRole('region', { name: 'Утро' });
  await morning.getByRole('button', { name: 'Показать ещё: Утро', exact: true }).click();
  await expect(morning.locator('.reminder-card')).toHaveCount(3);
  await morning.getByRole('button', { name: 'Свернуть: Утро', exact: true }).click();
  await expect(morning.locator('.reminder-card')).toHaveCount(2);
  for (const width of [1440, 1024, 760, 390, 320]) {
    await page.setViewportSize({ width, height: 1000 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    const positions = await page.locator('.reminder-time').evaluateAll(nodes => nodes.map(node => node.getBoundingClientRect().x));
    expect(Math.max(...positions) - Math.min(...positions)).toBeLessThan(1);
    for (const group of await page.locator('.reminder-groups section').all()) {
      expect(await group.locator('.reminder-group-actions').evaluate(node => getComputedStyle(node).marginTop)).toBe('8px');
      const more = group.getByRole('button', { name: /Показать ещё:/ });
      const button = (await more.boundingBox())!; const parent = (await group.boundingBox())!;
      expect(Math.abs(button.x + button.width - parent.x - parent.width)).toBeLessThan(1);
    }
    if ([1440, 390].includes(width)) {
      await page.locator('h1').click();
      await page.locator('.reminders-page').screenshot({ path: info.outputPath(`reminders-${width}.png`) });
    }
  }
  expect(errors).toEqual([]);
});

test('integrations starts close to its heading and home is named Главная', async ({ page }, info) => {
  await mockSchedule(page); await page.goto('/integrations');
  await expect(page.getByText('Подключите нужные сервисы')).toHaveCount(0);
  await expect(page.locator('.integration-card')).toHaveCount(3);
  await expect(page.getByRole('link', { name: 'Главная', exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Сегодня', exact: true })).toHaveCount(0);
  for (const width of [1440, 390, 320]) {
    await page.setViewportSize({ width, height: 900 });
    const heading = (await page.locator('h1').boundingBox())!; const search = (await page.locator('.integrations-page .search-box').boundingBox())!;
    expect(search.y - heading.y - heading.height).toBeLessThanOrEqual(25);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    if (width !== 320) await page.locator('.integrations-page').screenshot({ path: info.outputPath(`integrations-${width}.png`) });
  }
  await page.getByRole('button', { name: 'Открыть навигацию', exact: true }).click();
  const drawer = page.getByRole('dialog', { name: 'Навигация', exact: true });
  await drawer.getByRole('link', { name: 'Главная', exact: true }).click(); await expect(page).toHaveURL(/\/$/);
});

test('assistant reply shares one panel and speaks human text from its lower right edge', async ({ page }, info) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  const events = await mockSchedule(page); const event = events[0];
  const rawReply = `Изменено: #${event.id} «${event.title}» — 21.10.2026 в 10:00.`;
  const visibleReply = `Изменено: «${event.title}» — 21.10.2026 в 10:00.`;
  const history = [
    { message: 'Привет', created_at: '2026-10-21T09:20:00+03:00', response: { reply: 'Привет! Чем помочь?', request_id: 'plain-reply', events: [], action: 'help', pending: false } },
    { message: 'Измени встречу', created_at: '2026-10-21T09:21:00+03:00', response: { reply: rawReply, request_id: 'zoom-reply', events: [event], action: 'update', pending: false, zoom_meetings: [{ event_id: event.id, title: event.title, scheduled_at: '2026-10-21T10:00:00+03:00', timezone: 'Europe/Moscow', status: 'scheduled', join_url: 'https://zoom.us/j/123456789' }] } },
  ];
  await page.route('**/api/history/**', route => route.fulfill({ json: { exchanges: history, state: {} } }));
  let spoken = ''; await page.route('**/api/voice/synthesize', async route => {
    spoken = route.request().postDataJSON().text;
    await route.fulfill({ status: 503, json: { error: { message: 'Тестовая ошибка озвучки', code: 'tts_error' } } });
  });
  await page.goto('/');
  const answer = page.locator('#chat-message-zoom-reply .assistant-reply'); const bubble = answer.locator(':scope > .message-bubble');
  await expect(bubble.locator('.reply-text')).toHaveText(visibleReply);
  await expect(bubble.getByRole('region', { name: `Zoom: ${event.title}` })).toBeVisible();
  await expect(bubble.getByRole('link', { name: 'Подключиться', exact: true })).toHaveAttribute('href', 'https://zoom.us/j/123456789');
  await expect(bubble.getByRole('link', { name: 'Открыть в календаре' })).toHaveAttribute('href', '/calendar?date=2026-10-21');
  await expect(bubble.getByRole('button', { name: 'Скопировать ссылку' })).toBeEnabled();
  const speech = answer.getByRole('button', { name: 'Озвучить ответ', exact: true });
  await expect(bubble.locator('.tts-button')).toHaveCount(0);
  await answer.evaluate(async root => { await Promise.all(root.getAnimations().map(animation => animation.finished)); });
  for (const [width, height] of [[1440, 900], [768, 1024], [390, 844], [320, 568]]) {
    await page.setViewportSize({ width, height });
    const geometry = await answer.evaluate(root => {
      const panel = root.querySelector('.message-bubble')!.getBoundingClientRect(); const audio = root.querySelector('.tts-button')!.getBoundingClientRect();
      const children = [...root.querySelectorAll('.reply-text, .zoom-meeting-card, .calendar-link')].map(node => node.getBoundingClientRect());
      return { right: panel.right - audio.right, gap: audio.top - panel.bottom, enclosed: children.every(rect => rect.top >= panel.top && rect.bottom <= panel.bottom && rect.left >= panel.left && rect.right <= panel.right), background: getComputedStyle(root.querySelector('.message-bubble')!).backgroundColor, docWidth: document.documentElement.scrollWidth, docHeight: document.documentElement.scrollHeight, composerBottom: document.querySelector('.composer')!.getBoundingClientRect().bottom };
    });
    expect(Math.abs(geometry.right)).toBeLessThanOrEqual(3); expect(geometry.gap).toBeGreaterThanOrEqual(5);
    expect(geometry.enclosed).toBe(true); expect(geometry.background).not.toBe('rgba(0, 0, 0, 0)');
    expect(geometry.docWidth).toBeLessThanOrEqual(width); expect(geometry.docHeight).toBeLessThanOrEqual(height + 1); expect(geometry.composerBottom).toBeLessThan(height);
    await page.getByRole('log').evaluate(root => { root.scrollTop = root.scrollHeight; }); await expect(speech).toBeInViewport();
    if ([1440, 390].includes(width)) await page.screenshot({ path: info.outputPath(`unified-answer-${width}.png`) });
  }
  await speech.click(); await expect.poll(() => spoken).toBe(visibleReply); await expect(answer.getByRole('alert')).toHaveText('Тестовая ошибка озвучки');
  expect(history[1].response.reply).toBe(rawReply);
  await bubble.getByRole('link', { name: 'Открыть в календаре' }).click(); await expect(page).toHaveURL(/calendar\?date=2026-10-21/);
  expect(errors).toEqual([]);
});

test('archive has a compact heading and animated history/deleted tabs', async ({ page }, info) => {
  await mockSchedule(page); await page.goto('/archive');
  await expect(page.getByText('История дел и восстановление удалённых записей')).toHaveCount(0);
  const tabs = page.getByRole('group', { name: 'Раздел архива' });
  await expect(tabs).toHaveAttribute('data-ready', 'true');
  const moving = await tabs.evaluate(async root => {
    const pill = root.querySelector('.segment-indicator')!; pill.getBoundingClientRect();
    (root.querySelectorAll('button')[1] as HTMLButtonElement).click();
    await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    return pill.getAnimations().length > 0;
  });
  expect(moving).toBe(true); await expect(page).toHaveURL(/view=deleted/);
  await page.reload(); await expect(tabs.getByRole('button', { name: 'Удалённые', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await tabs.getByRole('button', { name: 'История', exact: true }).click(); await expect(page).not.toHaveURL(/view=deleted/);
  for (const width of [1440, 1024, 390, 320]) {
    await page.setViewportSize({ width, height: 1000 });
    const title = (await page.locator('h1').boundingBox())!; const track = (await tabs.boundingBox())!;
    expect(track.y - title.y - title.height).toBeLessThanOrEqual(25);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    if ([1440, 390].includes(width)) { await page.locator('h1').click(); await page.locator('.archive-page').screenshot({ path: info.outputPath(`archive-${width}.png`) }); }
  }
  await page.goto('/reminders'); await expect(page.locator('.reminder-card')).toHaveCount(6);
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 1000 });
    const morning = page.getByRole('region', { name: 'Утро' }); const cards = await morning.locator('.reminder-card').all();
    const first = (await cards[0].boundingBox())!; const second = (await cards[1].boundingBox())!;
    expect(Math.abs(second.y - first.y - first.height - 7)).toBeLessThan(1);
    expect(await page.locator('.reminder-groups').evaluate(node => getComputedStyle(node).rowGap)).toBe('12px');
    await page.locator('.reminders-page').screenshot({ path: info.outputPath(`compact-reminders-${width}.png`) });
  }
});

test('settings stays above the collapsed avatar and avatar menu only logs out', async ({ page }, info) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  await mockSchedule(page); await page.goto('/archive');
  const trigger = page.getByRole('button', { name: 'Меню аккаунта', exact: true });
  const settings = page.getByRole('link', { name: 'Настройки', exact: true });
  await expect(settings).toBeVisible();
  const settingsBox = (await settings.boundingBox())!; const avatarBox = (await trigger.boundingBox())!;
  expect(settingsBox.y + settingsBox.height).toBeLessThan(avatarBox.y);
  await trigger.click(); const menu = page.getByRole('menu', { name: 'Аккаунт' });
  await expect(menu.getByRole('menuitem')).toHaveCount(1); await expect(menu.getByRole('menuitem', { name: 'Выйти', exact: true })).toBeFocused();
  await page.keyboard.press('ArrowDown'); await expect(menu.getByRole('menuitem', { name: 'Выйти', exact: true })).toBeFocused();
  await page.keyboard.press('Escape'); await expect(menu).toHaveCount(0); await expect(trigger).toBeFocused();
  await settings.click(); await expect(page.locator('h1')).toHaveText('Настройки');
  await page.getByRole('button', { name: 'Развернуть навигацию', exact: true }).click();
  await expect(settings).toBeVisible(); await trigger.click(); await expect(menu.getByRole('menuitem')).toHaveCount(1); await page.keyboard.press('Escape');
  await page.setViewportSize({ width: 1024, height: 900 }); await expect(settings).toBeVisible();
  expect((await settings.boundingBox())!.y + (await settings.boundingBox())!.height).toBeLessThan((await trigger.boundingBox())!.y);
  await page.setViewportSize({ width: 390, height: 844 }); await page.getByRole('button', { name: 'Открыть навигацию', exact: true }).click();
  const drawer = page.getByRole('dialog', { name: 'Навигация', exact: true });
  await drawer.getByRole('button', { name: 'Меню аккаунта', exact: true }).click();
  await expect(drawer.getByRole('menuitem')).toHaveCount(1); await expect(drawer.getByRole('menuitem', { name: 'Настройки', exact: true })).toHaveCount(0);
  await page.keyboard.press('Escape'); await drawer.getByRole('link', { name: 'Настройки', exact: true }).click(); await expect(drawer).toHaveCount(0);
  await page.getByRole('button', { name: 'Открыть навигацию', exact: true }).click();
  await drawer.getByRole('button', { name: 'Меню аккаунта', exact: true }).click();
  await page.screenshot({ path: info.outputPath('avatar-logout-mobile.png') });
  let logouts = 0; page.on('request', request => { if (request.method() === 'POST' && request.url().endsWith('/api/auth/logout')) logouts++; });
  await drawer.getByRole('menuitem', { name: 'Выйти', exact: true }).click(); await expect(page).toHaveURL(/\/auth$/); expect(logouts).toBe(1);
  expect(errors).toEqual([]);
});
