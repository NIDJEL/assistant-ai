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
  for (const viewport of [{ width: 1920, height: 918 }, { width: 1920, height: 1080 }, { width: 1440, height: 900 }, { width: 1366, height: 768 }, { width: 1280, height: 720 }, { width: 1024, height: 768 }, { width: 768, height: 1024 }, { width: 593, height: 932 }, { width: 390, height: 844 }, { width: 360, height: 740 }, { width: 320, height: 740 }]) {
    await page.setViewportSize(viewport);
    await page.goto('/auth');
    await expect(page.getByRole('heading', { name: 'Добро пожаловать! Я Гена' })).toBeVisible();
    if (viewport.width > 900) {
      await expect(page.locator('.auth-information')).toBeVisible();
      await expect(page.locator('.auth-feature-card')).toHaveCount(3);
      await expect(page.locator('.auth-pitch h1')).toHaveCSS('opacity', '1');
      await expect(page.locator('.auth-mascot-scene')).toHaveCSS('opacity', '1');
      expect(await page.evaluate(() => {
        const image = document.querySelector('.auth-mascot-scene img')!.getBoundingClientRect();
        // The head occupies this area of the transparent illustration; allow for its float/parallax.
        const head = { left: image.left + image.width * .21 - 12, right: image.left + image.width * .74 + 12, top: image.top + image.height * .04 - 12, bottom: image.top + image.height * .44 };
        return [...document.querySelectorAll('.auth-pitch h1>span,.auth-description')].every(e => {
          const b = e.getBoundingClientRect();
          return b.right <= head.left || b.left >= head.right || b.bottom <= head.top || b.top >= head.bottom;
        });
      })).toBe(true);
    } else {
      await expect(page.locator('.auth-mobile-brand')).toHaveCount(0);
      await expect(page.locator('.gena-mascot')).toBeHidden();
      await expect(page.locator('.auth-brand')).toBeHidden();
    }
    for (const text of ['Ваши планы в надёжных руках', 'Гена. Всё сложится.', 'Конфиденциальность', 'Чуть меньше рутины. Чуть больше жизни.', 'На вашей стороне']) await expect(page.getByText(text, { exact: true })).toHaveCount(0);
    for (const mode of ['login', 'register']) {
      if (mode === 'register') await page.getByRole('tab', { name: 'Создать аккаунт' }).click();
      await settle(page);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      expect(await page.evaluate(() => document.documentElement.scrollHeight <= innerHeight)).toBe(true);
      expect(await page.locator('.auth-card').evaluate(e => e.scrollWidth <= e.clientWidth && e.scrollHeight <= e.clientHeight + 1)).toBe(true);
      expect(await page.locator('.auth-legal').evaluate(e => parseFloat(getComputedStyle(e).fontSize))).toBeGreaterThanOrEqual(13);
      for (const field of await page.locator('.auth-form input:not([type=checkbox])').all()) {
        await field.scrollIntoViewIfNeeded();
        await expect(field).toBeInViewport({ ratio: 1 });
        expect(await field.evaluate(e => e.closest('.auth-input')!.getBoundingClientRect().height)).toBeGreaterThanOrEqual(viewport.width <= 900 && viewport.height <= 760 ? 44 : 48);
      }
      const submit = page.getByRole('button', { name: mode === 'login' ? 'Войти' : 'Создать аккаунт', exact: true });
      await submit.scrollIntoViewIfNeeded();
      await expect(submit).toBeInViewport({ ratio: 1 });
      await page.evaluate(() => window.scrollTo(0, 1000));
      expect(await page.evaluate(() => scrollY)).toBe(0);
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

for (const event of ['pageshow', 'visibilitychange']) test(`returning from Yandex via ${event} restores credentials and consent`, async ({ page }) => {
  let starts = 0;
  let release!: () => void; const pending = new Promise<void>(resolve => { release = resolve; });
  await page.route('**/api/auth/yandex/start', async route => {
    starts++;
    if (starts === 1) await pending;
    return starts === 1
      ? route.fulfill({ json: { redirect_url: new URL('/oauth-cancel-test', page.url()).href } })
      : route.fulfill({ status: 503, json: { error: { message: 'Повторный вход доступен' } } });
  });
  await page.route('**/oauth-cancel-test', route => route.abort('aborted'));
  await page.goto('/auth');
  await page.getByLabel('Почта', { exact: true }).fill('return@example.test');
  await page.getByRole('checkbox', { name: /Принимаю/ }).check();
  await page.getByRole('checkbox', { name: /Даю/ }).check();
  const cancelledNavigation = page.waitForEvent('requestfailed', request => request.url().endsWith('/oauth-cancel-test'));
  await page.getByRole('button', { name: 'Войти через Яндекс' }).click();
  await page.evaluate(() => window.dispatchEvent(new PageTransitionEvent('pageshow', { persisted: true })));
  await expect(page.getByRole('button', { name: 'Переходим в Яндекс…' })).toBeDisabled();
  await expect(page.getByRole('tab', { name: 'Создать аккаунт' })).toBeDisabled();
  release();
  await cancelledNavigation;
  await expect(page.getByRole('button', { name: 'Переходим в Яндекс…' })).toBeDisabled();
  // A cached document gets pageshow rather than a fresh React mount.
  await page.evaluate(event => {
    if (event === 'pageshow') window.dispatchEvent(new PageTransitionEvent('pageshow', { persisted: true }));
    else {
      Object.defineProperty(document, 'hidden', { configurable: true, get: () => true }); document.dispatchEvent(new Event('visibilitychange'));
      Object.defineProperty(document, 'hidden', { configurable: true, get: () => false }); document.dispatchEvent(new Event('visibilitychange'));
    }
  }, event);
  await expect(page.getByRole('button', { name: 'Войти через Яндекс' })).toBeEnabled();
  await expect(page.getByRole('tab', { name: 'Создать аккаунт' })).toBeEnabled();
  await expect(page.getByLabel('Почта', { exact: true })).toHaveValue('return@example.test');
  await expect(page.getByRole('checkbox', { name: /Принимаю/ })).toBeChecked();
  await expect(page.getByRole('checkbox', { name: /Даю/ })).toBeChecked();
  await page.getByRole('button', { name: 'Войти через Яндекс' }).click();
  await expect(page.getByRole('alert')).toHaveText('Повторный вход доступен');
  expect(starts).toBe(2);
});

test('installed app metadata and safe areas keep mobile auth controls clear of system indicators', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.setViewportSize({ width: 390, height: 844 }); await page.goto('/auth');
  await expect(page.locator('meta[name=viewport]')).toHaveAttribute('content', /viewport-fit=cover/);
  await expect(page.locator('meta[name=apple-mobile-web-app-status-bar-style]')).toHaveAttribute('content', 'black-translucent');
  const manifest = await (await page.request.get('/manifest.webmanifest')).json();
  expect(manifest).toMatchObject({ display: 'standalone', theme_color: '#080f21', background_color: '#080f21', scope: '/' });
  await page.evaluate(() => { document.documentElement.style.setProperty('--safe-top', '47px'); document.documentElement.style.setProperty('--safe-bottom', '34px'); });
  for (const name of ['Создать аккаунт', 'Войти']) {
    await page.getByRole('tab', { name, exact: true }).click(); await settle(page);
    const card = (await page.locator('.auth-card').boundingBox())!;
    expect(card.y).toBeGreaterThanOrEqual(47); expect(card.y + card.height).toBeLessThanOrEqual(810);
    expect(await page.evaluate(() => document.documentElement.scrollHeight <= innerHeight)).toBe(true);
  }
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

test('touch devices show only the form without mobile branding or pointer parallax', async ({ browser, baseURL }) => {
  const context = await browser.newContext({ baseURL, viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });
  const page = await context.newPage();
  try {
    await page.route('**/api/auth/me', route => route.fulfill({ status: 401, json: {} }));
    await page.goto('/auth'); await settle(page);
    await expect(page.locator('.auth-mobile-brand')).toHaveCount(0);
    await expect(page.locator('.gena-mascot')).toBeHidden();
    await expect(page.locator('.auth-brand')).toBeHidden();
    expect(await page.locator('.auth-form').evaluate(e => parseFloat(getComputedStyle(e).rowGap))).toBeGreaterThanOrEqual(12);
    await page.getByLabel('Почта', { exact: true }).tap();
    expect(await page.locator('.auth-premium').evaluate(e => Number(e.style.getPropertyValue('--pointer-x')))).toBe(0);
  } finally { await context.close(); }
});
