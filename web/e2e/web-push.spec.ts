import { test, expect, type Page } from '@playwright/test';
import { createECDH } from 'node:crypto';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { resolve } from 'node:path';
const run = promisify(execFile);
const subscriptionKeys = createECDH('prime256v1'); subscriptionKeys.generateKeys();
async function probe(userId: string, event: number | 'test') {
  const { stdout } = await run('go', ['run', './tests/e2e-push', userId, String(event)], { cwd: resolve('..') });
  return JSON.parse(stdout) as { payload?: Record<string, unknown>; deliveries?: number };
}
test.beforeEach(async ({ context }) => {
  const response = await context.request.post('/api/auth/register', { data: { name: 'Уведомления', email: `push-${crypto.randomUUID()}@example.test`, password: 'test password 12345', password_confirmation: 'test password 12345' } });
  expect(response.status()).toBe(201); expect((await context.request.post('/api/auth/onboarding', { data: {} })).status()).toBe(200);
});
async function mockSubscription(page: Page) {
  await page.addInitScript(({ endpoint, p256dh }) => {
    // This Windows headless runtime denies OS notification permission; mock the
    // permission service and subscription provider, while keeping API/SW real.
    Object.defineProperty(Notification,'permission',{ configurable: true, get: () => 'granted' });
    Notification.requestPermission = async () => 'granted';
    let current: PushSubscription | null = null;
    const subscription = { expirationTime: null, options: {}, endpoint, toJSON: () => ({ endpoint, expirationTime: null, keys: { p256dh, auth: 'AAAAAAAAAAAAAAAAAAAAAA' } }), unsubscribe: async () => { current = null; return true; } } as unknown as PushSubscription;
    Object.defineProperty(PushManager.prototype, 'getSubscription', { configurable: true, value: async () => current });
    Object.defineProperty(PushManager.prototype, 'subscribe', { configurable: true, value: async () => { current = subscription; return current; } });
  }, { endpoint: `https://push.invalid/${crypto.randomUUID()}`, p256dh: subscriptionKeys.getPublicKey().toString('base64url') });
}
test('permission grant registers a device with the real authenticated API', async ({ page, context, baseURL }) => {
  await context.grantPermissions(['notifications'], { origin: baseURL }); await mockSubscription(page); await page.goto('/settings');
  await page.getByRole('button', { name: 'Включить уведомления', exact: true }).click();
  await expect(page.getByText('Уведомления включены', { exact: true })).toBeVisible();
  const devices = await (await context.request.get('/api/notifications/push/subscriptions')).json(); expect(devices.subscriptions).toHaveLength(1);
  expect(devices.subscriptions[0]).not.toHaveProperty('endpoint');
  await page.getByRole('button', { name: 'Выключить уведомления', exact: true }).click(); await expect(page.getByText('Уведомления выключены', { exact: true })).toBeVisible();
  expect((await (await context.request.get('/api/notifications/push/subscriptions')).json()).subscriptions[0].disabled_at).not.toBeNull();
});
test('test notification queues a server payload and the installed worker handles push', async ({ page, context, baseURL }) => {
  await context.grantPermissions(['notifications'], { origin: baseURL }); await mockSubscription(page); await page.goto('/settings');
  await page.getByRole('button', { name: 'Включить уведомления', exact: true }).click(); await expect(page.getByText('Уведомления включены', { exact: true })).toBeVisible();
  const user = (await (await context.request.get('/api/auth/me')).json()).user;
  await page.getByRole('button', { name: 'Отправить тестовое уведомление' }).click();
  const data = await probe(user.id, 'test'); expect(data.deliveries).toBe(1); expect(data.payload).toMatchObject({ title: 'Гена', body: 'Уведомления работают', kind: 'test' });
  const sw = context.serviceWorkers()[0]; expect(sw).toBeTruthy();
  // The provider/display are mocked; dispatch to the real installed worker.
  await sw.evaluate(async payload => {
    const scope = self as unknown as { registration: { showNotification: (title:string,options:NotificationOptions)=>Promise<void> }; __notices: {title:string;body?:string}[] };
    scope.__notices=[]; scope.registration.showNotification=async(title,options)=>{scope.__notices.push({title,body:options.body});};
    const Push = (self as unknown as { PushEvent: new(type: string, options: { data: string }) => Event }).PushEvent;
    const event = new Push('push', { data: JSON.stringify(payload) }); let pending: Promise<void> | undefined;
    Object.defineProperty(event, 'waitUntil', { value: (promise: Promise<void>) => { pending = promise; } });
    self.dispatchEvent(event); await pending;
  }, data.payload);
  const notices = await sw.evaluate(() => (self as unknown as { __notices: {title:string;body?:string}[] }).__notices);
  expect(notices).toContainEqual({ title: 'Гена', body: 'Уведомления работают' });
});
test('a reminder creates persistent deliveries while the page is closed', async ({ page, context, baseURL }) => {
  await context.grantPermissions(['notifications'], { origin: baseURL }); await mockSubscription(page); await page.goto('/settings');
  await page.getByRole('button', { name: 'Включить уведомления', exact: true }).click(); await expect(page.getByText('Уведомления включены', { exact: true })).toBeVisible();
  const user = (await (await context.request.get('/api/auth/me')).json()).user;
  const at = new Date(Date.now() - 60000); const date = new Intl.DateTimeFormat('sv-SE', { timeZone: 'Europe/Moscow' }).format(at); const time = new Intl.DateTimeFormat('en-GB', { timeZone: 'Europe/Moscow', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).format(at);
  const response = await context.request.post('/api/events', { data: { kind: 'reminder', title: 'Закрытая вкладка', date, time } }); expect(response.status()).toBe(201); const event = await response.json();
  await page.close(); await expect.poll(async () => (await probe(user.id, event.id)).deliveries, { timeout: 20000 }).toBe(1);
});
test('a recurring reminder advances server-side and has one occurrence delivery', async ({ context }) => {
  const config = await (await context.request.get('/api/notifications/push/config')).json();
  const user = (await (await context.request.get('/api/auth/me')).json()).user;
  expect((await context.request.post('/api/notifications/push/subscriptions', { data: { endpoint: `https://push.invalid/${crypto.randomUUID()}`, platform: 'Windows', keys: { p256dh: config.public_key, auth: 'AAAAAAAAAAAAAAAAAAAAAA' } } })).status()).toBe(201);
  const response = await context.request.post('/api/events', { data: { kind: 'reminder', title: 'Фоновое повторение', recurrence: { frequency: 'minute', interval: 1, timezone: 'Europe/Moscow' }, reminder_minutes: 1, reminder_enabled: true } });
  expect(response.status()).toBe(201); const event = await response.json();
  await expect.poll(async () => (await probe(user.id, event.id)).deliveries, { timeout: 20000 }).toBe(1);
  const next = await (await context.request.get(`/api/events/${event.id}`)).json(); expect(Date.parse(next.occurrence_at)).toBeGreaterThan(Date.parse(event.occurrence_at)); expect(next.recurrence.anchor).toBe(event.recurrence.anchor);
});
test('iPhone browser explains installation without permission prompts', async ({ page }) => {
  await page.addInitScript(() => { Object.defineProperty(navigator, 'userAgent', { get: () => 'iPhone Safari' }); Object.defineProperty(navigator, 'standalone', { value: false }); });
  await page.goto('/settings'); await expect(page.getByText('Выберите «На экран Домой».')).toBeVisible(); await expect(page.getByRole('button', { name: 'Включить уведомления', exact: true })).toHaveCount(0);
});
