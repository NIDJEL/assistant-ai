import { test, expect, type Page } from '@playwright/test';
import { LEGAL_VERSION } from '../src/pages/Legal/documents';

declare global { interface Window { authFrameRequests: number } }
const pageErrors = new WeakMap<Page, string[]>();

// These UI checks isolate the visual scene from the database and provider accounts.
// The existing auth.spec.ts covers real accounts, sessions and onboarding.
test.beforeEach(async ({ page }) => {
  const errors: string[] = []; pageErrors.set(page, errors);
  page.on('pageerror', error => errors.push(error.message));
  await page.route('**/api/auth/me', route => route.fulfill({ status: 401, json: { error: { message: 'Войдите' } } }));
});
test.afterEach(async ({ page }) => { expect(pageErrors.get(page)).toEqual([]); });

async function settle(page: Page) {
  await page.evaluate(async () => {
    await Promise.allSettled(document.getAnimations().filter(a => a.effect?.getComputedTiming().iterations !== Infinity).map(a => a.finished));
  });
}

test('desktop and mobile layouts keep the form, illustrations and consents intact', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  for (const viewport of [{ width: 1920, height: 1080 }, { width: 1440, height: 900 }, { width: 1366, height: 768 }, { width: 1280, height: 720 }, { width: 1024, height: 768 }, { width: 768, height: 1024 }, { width: 593, height: 932 }, { width: 390, height: 844 }, { width: 360, height: 740 }, { width: 320, height: 740 }]) {
    await page.setViewportSize(viewport);
    await page.goto('/auth');
    await expect(page.getByRole('heading', { name: 'Добро пожаловать! Я Гена' })).toBeVisible();
    if (viewport.width > 900) {
      await expect(page.locator('.auth-information')).toBeVisible();
      await expect(page.locator('.auth-feature-card')).toHaveCount(3);
      await expect(page.locator('.auth-pitch h1')).toHaveCSS('opacity', '1');
      await expect(page.locator('.auth-mascot-scene')).toHaveCSS('opacity', '1');
    } else await expect(page.locator('.auth-mobile-brand .gena-mascot')).toBeVisible();
    for (const mode of ['login', 'register']) {
      if (mode === 'register') await page.getByRole('tab', { name: 'Создать аккаунт' }).click();
      await settle(page);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      expect(await page.locator('.auth-card').evaluate(e => e.scrollWidth <= e.clientWidth && e.scrollHeight <= e.clientHeight + 1)).toBe(true);
      expect(await page.locator('.auth-legal').evaluate(e => parseFloat(getComputedStyle(e).fontSize))).toBeGreaterThanOrEqual(13);
      for (const field of await page.locator('.auth-form input:not([type=checkbox])').all()) {
        await field.scrollIntoViewIfNeeded();
        await expect(field).toBeInViewport({ ratio: 1 });
        expect(await field.evaluate(e => e.closest('.auth-input')!.getBoundingClientRect().height)).toBeGreaterThanOrEqual(48);
      }
      const submit = page.getByRole('button', { name: mode === 'login' ? 'Войти' : 'Создать аккаунт', exact: true });
      await submit.scrollIntoViewIfNeeded();
      await expect(submit).toBeInViewport({ ratio: 1 });
      expect(await page.locator('.auth-premium img').evaluateAll(images => images.every(image => image instanceof HTMLImageElement && image.complete && image.naturalWidth > 0))).toBe(true);
    }
  }
});

test('tabs slide, support the keyboard and retain input without moving focus into the scene', async ({ page }) => {
  await page.goto('/auth'); await settle(page);
  await page.getByLabel('Почта', { exact: true }).fill('motion@example.test');
  await page.getByRole('tab', { name: 'Войти', exact: true }).focus();
  await page.keyboard.press('ArrowRight');
  await expect(page.getByRole('tab', { name: 'Создать аккаунт' })).toBeFocused();
  await expect(page.getByLabel('Повторите пароль')).toBeVisible();
  await expect(page.getByRole('tabpanel')).toHaveAttribute('aria-labelledby', 'auth-tab-register');
  await page.keyboard.press('Home');
  await expect(page.getByRole('tab', { name: 'Войти', exact: true })).toBeFocused();
  await expect(page.getByLabel('Почта', { exact: true })).toHaveValue('motion@example.test');
  await expect(page.getByLabel('Повторите пароль')).toHaveCount(0);
});

test('the redesigned form sends the same credentials and consent payload and blocks duplicate submissions', async ({ page }) => {
  let finish!: () => void;
  const pending = new Promise<void>(resolve => { finish = resolve; });
  let attempts = 0;
  let body: unknown;
  await page.route('**/api/auth/login', async route => {
    attempts++; body = route.request().postDataJSON(); await pending;
    await route.fulfill({ status: 401, json: { error: { message: 'Неверная почта или пароль' } } });
  });
  await page.goto('/auth');
  await expect(page.getByRole('button', { name: 'Войти', exact: true })).toBeDisabled();
  await expect(page.getByRole('button', { name: 'Войти через Яндекс' })).toBeDisabled();
  await page.getByLabel('Почта', { exact: true }).fill('anna@example.test');
  await page.getByLabel('Пароль', { exact: true }).fill('test password 12345');
  await page.getByRole('button', { name: 'Показать пароль' }).click();
  await expect(page.getByLabel('Пароль', { exact: true })).toHaveAttribute('type', 'text');
  await page.getByRole('checkbox', { name: /Принимаю/ }).check();
  await page.getByRole('checkbox', { name: /Даю/ }).check();
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Входим…' })).toBeDisabled();
  await page.getByLabel('Пароль', { exact: true }).press('Enter');
  expect(attempts).toBe(1);
  expect(body).toEqual({ email: 'anna@example.test', password: 'test password 12345', terms_accepted: true, personal_data_consent: true, legal_version: LEGAL_VERSION });
  finish();
  await expect(page.getByRole('alert')).toHaveText('Неверная почта или пароль');
  await page.getByRole('button', { name: 'Забыли пароль?' }).click();
  await expect(page.getByRole('status')).toContainText('Восстановление пароля по почте пока недоступно');
});

