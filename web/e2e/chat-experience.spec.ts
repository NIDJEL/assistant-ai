import { test, expect, type BrowserContext } from '@playwright/test';
import { legalAcceptance } from './legal-acceptance';
import { noOverflow, ready } from './helpers';

async function register(context: BrowserContext, name: string) {
  const response = await context.request.post('/api/auth/register', { data: { ...legalAcceptance, name, email: `chat-${crypto.randomUUID()}@example.test`, password: 'chat experience test password 12345', password_confirmation: 'chat experience test password 12345' } });
  expect(response.status()).toBe(201); const { user } = await response.json();
  expect((await context.request.post('/api/auth/onboarding', { data: {} })).status()).toBe(200);
  await context.request.put(`/api/preferences/${user.id}`, { data: { widgets: false, revision: 1 } });
  return user;
}
function exchange(index: number, notification = false) {
  return { message: `Сообщение ${index}`, notification, read: false, created_at: new Date(Date.UTC(2026, 9, 21, 8, index)).toISOString(), response: { request_id: `00000000-0000-4000-8000-${String(index).padStart(12, '0')}`, session_id: 'notifications', reply: notification ? 'Новое сообщение о встрече' : `Ответ на сообщение ${index}. Проверяем удобство чтения длинной истории.`, events: notification ? null : [], action: 'list', candidates: null, conflicts: null, provider: 'rules', timezone: 'Europe/Moscow', pending: false } };
}
test('greeting gives way to the conversation and chat fits desktop and narrow phones', async ({ page }, info) => {
  await register(page.context(), 'Чат Тест'); await ready(page);
  await expect(page.getByRole('heading', { name: 'Чем помочь?' })).toBeVisible();
  await page.getByRole('textbox', { name: 'Сообщение ассистенту' }).fill('Создай задачу проверить чат завтра');
  await page.getByRole('button', { name: 'Отправить сообщение' }).click();
  await expect(page.locator('.assistant-message .reply-text')).toBeVisible();
  await expect(page.getByText('Чем помочь?', { exact: true })).toHaveCount(0);
  await expect(page.getByText('Скажите задачу или просто попросите.')).toHaveCount(0);
  await page.reload(); await expect(page.locator('.today-page')).toHaveAttribute('aria-busy', 'false');
  for (const [width, height] of [[1920, 1080], [1440, 900], [768, 1024], [390, 844], [320, 568]]) {
    await page.setViewportSize({ width, height }); await noOverflow(page);
    const geometry = await page.evaluate(() => {
      const chat = document.querySelector('.conversation')!; const input = document.querySelector('.composer')!.getBoundingClientRect();
      return { top: chat.getBoundingClientRect().top, width: chat.clientWidth, scrollbar: getComputedStyle(chat).scrollbarWidth, documentHeight: document.documentElement.scrollHeight, viewport: innerHeight, bottom: input.bottom };
    });
    expect(geometry.top).toBeLessThanOrEqual(width <= 760 ? 65 : width <= 1179 ? 89 : 26);
    expect(geometry.scrollbar).toBe('none'); expect(geometry.documentHeight).toBeLessThanOrEqual(height + 1); expect(geometry.bottom).toBeLessThan(height);
    if (width === 1920) expect(geometry.width).toBe(1040);
    const controlsOverlap = await page.evaluate(() => {
      const bubble = document.querySelector('.user-message .message-bubble')!.getBoundingClientRect();
      return [...document.querySelectorAll('.shell-controls button')].some(button => {
        const rect = button.getBoundingClientRect();
        return rect.width > 0 && rect.left < bubble.right && rect.right > bubble.left && rect.top < bubble.bottom && rect.bottom > bubble.top;
      });
    });
    expect(controlsOverlap).toBe(false);
    if (width === 1920 || width === 390) await page.screenshot({ path: info.outputPath(`chat-${width}.png`) });
  }
});
test('new messages do not steal scroll position, open once from the notice and stay read', async ({ page }, info) => {
  await register(page.context(), 'История Тест');
  const history = Array.from({ length: 24 }, (_, i) => exchange(i)); let notifications: ReturnType<typeof exchange>[] = []; const acknowledgements: string[][] = [];
  await page.route('**/api/history/**', route => route.fulfill({ json: { exchanges: history, state: {} } }));
  await page.route('**/api/chat/notifications', route => route.fulfill({ json: { exchanges: notifications } }));
  await page.route('**/api/chat/notifications/read', async route => {
    const { ids } = route.request().postDataJSON(); acknowledgements.push(ids); notifications = notifications.map(item => ids.includes(item.response.request_id) ? { ...item, read: true } : item);
    await route.fulfill({ status: 204 });
  });
  await ready(page); const log = page.getByRole('log');
  await log.evaluate(element => { element.scrollTop = 250; element.dispatchEvent(new Event('scroll')); });
  notifications = [exchange(30, true)]; history.push(notifications[0]); // Same persisted ID must render only once.
  await page.evaluate(() => window.dispatchEvent(new Event('focus')));
  await expect(page.getByText('Новое сообщение о встрече', { exact: true })).toHaveCount(1);
  expect(await log.evaluate(element => element.scrollTop)).toBeCloseTo(250, 0); expect(acknowledgements).toHaveLength(0);
  await log.hover(); await page.mouse.wheel(0, 160);
  await expect.poll(() => log.evaluate(element => element.scrollTop)).toBeGreaterThan(250);
  await page.getByRole('navigation', { name: 'Основная навигация' }).getByRole('link', { name: 'Календарь', exact: true }).click();
  await page.getByRole('link', { name: 'Новые сообщения Гены: 1', exact: true }).click();
  const notification = page.locator('[data-notification-id]'); await expect(notification).toHaveCount(1); await expect(notification).toBeInViewport();
  await expect.poll(() => notifications[0].read).toBe(true); expect(acknowledgements.flat()).toEqual([notifications[0].response.request_id]);
  await expect(notification.locator('.user-message')).toHaveCount(0); await expect(notification).toContainText('Новое сообщение о встрече');
  await page.screenshot({ path: info.outputPath('opened-notification.png') });
  await page.reload(); await expect(page.locator('.today-page')).toHaveAttribute('aria-busy', 'false');
  await expect(page.getByText('Новое сообщение о встрече', { exact: true })).toHaveCount(1);
  await page.getByRole('navigation', { name: 'Основная навигация' }).getByRole('link', { name: 'Календарь', exact: true }).click();
  await expect(page.getByRole('link', { name: /Новые сообщения Гены:/ })).toHaveCount(0);
});
test('a real persisted friend notification is displayed and acknowledged after opening the chat', async ({ browser }) => {
  const baseURL = process.env.P1_BASE_URL || 'http://127.0.0.1:8083';
  const a = await browser.newContext({ baseURL }); const b = await browser.newContext({ baseURL });
  try {
    await register(a, 'Алиса Уведомления'); await register(b, 'Борис Уведомления');
    const code = (await (await a.request.get('/api/social/people')).json()).profile.code;
    const sender = await b.newPage(); await sender.goto('/friends');
    await sender.getByRole('textbox', { name: 'Код друга', exact: true }).fill(code);
    await sender.getByRole('button', { name: 'Отправить заявку', exact: true }).click(); await expect(sender.getByRole('status')).toContainText('Заявка отправлена');
    const recipient = await a.newPage(); await ready(recipient, '/calendar');
    await recipient.getByRole('link', { name: 'Новые сообщения Гены: 1' }).click();
    await expect(recipient.getByRole('log')).toContainText('Борис Уведомления хочет добавить вас в друзья.');
    await expect.poll(async () => (await (await a.request.get('/api/chat/notifications')).json()).exchanges.every((item: { read: boolean }) => item.read)).toBe(true);
    await expect(recipient.locator('.user-message')).toHaveCount(0);
    await ready(recipient, '/calendar'); await expect(recipient.getByRole('link', { name: /Новые сообщения Гены:/ })).toHaveCount(0);
    await ready(recipient); await expect(recipient.getByRole('log')).toContainText('Борис Уведомления хочет добавить вас в друзья.');
  } finally { await a.close(); await b.close(); }
});
