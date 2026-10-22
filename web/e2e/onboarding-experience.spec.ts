import { test, expect } from '@playwright/test';
import { legalAcceptance } from './legal-acceptance';

test.beforeEach(async ({ context }) => {
  const result = await context.request.post('/api/auth/register', { data: { ...legalAcceptance, name: 'Анна', email: `onboarding-ui-${crypto.randomUUID()}@example.test`, password: 'test password 12345', password_confirmation: 'test password 12345' } });
  expect(result.status()).toBe(201);
});

const titles = [/Добро пожаловать/, /Ваш день/, /Важное — рядом/];
for (const [width, height] of [[1440, 900], [1024, 768], [768, 1024], [390, 844], [320, 740]]) {
  test(`all compositions fit ${width} × ${height} and keep state`, async ({ page }, testInfo) => {
    const errors: string[] = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.setViewportSize({ width, height });
    await page.goto('/onboarding');
    for (let step = 0; step < 3; step++) {
      if (step) await page.getByRole('button', { name: step === 1 ? 'Начать' : 'Далее', exact: true }).click();
      await expect(page.getByRole('heading', { name: titles[step] })).toBeVisible();
      await expect(page.getByRole('navigation', { name: `Шаг ${step + 1} из 3` })).toBeVisible();
      await expect.poll(() => page.locator('.ob-art img').evaluateAll(images => images.every(image => (image as HTMLImageElement).complete && (image as HTMLImageElement).naturalWidth > 0))).toBe(true);
      await page.evaluate(async () => { await Promise.allSettled(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished)); });
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      const primary = page.locator('.ob-primary');
      await expect(primary).toBeInViewport({ ratio: 1 });
      await expect(page.getByRole('heading', { name: titles[step] })).toBeInViewport({ ratio: 1 });
      expect(await page.locator('.ob-copy').evaluate(element => element.scrollWidth <= element.clientWidth + 1)).toBe(true);
      await page.screenshot({ path: testInfo.outputPath(`step-${step + 1}.png`), animations: 'disabled', fullPage: true });
    }
    await page.reload();
    await expect(page.getByRole('heading', { name: titles[2] })).toBeVisible();
    await page.getByRole('button', { name: 'Назад' }).click();
    await expect(page.getByRole('heading', { name: titles[1] })).toBeVisible();
    await page.getByRole('button', { name: 'Шаг 1: Знакомство' }).click();
    await expect(page.getByRole('heading', { name: titles[0] })).toBeVisible();
    expect(errors).toEqual([]);
  });
}

test('reduced motion removes ambient animation and parallax and still navigates', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('/onboarding');
  await expect(page.locator('.gena-onboarding')).toHaveAttribute('data-reduced', 'true');
  await expect(page.getByRole('heading', { name: titles[0] })).toBeVisible();
  await page.mouse.move(1100, 300);
  expect(await page.locator('.ob-scene').evaluate(element => getComputedStyle(element).transform)).toBe('none');
  expect(await page.evaluate(() => document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations === Infinity).length)).toBe(0);
  await page.getByRole('button', { name: 'Начать', exact: true }).click();
  await expect(page.getByRole('heading', { name: titles[1] })).toBeVisible();
  await page.getByRole('button', { name: 'Далее', exact: true }).click();
  await page.getByRole('button', { name: 'Сделать позже' }).click();
  await expect(page.getByRole('heading', { name: 'Чем помочь?' })).toBeVisible();
});

test('keyboard navigation, animated progress and real integration destination', async ({ page }) => {
  await page.goto('/onboarding');
  const first = page.getByRole('button', { name: 'Шаг 1: Знакомство' });
  await first.focus(); await page.keyboard.press('Tab'); await page.keyboard.press('Enter');
  await expect(page.getByRole('heading', { name: titles[1] })).toBeFocused();
  await page.getByRole('button', { name: 'Далее', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Шаг 3: Интеграции' })).toHaveAttribute('aria-current', 'step');
  await page.getByRole('button', { name: 'Все интеграции' }).click();
  await expect(page).toHaveURL(/\/integrations$/);
  await expect(page.getByRole('heading', { name: 'Интеграции', exact: true })).toBeVisible();
});

test('ghost floats, pointer settles, hover responds and a hidden tab pauses the scene', async ({ page }) => {
  await page.goto('/onboarding');
  const ghost = page.locator('.ob-ghost');
  await expect(ghost).toBeVisible();
  const before = await ghost.evaluate(element => getComputedStyle(element).transform);
  await expect.poll(() => ghost.evaluate(element => getComputedStyle(element).transform)).not.toBe(before);
  await page.mouse.move(1050, 330);
  await expect.poll(() => page.locator('.gena-onboarding').evaluate(element => Number(element.style.getPropertyValue('--pointer-x')))).toBeGreaterThan(.1);
  const button = page.locator('.ob-primary');
  await button.hover();
  await expect.poll(() => page.locator('.ob-arrow').evaluate(element => getComputedStyle(element).transform)).not.toBe('none');
  await page.evaluate(() => { Object.defineProperty(document, 'hidden', { configurable: true, value: true }); document.dispatchEvent(new Event('visibilitychange')); });
  expect(await ghost.evaluate(element => getComputedStyle(element).animationPlayState)).toBe('paused');
});

test('touch device keeps scenes inside the screen and does not start parallax', async ({ browser, baseURL }) => {
  const context = await browser.newContext({ baseURL, viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, deviceScaleFactor: 2 });
  try {
    await context.request.post('/api/auth/register', { data: { ...legalAcceptance, name: 'Анна', email: `onboarding-touch-${crypto.randomUUID()}@example.test`, password: 'test password 12345', password_confirmation: 'test password 12345' } });
    const page = await context.newPage();
    await page.goto('/onboarding');
    await page.getByRole('button', { name: 'Начать', exact: true }).tap();
    await expect(page.getByRole('heading', { name: titles[1] })).toBeVisible();
    await page.getByRole('button', { name: 'Далее', exact: true }).tap();
    await expect(page.getByRole('heading', { name: titles[2] })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    expect(await page.locator('.ob-scene').evaluate(element => getComputedStyle(element).transform)).toBe('none');
    await expect(page.locator('.ob-primary')).toBeInViewport({ ratio: 1 });
  } finally { await context.close(); }
});
