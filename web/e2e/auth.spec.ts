import { test, expect, type Page } from '@playwright/test';
import { noOverflow } from './helpers';
import { mkdir } from 'node:fs/promises';
import { resolve } from 'node:path';
const password = 'test password 12345';
async function settle(page: Page) { await page.evaluate(async () => { await Promise.allSettled(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished)); }); }
test('onboarding mascot keeps its proportions on all steps and pixel densities', async ({ browser, baseURL }) => {
 const directory = resolve('../docs/screenshots/p1-2'); await mkdir(directory, { recursive: true });
 for (const display of [{ width: 913, height: 809, density: 1 }, { width: 913, height: 809, density: 2 }, { width: 390, height: 844, density: 2 }, { width: 360, height: 740, density: 2 }]) {
  const context = await browser.newContext({ baseURL, viewport: { width: display.width, height: display.height }, deviceScaleFactor: display.density, reducedMotion: 'reduce', locale: 'ru-RU', serviceWorkers:'block' });
  try {
   const page = await context.newPage();
   await page.route('**/api/integrations',route=>route.fulfill({json:{connections:[],yandex_configured:true}}));
   await page.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: '11111111-1111-4111-8111-111111111111', name: 'Мария', email: 'onboarding@example.test', avatar_url: '', providers: [], onboarding_completed: false } } }));
   await page.goto('/onboarding');
   for (let step = 0; step < 2; step++) {
    await expect(page.locator('.onboarding-dots')).toHaveAttribute('aria-label', `Шаг ${step + 1} из 2`); await settle(page);
    const scale = await page.locator('.onboarding-card .auth-wave-face').evaluate(element => {
     const matrix = (element as SVGGraphicsElement).getScreenCTM()!;
     return { x: Math.hypot(matrix.a, matrix.b), y: Math.hypot(matrix.c, matrix.d) };
    });
    expect(scale.x).toBeGreaterThan(0); expect(Math.abs(scale.x - scale.y)).toBeLessThan(.001);
    await expect(page.locator('.auth-wave-face')).toBeInViewport({ ratio: .999 });
    await expect(page.locator('.onboarding-step .auth-primary')).toBeInViewport({ ratio: 1 }); await noOverflow(page);
    expect(await page.evaluate(() => document.documentElement.scrollHeight <= innerHeight)).toBe(true);
    if (step > 0 && display.width === 913) await page.screenshot({ path: resolve(directory, `onboarding-${step === 1 ? 'calendar' : 'ready'}${display.density === 2 ? '@2x' : ''}.png`), animations: 'disabled' });
    if (step === 1 && display.width === 390) await page.screenshot({ path: resolve(directory, 'onboarding-mobile@2x.png'), animations: 'disabled' });
    if (step === 0) await page.getByRole('button', { name: 'Начать', exact: true }).click();
   }
  } finally { await context.close(); }
 }
});
test('auth tabs visibly slide in both directions and preserve entered values', async ({ page }) => {
 for (const reducedMotion of ['no-preference', 'reduce'] as const) {
  await page.emulateMedia({ reducedMotion }); await page.goto('/auth'); await settle(page);
  await page.getByLabel('Почта', { exact: true }).fill('motion@example.test');
  for (const name of ['Создать аккаунт', 'Войти']) {
   const movement = await page.getByRole('tab', { name, exact: true }).evaluate(async button => {
    const slider = button.parentElement!.querySelector('span')!; const from = slider.getBoundingClientRect().left; const positions: number[] = [];
    (button as HTMLElement).click();
    for (let frame = 0; frame < 40; frame++) { await new Promise(resolve => requestAnimationFrame(resolve)); positions.push(slider.getBoundingClientRect().left); }
    return { from, to: positions.at(-1)!, positions };
   });
   expect(Math.abs(movement.to - movement.from)).toBeGreaterThan(50);
   expect(movement.positions.some(position => position > Math.min(movement.from, movement.to) + 2 && position < Math.max(movement.from, movement.to) - 2)).toBe(true);
   await expect(page.getByRole('tab', { name, exact: true })).toHaveAttribute('aria-selected', 'true');
   await expect(page.getByLabel('Почта', { exact: true })).toHaveValue('motion@example.test');
  }
 }
});
test('registration uses persistent labels inside spacious fields', async ({ page }) => {
 await page.setViewportSize({ width: 1366, height: 768 }); await page.goto('/auth'); await page.getByRole('tab', { name: 'Создать аккаунт' }).click(); await settle(page);
 await expect(page.getByText('Ваши дела — в одном спокойном ритме')).toHaveCount(0);
 for (const label of ['Имя', 'Почта', 'Пароль', 'Повторите пароль']) {
  const field = page.getByLabel(label, { exact: true });
  expect(await field.evaluate(input => {
   const control = input.closest('.auth-input')!.getBoundingClientRect(); const label = input.parentElement!.querySelector('.auth-input-label')!.getBoundingClientRect();
   return control.height >= 48 && label.top >= control.top && label.bottom <= control.bottom;
  })).toBe(true);
 }
 await page.getByLabel('Имя', { exact: true }).fill('Анна'); await page.getByLabel('Почта', { exact: true }).focus();
 await expect.poll(() => page.getByLabel('Имя', { exact: true }).evaluate(input => input.parentElement!.querySelector('.auth-input-label')!.getBoundingClientRect().bottom < input.getBoundingClientRect().top + input.getBoundingClientRect().height / 2)).toBe(true);
});
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
 await expect(page).toHaveTitle('Ассистент Гена'); await expect(page.getByRole('heading', { name: 'Добро пожаловать! Я Гена' })).toBeVisible(); await expect(page.getByRole('button', { name: 'Войти через Яндекс' })).toBeVisible(); await expect(page.getByRole('button', { name: /VK/ })).toHaveCount(0); await noOverflow(page);
 await page.getByRole('tab', { name: 'Создать аккаунт' }).click(); await page.getByLabel('Имя', { exact: true }).fill('Мария Гена'); await page.getByLabel('Почта', { exact: true }).fill(`ui-${Date.now()}@example.test`); await page.getByLabel('Пароль', { exact: true }).fill(password); await page.getByLabel('Повторите пароль').fill('mismatch password');
 await page.getByRole('button', { name: 'Создать аккаунт', exact: true }).click(); await expect(page.getByRole('alert')).toHaveText('Пароли не совпадают');
 await page.getByRole('button', { name: 'Показать пароль' }).click(); await expect(page.getByLabel('Пароль', { exact: true })).toHaveAttribute('type', 'text'); await page.getByRole('button', { name: 'Скрыть пароль' }).click();
 await page.getByLabel('Повторите пароль').fill(password); await page.getByRole('button', { name: 'Создать аккаунт', exact: true }).click(); await expect(page.getByRole('heading', { name: 'Добро пожаловать! Я Гена' })).toBeVisible();
 await page.getByRole('button', { name: 'Начать', exact: true }).click(); await expect(page.getByRole('heading', { name: 'Подключить Яндекс Календарь?' })).toBeVisible();
 const directory = resolve('../docs/screenshots/p1-2'); await mkdir(directory, { recursive: true }); await settle(page); await page.screenshot({ path: resolve(directory, 'onboarding.png'), animations: 'disabled' });
 expect((await (await context.request.get('/api/integrations')).json()).connections).toEqual([]); await page.getByRole('button', { name: 'Сделать позже' }).click(); await expect(page.locator('h1')).toHaveText('Чем помочь?');
 await page.reload(); await expect(page.locator('.profile-name')).toHaveText('Мария Гена');
 const cookies = await context.cookies(); expect(cookies.find(cookie => cookie.name === 'rhythm_session')?.httpOnly).toBe(true); expect(await page.evaluate(() => document.cookie)).not.toContain('rhythm_session');
 await page.getByRole('link', { name: 'Настройки', exact: true }).click(); await expect(page.getByRole('heading', { level: 1, name: 'Настройки' })).toBeVisible(); await expect(page.locator('.account-card')).toContainText('Мария Гена'); await settle(page); await page.screenshot({ path: resolve(directory, 'account.png'), animations: 'disabled' });
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
