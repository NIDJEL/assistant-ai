import { test, expect, type Page } from '@playwright/test';
import { legalAcceptance } from './legal-acceptance';
import { themeContrastIssues } from './theme-contrast';

const password = 'test password 12345';
async function account(page: Page) {
  const email = `light-artwork-${crypto.randomUUID()}@example.test`;
  expect((await page.request.post('/api/auth/register', { data: { ...legalAcceptance, name: 'Анна', email, password, password_confirmation: password } })).status()).toBe(201);
}
async function settle(page: Page) {
  await page.evaluate(async () => { await Promise.allSettled(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished)); });
}
for (const [width, height] of [[1440, 900], [768, 1024], [390, 844], [320, 740]]) {
  test(`light artwork fits all three steps at ${width}px`, async ({ page }, info) => {
    await account(page); await page.setViewportSize({ width, height }); await page.goto('/onboarding');
    await page.getByRole('radio', { name: 'Светлая', exact: true }).check();
    await expect(page.locator('.ob-scene')).toHaveAttribute('data-artwork-theme', 'light');
    await expect(page.locator('.ob-brand-light')).toBeVisible();
    for (let step = 0; step < 3; step++) {
      if (step) await page.getByRole('button', { name: step === 1 ? 'Начать' : 'Далее', exact: true }).click();
      await expect(page.locator('h1')).toBeVisible(); await settle(page);
      await expect(page.locator('.ob-art-light')).toHaveCount(1); await expect(page.locator('.ob-art-dark')).toHaveCount(0);
      const images = await page.locator('.ob-art img').evaluateAll(elements => elements.map(element => { const image = element as HTMLImageElement; return { src: image.src, width: image.naturalWidth, loaded: image.complete, bounds: image.getBoundingClientRect().toJSON() }; }));
      expect(images.every(image => image.loaded && image.width > 0)).toBe(true);
      // Each supplied light image has its own intrinsic dimensions; this also
      // catches an accidental return to the original dark illustrations.
      expect(images.map(image => image.width)).toEqual(step === 0 ? [380, 530] : step === 1 ? [794] : [215, 197, 224, 739]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
      await expect(page.locator('.ob-primary')).toBeInViewport({ ratio: 1 });
      expect(await themeContrastIssues(page)).toEqual([]);
      await page.screenshot({ path: info.outputPath(`light-step-${step + 1}-${width}.png`), fullPage: true, animations: 'disabled' });
    }
    await page.reload(); await expect(page.locator('.gena-onboarding')).toHaveAttribute('data-step', '2');
    await expect(page.locator('.ob-scene')).toHaveAttribute('data-artwork-theme', 'light');
  });
}
test('theme waits for artwork, crossfades and survives rapid changes with reduced motion', async ({ page }) => {
  await account(page); await page.goto('/onboarding'); await settle(page);
  const original = await page.locator('.ob-ghost img').getAttribute('src');
  let release!: () => void; const pending = new Promise<void>(resolve => { release = resolve; }); let delayed = false;
  await page.route('**/assets/ghost-*.webp', async route => { if (!route.request().url().endsWith(original!)) { delayed = true; await pending; } await route.continue(); });
  await page.getByRole('radio', { name: 'Светлая', exact: true }).check();
  await expect.poll(() => delayed).toBe(true); await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  await expect(page.locator('.ob-scene')).toHaveAttribute('data-artwork-theme', 'dark');
  await expect(page.locator('.ob-ghost img')).toHaveAttribute('src', original!);
  release(); await expect(page.locator('.ob-scene')).toHaveAttribute('data-artwork-theme', 'light');
  const samples = await page.evaluate(async () => {
    const values: number[] = [];
    for (let i = 0; i < 10; i++) { await new Promise(requestAnimationFrame); const art = document.querySelector('.ob-art-light'); if (art) values.push(Number(getComputedStyle(art).opacity)); }
    return values;
  });
  expect(samples.some(value => value > 0 && value < 1)).toBe(true); await settle(page);
  await page.getByRole('radio', { name: 'Тёмная', exact: true }).check(); await page.getByRole('radio', { name: 'Светлая', exact: true }).check();
  await expect(page.locator('.ob-scene')).toHaveAttribute('data-artwork-theme', 'light'); await settle(page); await expect(page.locator('.ob-art')).toHaveCount(1);
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.getByRole('radio', { name: 'Тёмная', exact: true }).check(); await expect(page.locator('.ob-scene')).toHaveAttribute('data-artwork-theme', 'dark');
  await page.getByRole('radio', { name: 'Светлая', exact: true }).check(); await expect(page.locator('.ob-scene')).toHaveAttribute('data-artwork-theme', 'light');
  await expect(page.locator('.ob-art')).toHaveCount(1);
  expect(await page.evaluate(() => document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations === Infinity).length)).toBe(0);
});
for (const theme of ['dark', 'light']) for (const width of [1440, 390]) {
  test(`missing consent is visibly red and never starts OAuth: ${theme} ${width}`, async ({ page }, info) => {
    await page.addInitScript(value => localStorage.setItem('gena.theme.v1', value), theme);
    await page.setViewportSize({ width, height: 900 }); await page.goto('/auth');
    let attempts = 0; await page.route('**/api/auth/yandex/start', route => { attempts++; return route.fulfill({ status: 503, json: { error: { message: 'Тестовый провайдер недоступен' } } }); });
    const terms = page.locator('#auth-terms'), consent = page.locator('#auth-data-consent');
    const before = await terms.evaluate(element => getComputedStyle(element).borderColor);
    await page.getByRole('button', { name: 'Войти через Яндекс' }).click();
    await expect(terms).toBeFocused(); await expect(terms).toHaveAttribute('aria-invalid', 'true'); await expect(consent).toHaveAttribute('aria-invalid', 'true');
    await settle(page); expect(await terms.evaluate(element => getComputedStyle(element).borderColor)).not.toBe(before);
    expect(attempts).toBe(0); await expect(page.locator('.auth-feature-symbol')).toHaveCount(0); await expect(page.locator('.auth-feature-art img')).toHaveCount(3);
    expect(await themeContrastIssues(page)).toEqual([]);
    await page.screenshot({ path: info.outputPath(`consent-${theme}-${width}.png`), fullPage: true, animations: 'disabled' });
    await terms.check(); await page.getByRole('button', { name: 'Войти через Яндекс' }).click(); await expect(consent).toBeFocused(); expect(attempts).toBe(0);
    await consent.check(); await expect(page.locator('#auth-consent-error')).toHaveCount(0);
    await page.getByRole('button', { name: 'Войти через Яндекс' }).click(); await expect.poll(() => attempts).toBe(1);
    await expect(page.getByRole('alert')).toContainText('Тестовый провайдер недоступен');
  });
}
test('name validation protects registration and profile in the UI and direct API', async ({ page }) => {
  const email = `validated-${crypto.randomUUID()}@example.test`;
  await page.goto('/auth'); await page.getByRole('tab', { name: 'Создать аккаунт' }).click();
  await page.getByLabel('Имя', { exact: true }).fill('Анна123'); await page.getByLabel('Почта', { exact: true }).fill(email);
  await page.getByLabel('Пароль', { exact: true }).fill(password); await page.getByLabel('Повторите пароль').fill(password);
  await page.locator('#auth-terms').check(); await page.locator('#auth-data-consent').check();
  let registrations = 0; page.on('request', request => { if (request.url().endsWith('/api/auth/register')) registrations++; });
  await page.getByRole('button', { name: 'Создать аккаунт', exact: true }).click(); await expect(page.locator('#auth-name')).toBeFocused();
  await expect(page.locator('#auth-name-error')).toContainText('только буквы'); expect(registrations).toBe(0);
  for (const name of ['Анна123', 'Анна١', '<script>alert</script>', 'Анна\u202e', 'Анна\nМария']) {
    expect((await page.request.post('/api/auth/register', { data: { ...legalAcceptance, name, email, password, password_confirmation: password } })).status()).toBe(422);
  }
  await page.getByLabel('Имя', { exact: true }).fill('Анна-Мария'); await page.getByRole('button', { name: 'Создать аккаунт', exact: true }).click();
  await expect(page).toHaveURL(/\/onboarding$/); await page.getByRole('button', { name: 'Пропустить знакомство' }).click();
  await page.goto('/settings'); await page.getByLabel('Ваше имя', { exact: true }).fill('Анна123'); await page.getByRole('button', { name: 'Сохранить имя' }).click();
  await expect(page.locator('#account-name-error')).toContainText('только буквы');
  expect((await page.request.patch('/api/auth/me', { data: { name: 'Анна\0' } })).status()).toBe(422);
  expect((await (await page.request.get('/api/auth/me')).json()).user.name).toBe('Анна-Мария');
  await page.getByLabel('Ваше имя', { exact: true }).fill('O’Connor'); await page.getByRole('button', { name: 'Сохранить имя' }).click();
  await expect(page.locator('.account-card').getByRole('status')).toContainText('Имя сохранено'); await page.reload(); await expect(page.getByLabel('Ваше имя', { exact: true })).toHaveValue('O’Connor');
});
