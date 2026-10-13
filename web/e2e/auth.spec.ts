import { test, expect, type Page } from '@playwright/test';
import { noOverflow } from './helpers';
import { mkdir } from 'node:fs/promises';
import { resolve } from 'node:path';
const password = 'test password 12345';
async function settle(page: Page) { await page.evaluate(async () => { await Promise.allSettled(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished)); }); }
test('login and registration fit desktop and mobile windows without scrolling or clipping', async ({ page }) => {
 await page.emulateMedia({ reducedMotion: 'reduce' });
 for (const viewport of [{ width: 1536, height: 1024 }, { width: 1440, height: 900 }, { width: 1366, height: 768 }, { width: 1280, height: 720 }, { width: 593, height: 932 }, { width: 390, height: 844 }, { width: 360, height: 740 }]) {
  await page.setViewportSize(viewport); await page.goto('/auth');
  for (const mode of ['login', 'register']) {
   if (mode === 'register') { await page.getByRole('tab', { name: 'Создать аккаунт' }).click(); await expect(page.getByLabel('Повторите пароль')).toBeVisible(); }
   const size = await page.evaluate(() => ({ height: document.documentElement.scrollHeight, width: document.documentElement.scrollWidth, viewportHeight: innerHeight, viewportWidth: innerWidth }));
   expect(size.height, `${mode} at ${viewport.width}×${viewport.height}`).toBeLessThanOrEqual(size.viewportHeight);
   expect(size.width).toBeLessThanOrEqual(size.viewportWidth);
   await expect(page.locator('.auth-segment')).toBeInViewport({ ratio: 1 });
   await expect(page.locator('.auth-form .auth-primary')).toBeInViewport({ ratio: 1 });
   await expect(page.locator('.auth-legal')).toBeInViewport({ ratio: 1 });
   for (const input of await page.locator('.auth-form input').all()) await expect(input).toBeInViewport({ ratio: 1 });
   expect(await page.locator('.auth-card').evaluate(element => element.scrollHeight <= element.clientHeight + 1)).toBe(true);
   expect(await page.locator('.auth-oauth img').evaluateAll(images => images.every(image => image instanceof HTMLImageElement && image.complete && image.naturalWidth > 0))).toBe(true);
  }
 }
});
test('logout clears protected content in another tab sharing the session', async ({ page, context }) => {
 const registered = await context.request.post('/api/auth/register', { data: { name: 'Две вкладки', email: `tabs-${Date.now()}@example.test`, password, password_confirmation: password } }); expect(registered.status()).toBe(201); await context.request.post('/api/auth/onboarding', { data: {} });
 await page.goto('/tasks'); await expect(page.locator('.profile-name')).toHaveText('Две вкладки'); const other = await context.newPage(); await other.goto('/settings'); await other.getByRole('button', { name: 'Выйти', exact: true }).click();
 await expect(page).toHaveURL(/\/auth$/); await expect(page.locator('.app-shell')).toHaveCount(0); await other.close();
});
test('approved auth experience, registration, onboarding, session refresh, account and logout', async ({ page, context }) => {
 await page.setViewportSize({ width: 1536, height: 1024 }); await page.goto('/tasks'); await expect(page).toHaveURL(/\/auth$/);
 await expect(page.getByRole('heading', { name: 'Добро пожаловать в Ритм' })).toBeVisible(); await expect(page.getByRole('button', { name: 'Войти через Яндекс' })).toBeVisible(); await expect(page.getByRole('button', { name: 'Войти через VK ID' })).toBeVisible(); await noOverflow(page);
 await page.getByRole('tab', { name: 'Создать аккаунт' }).click(); await page.getByLabel('Имя', { exact: true }).fill('Мария Ритм'); await page.getByLabel('Почта', { exact: true }).fill(`ui-${Date.now()}@example.test`); await page.getByLabel('Пароль', { exact: true }).fill(password); await page.getByLabel('Повторите пароль').fill('mismatch password');
 await page.getByRole('button', { name: 'Создать аккаунт', exact: true }).click(); await expect(page.getByRole('alert')).toHaveText('Пароли не совпадают');
 await page.getByRole('button', { name: 'Показать пароль' }).click(); await expect(page.getByLabel('Пароль', { exact: true })).toHaveAttribute('type', 'text'); await page.getByRole('button', { name: 'Скрыть пароль' }).click();
 await page.getByLabel('Повторите пароль').fill(password); await page.getByRole('button', { name: 'Создать аккаунт', exact: true }).click(); await expect(page.getByRole('heading', { name: 'Добро пожаловать в Ритм' })).toBeVisible();
 await page.getByRole('button', { name: 'Начать', exact: true }).click(); await expect(page.getByRole('heading', { name: 'Подключить Яндекс Календарь?' })).toBeVisible();
 const directory = resolve('../docs/screenshots/p1-2'); await mkdir(directory, { recursive: true }); await settle(page); await page.screenshot({ path: resolve(directory, 'onboarding.png'), animations: 'disabled' });
 expect((await (await context.request.get('/api/integrations')).json()).connections).toEqual([]); await page.getByRole('button', { name: 'Сделать позже' }).click(); await page.getByRole('button', { name: 'Перейти в Ритм' }).click(); await expect(page.locator('h1')).toHaveText('Чем помочь?');
 await page.reload(); await expect(page.locator('.profile-name')).toHaveText('Мария Ритм');
 const cookies = await context.cookies(); expect(cookies.find(cookie => cookie.name === 'rhythm_session')?.httpOnly).toBe(true); expect(await page.evaluate(() => document.cookie)).not.toContain('rhythm_session');
 await page.getByRole('link', { name: 'Настройки', exact: true }).click(); await expect(page.getByRole('heading', { level: 1, name: 'Настройки' })).toBeVisible(); await expect(page.locator('.account-card')).toContainText('Мария Ритм'); await settle(page); await page.screenshot({ path: resolve(directory, 'account.png'), animations: 'disabled' });
 await page.getByRole('button', { name: 'Выйти', exact: true }).click(); await expect(page).toHaveURL(/\/auth$/); expect((await context.request.get('/api/events')).status()).toBe(401);
});
test('login errors, pending state, session expiration and OAuth cancellation stay inline', async ({ page }) => {
 await page.goto('/auth?error=oauth_cancelled'); await expect(page.getByRole('alert')).toHaveText('Авторизация была отменена');
 await page.getByLabel('Почта').fill('missing@example.test'); await page.getByLabel('Пароль', { exact: true }).fill('wrong password');
 await page.getByRole('button', { name: 'Войти', exact: true }).click(); await expect(page.getByRole('alert')).toHaveText('Неверная почта или пароль');
 await page.getByRole('button', { name: 'Забыли пароль?' }).click(); await expect(page.getByRole('status')).toContainText('Восстановление пароля');
 await page.route('**/api/auth/login', async route => { await new Promise(resolve => setTimeout(resolve, 200)); await route.fulfill({ status: 401, json: { error: { code: 'invalid_credentials', message: 'Неверная почта или пароль' } } }); });
 await page.getByRole('button', { name: 'Войти', exact: true }).click(); await expect(page.getByRole('button', { name: 'Входим…' })).toBeDisabled(); await expect(page.getByRole('alert')).toBeVisible();
 await page.goto('/api/auth/yandex/start'); await expect(page).toHaveURL(/\/auth\?error=oauth_unavailable/); await expect(page.getByRole('alert')).toContainText('ещё не настроен');
});
test('auth mobile and reduced motion have no overflow or decorative animation', async ({ page }) => {
 await page.emulateMedia({ reducedMotion: 'reduce' }); await page.setViewportSize({ width: 390, height: 844 }); await page.goto('/auth'); await expect(page.locator('.auth-mobile-brand')).toBeVisible(); await expect(page.locator('.auth-information')).toBeHidden(); await noOverflow(page);
 expect(await page.locator('.auth-wave-face').evaluate(element => getComputedStyle(element).animationName)).toBe('none');
 await page.getByRole('tab', { name: 'Создать аккаунт' }).click(); await expect(page.getByLabel('Повторите пароль')).toBeVisible(); await noOverflow(page);
});
test('two browser accounts never share events or preferences', async ({ browser, baseURL }) => {
 const first = await browser.newContext({ baseURL }); const second = await browser.newContext({ baseURL });
 try { const identities = []; for (const [i, context] of [first, second].entries()) { const response = await context.request.post('/api/auth/register', { data: { name: `User ${i}`, email: `tenant-${i}-${Date.now()}@example.test`, password, password_confirmation: password } }); expect(response.status()).toBe(201); identities.push((await response.json()).user.id); }
 const created = await first.request.post('/api/events', { data: { kind: 'task', title: 'Секретная задача' } }); expect(created.status()).toBe(201); const event = await created.json();
 expect((await second.request.get(`/api/events/${event.id}`)).status()).toBe(404); expect((await (await second.request.get('/api/events?q=Секретная')).json()).total).toBe(0);
 expect((await second.request.get(`/api/preferences/${identities[0]}`)).status()).toBe(404);
 } finally { await first.close(); await second.close(); }
});
