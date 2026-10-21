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
    else if (url.pathname === '/api/reminders/due') body = { events: [] };
    else if (url.pathname.includes('/history/') || url.pathname === '/api/chat/notifications') body = { exchanges: [], state: {} };
    else if (url.pathname === '/api/notifications/push/config') body = { enabled: false, public_key: '' };
    await route.fulfill({ json: body });
  });
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