test('Yandex keeps the real start endpoint and consent payload', async ({ page }) => {
  let body: unknown;
  await page.route('**/api/auth/yandex/start', async route => {
    body = route.request().postDataJSON();
    await route.fulfill({ status: 503, json: { error: { message: 'Вход через Яндекс ещё не настроен' } } });
  });
  await page.goto('/auth');
  await page.getByRole('checkbox', { name: /Принимаю/ }).check();
  await page.getByRole('checkbox', { name: /Даю/ }).check();
  await page.getByRole('button', { name: 'Войти через Яндекс' }).click();
  await expect(page.getByRole('alert')).toContainText('ещё не настроен');
  expect(body).toEqual({ terms_accepted: true, personal_data_consent: true, legal_version: LEGAL_VERSION });
});

test('reduced motion disables decorative animation and pointer movement', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('/auth'); await settle(page);
  await page.mouse.move(200, 200); await page.mouse.move(700, 600);
  expect(await page.locator('.auth-premium').evaluate(e => Number(e.style.getPropertyValue('--pointer-x')))).toBe(0);
  const moving = await page.evaluate(() => document.getAnimations().filter(a => a.playState === 'running').length);
  expect(moving).toBe(0);
  await page.getByRole('tab', { name: 'Создать аккаунт' }).click();
  await expect(page.getByLabel('Повторите пароль')).toBeVisible();
  expect(await page.locator('.auth-segment>span').evaluate(e => getComputedStyle(e).transitionDuration)).toBe('0s');
});

test('pointer spring settles and returns to neutral without a persistent animation frame', async ({ page }) => {
  await page.addInitScript(() => {
    const raf = window.requestAnimationFrame.bind(window);
    const state = window;
    state.authFrameRequests = 0;
    window.requestAnimationFrame = callback => { state.authFrameRequests++; return raf(callback); };
  });
  await page.goto('/auth'); await settle(page);
  const card = page.locator('.auth-feature-reveal').first();
  const bounds = (await card.boundingBox())!;
  await page.mouse.move(bounds.x + bounds.width * .85, bounds.y + bounds.height * .2);
  await expect.poll(() => card.evaluate(e => Number(e.style.getPropertyValue('--pointer-x')))).toBeGreaterThan(.5);
  await page.mouse.move(-5, -5);
  await expect.poll(() => card.evaluate(e => Number(e.style.getPropertyValue('--pointer-x')))).toBe(0);
  await expect.poll(() => page.locator('.auth-premium').evaluate(e => Number(e.style.getPropertyValue('--pointer-x')))).toBe(0);
  expect(await page.evaluate(async () => {
    const state = window;
    // The observer's own frame is the only request once the springs settle.
    const before = state.authFrameRequests;
    await new Promise(requestAnimationFrame);
    return state.authFrameRequests - before;
  })).toBe(1);
});

test('an inactive document pauses the whole scene and clears pointer motion', async ({ page }) => {
  await page.goto('/auth'); await settle(page);
  await page.evaluate(() => {
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => true });
    document.dispatchEvent(new Event('visibilitychange'));
  });
  await expect(page.locator('.auth-premium')).toHaveAttribute('data-paused', 'true');
  expect(await page.locator('.auth-orbit-front').evaluate(e => getComputedStyle(e).animationPlayState)).toBe('paused');
  expect(await page.locator('.auth-premium').evaluate(e => Number(e.style.getPropertyValue('--pointer-x')))).toBe(0);
  await page.evaluate(() => {
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => false });
    document.dispatchEvent(new Event('visibilitychange'));
  });
  await expect(page.locator('.auth-premium')).toHaveAttribute('data-paused', 'false');
  expect(await page.locator('.auth-orbit-front').evaluate(e => getComputedStyle(e).animationPlayState)).toBe('running');
});

test('touch devices retain the mascot without pointer parallax', async ({ browser, baseURL }) => {
  const context = await browser.newContext({ baseURL, viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });
  const page = await context.newPage();
  try {
    await page.route('**/api/auth/me', route => route.fulfill({ status: 401, json: {} }));
    await page.goto('/auth'); await settle(page);
    await expect(page.locator('.auth-mobile-brand .gena-mascot')).toBeVisible();
    await page.getByLabel('Почта', { exact: true }).tap();
    expect(await page.locator('.auth-premium').evaluate(e => Number(e.style.getPropertyValue('--pointer-x')))).toBe(0);
  } finally { await context.close(); }
});
