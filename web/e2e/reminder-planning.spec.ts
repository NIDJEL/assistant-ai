import { test, expect } from './fixtures';
import { ready, screenshot } from './helpers';
import type { ChatResponse, Health } from '../src/types/api';

test('the notification service worker installs from the production Go static route', async ({ page }) => {
  await ready(page);
  const worker = await page.evaluate(async () => {
    await navigator.serviceWorker.register('/notifications-sw.js');
    const registration = await navigator.serviceWorker.ready;
    return { scope: new URL(registration.scope).pathname };
  });
  expect(worker.scope).toBe('/');
  await expect.poll(() => page.evaluate(async () => (await navigator.serviceWorker.getRegistration('/'))?.active?.state)).toBe('activated');
});

test('event subject, event time and explicit notification clock stay separate through editing and reload', async ({ page, request }, info) => {
  await ready(page);
  const draft = page.getByRole('textbox', { name: 'Сообщение ассистенту' });
  await draft.fill('у меня завтра пара в 12:50 Поставь пожалуйста уведомление на 12:00 чтобы я не опоздал на пару');
  const sent = page.waitForResponse(r => r.url().endsWith('/api/chat') && r.request().method() === 'POST');
  await draft.press('Enter'); const response = await (await sent).json() as ChatResponse;
  expect(response.action).toBe('create'); expect(response.events[0]).toMatchObject({ title: 'Пара', time: '12:50', reminder_enabled: true, reminder_minutes: 50 });
  await expect(page.getByText('За 50 мин.', { exact: true })).toBeVisible();
  await expect(page.locator('.reply-text').last()).toContainText('12:00');
  await screenshot(page, 'separate-event-and-reminder', info);
  await page.locator('.chat-event-card').last().click();
  const dialog = page.getByRole('dialog'); await expect(dialog.getByLabel('Время', { exact: true })).toHaveValue('12:50');
  await dialog.getByLabel('Когда напомнить', { exact: true }).selectOption('60');
  const updated = page.waitForResponse(r => r.url().endsWith(`/api/events/${response.events[0].id}`) && r.request().method() === 'PATCH');
  await dialog.getByRole('button', { name: 'Сохранить', exact: true }).click(); await updated;
  const persisted = await (await request.get(`/api/events/${response.events[0].id}`)).json();
  expect(persisted).toMatchObject({ title: 'Пара', time: '12:50', reminder_minutes: 60 });
  await page.reload(); await expect(page.getByText('За 1 ч.', { exact: true })).toBeVisible();
});

test('missing lead time is clarified before creation and survives a reload', async ({ page, request }) => {
  await ready(page); const before = await (await request.get('/health')).json() as Health;
  const draft = page.getByRole('textbox', { name: 'Сообщение ассистенту' });
  await draft.fill('У меня завтра собеседование в 15:00. Поставь напоминание');
  const asked = page.waitForResponse(r => r.url().endsWith('/api/chat') && r.request().method() === 'POST'); await draft.press('Enter');
  const question = await (await asked).json() as ChatResponse; expect(question.action).toBe('clarify'); expect(question.pending).toBe(true); expect(question.events).toHaveLength(0);
  expect((await (await request.get('/health')).json() as Health).events).toBe(before.events);
  await page.reload(); await page.getByRole('button', { name: 'За час', exact: true }).click();
  await expect(draft).toHaveValue('За час');
  const saved = page.waitForResponse(r => r.url().endsWith('/api/chat') && r.request().method() === 'POST'); await draft.press('Enter');
  const event = (await (await saved).json() as ChatResponse).events[0];
  expect(event).toMatchObject({ title: 'Собеседование', time: '15:00', reminder_minutes: 60 });
});

test('due events show an in-app alert and a system notification only once after reload', async ({ page, context, request }) => {
  await context.addInitScript(() => {
    const notices: unknown[] = [];
    class FakeNotification { static permission = 'granted'; onclick: (() => void) | null = null; constructor(title: string, options: unknown) { notices.push({ title, options }); } close() {} }
    Object.assign(window, { Notification: FakeNotification, __rhythmNotices: notices });
    Object.defineProperty(navigator, 'serviceWorker', { value: undefined, configurable: true });
  });
  const me = await (await request.get('/api/auth/me')).json(); const preferencesURL = `/api/preferences/${me.user.id}`;
  const preferences = await (await request.get(preferencesURL)).json();
  await request.put(preferencesURL, { data: { ...preferences, browserNotifications: true, revision: Date.now() } });
  const health = await (await request.get('/health')).json() as Health;
  const parts = new Intl.DateTimeFormat('sv-SE', { timeZone: health.timezone, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).formatToParts(new Date(Date.parse(health.now) + 3600000));
  const value = (name: string) => parts.find(part => part.type === name)!.value;
  const created = await request.post('/api/events', { data: { kind: 'reminder', title: 'Проверка уведомления заранее', date: `${value('year')}-${value('month')}-${value('day')}`, time: `${value('hour')}:${value('minute')}`, reminder_enabled: true, reminder_minutes: 60 } });
  expect(created.status()).toBe(201);
  await ready(page);
  await expect(page.locator('.reminder-notice')).toContainText('Проверка уведомления заранее');
  await expect.poll(() => page.evaluate(() => (window as unknown as { __rhythmNotices: unknown[] }).__rhythmNotices.length)).toBe(1);
  await page.reload(); await expect(page.locator('h1')).toBeVisible();
  const due = page.waitForResponse(r => r.url().endsWith('/api/reminders/due')); await page.reload(); await due;
  await expect(page.locator('.reminder-notice')).toHaveCount(0);
  expect(await page.evaluate(() => (window as unknown as { __rhythmNotices: unknown[] }).__rhythmNotices.length)).toBe(0);
});
