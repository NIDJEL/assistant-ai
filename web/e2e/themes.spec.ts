import { test, expect } from '@playwright/test';
import { mockSchedule } from './mock-workspace';
import { legalAcceptance } from './legal-acceptance';
import { themeContrastIssues } from './theme-contrast';

const removed = ['Меньше забот. Больше жизни.', 'Ваш день становится чуть легче', 'Порядок начинается с малого', 'Всё сложится', 'Всё работает вместе', 'Знакомство с ассистентом'];
test('onboarding theme saves to the profile, survives completion and returns from the server', async ({ page, context }) => {
  expect((await context.request.post('/api/auth/register', { data: { ...legalAcceptance, name: 'Анна', email: `theme-${crypto.randomUUID()}@example.test`, password: 'test password 12345', password_confirmation: 'test password 12345' } })).status()).toBe(201);
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('/onboarding'); await page.getByRole('radio', { name: 'Светлая', exact: true }).check();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  await expect(page.locator('meta[name="theme-color"]')).toHaveAttribute('content', '#f3f7fc');
  await expect(page.locator('meta[name="apple-mobile-web-app-status-bar-style"]')).toHaveAttribute('content', 'default');
  const user = (await (await context.request.get('/api/auth/me')).json()).user;
  await expect.poll(async () => (await (await context.request.get(`/api/preferences/${user.id}`)).json()).theme).toBe('light');
  for (let step = 0; step < 3; step++) {
    if (step) await page.getByRole('button', { name: step === 1 ? 'Начать' : 'Далее', exact: true }).click();
    await expect(page.locator('.gena-onboarding')).toHaveAttribute('data-step', String(step));
    for (const text of removed) await expect(page.getByText(text, { exact: true })).toHaveCount(0);
    await expect(page.locator('.ob-counter')).toHaveCount(0);
  }
  await page.getByRole('button', { name: 'Сделать позже' }).click();
  await expect(page.getByRole('heading', { name: 'Чем помочь?' })).toBeVisible();
  await page.evaluate(() => localStorage.clear()); await page.reload();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
  await page.goto('/settings'); await page.getByRole('radio', { name: 'Тёмная', exact: true }).check();
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  await expect.poll(async () => (await (await context.request.get(`/api/preferences/${user.id}`)).json()).theme).toBe('dark');
});

for (const theme of ['light', 'dark'] as const) {
  for (const width of [1440, 390]) {
    test(`${theme} workspace is readable across every page at ${width}px`, async ({ page }, info) => {
      test.setTimeout(90000);
      const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
      await mockSchedule(page); await page.emulateMedia({ reducedMotion: 'reduce' });
      await page.setViewportSize({ width, height: width === 1440 ? 1000 : 844 });
      await page.goto('/settings'); await page.getByRole('radio', { name: theme === 'light' ? 'Светлая' : 'Тёмная', exact: true }).check();
      const problems: { page: string; issues: Awaited<ReturnType<typeof themeContrastIssues>> }[] = [];
      for (const path of ['/', '/tasks', '/calendar', '/meetings', '/reminders', '/archive', '/integrations', '/friends', '/settings']) {
        await page.goto(path); await expect(page.locator('.main-panel > .page')).toBeVisible();
        await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
        await expect(page.locator('.main-panel h1')).toBeVisible();
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
        const issues = await themeContrastIssues(page); if (issues.length) problems.push({ page: path, issues });
        await page.screenshot({ path: info.outputPath(`${theme}-${path.slice(1) || 'chat'}-${width}.png`), animations: 'disabled' });
      }
      expect(errors).toEqual([]); expect(problems).toEqual([]);
    });
  }
}

test('light dialogs, dropdowns, inputs and date picker keep a readable surface', async ({ page }, info) => {
  await mockSchedule(page); await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('/settings'); await page.getByRole('radio', { name: 'Светлая', exact: true }).check();
  await page.goto('/archive'); await page.getByRole('combobox', { name: 'Тип в архиве' }).click();
  const list = page.getByRole('listbox'); await expect(list).toBeVisible();
  expect(await list.evaluate(node => getComputedStyle(node).backgroundColor)).toBe('rgb(255, 255, 255)');
  await page.screenshot({ path: info.outputPath('light-dropdown.png') }); await page.keyboard.press('Escape');
  await page.goto('/tasks'); await page.getByRole('button', { name: 'Новая задача', exact: true }).click();
  await expect(page.getByRole('dialog')).toBeVisible();
  expect(await page.getByRole('dialog').evaluate(node => getComputedStyle(node).backgroundColor)).toBe('rgb(255, 255, 255)');
  await page.screenshot({ path: info.outputPath('light-task-dialog.png') });
  expect(await themeContrastIssues(page)).toEqual([]);
  await page.locator('.dialog[open]').getByRole('button', { name: 'Выбрать дату', exact: true }).click();
  const picker = page.getByRole('dialog', { name: 'Выбор даты' }); await expect(picker).toBeVisible();
  expect(await picker.evaluate(node => getComputedStyle(node).backgroundColor)).toBe('rgb(255, 255, 255)');
  await page.screenshot({ path: info.outputPath('light-date-picker.png') });
  expect(await themeContrastIssues(page)).toEqual([]);
});

for (const width of [1440, 390, 320]) {
  test(`onboarding compositions and theme cards fit both themes at ${width}px`, async ({ page, context }, info) => {
    await context.request.post('/api/auth/register', { data: { ...legalAcceptance, name: 'Анна', email: `theme-layout-${crypto.randomUUID()}@example.test`, password: 'test password 12345', password_confirmation: 'test password 12345' } });
    await page.emulateMedia({ reducedMotion: 'reduce' }); await page.setViewportSize({ width, height: width === 320 ? 740 : width === 390 ? 844 : 900 });
    await page.goto('/onboarding');
    for (const theme of ['dark', 'light']) {
      await page.getByRole('button', { name: 'Шаг 1: Знакомство' }).click();
      await page.getByRole('radio', { name: theme === 'dark' ? 'Тёмная' : 'Светлая', exact: true }).check();
      for (let step = 0; step < 3; step++) {
        if (step) await page.getByRole('button', { name: step === 1 ? 'Начать' : 'Далее', exact: true }).click();
        await expect(page.locator('.gena-onboarding')).toHaveAttribute('data-step', String(step));
        expect(await page.locator('html').evaluate(node => getComputedStyle(node).colorScheme)).toBe(theme);
        await expect(page.getByRole('heading', { name: [/Добро пожаловать/, /Ваш день/, /Важное — рядом/][step] })).toBeVisible();
        await expect.poll(() => page.locator('.ob-copy > *').evaluateAll(nodes => nodes.every(node => getComputedStyle(node).opacity === '1'))).toBe(true);
        await expect.poll(() => page.locator('.ob-art img').evaluateAll(images => images.every(image => (image as HTMLImageElement).complete))).toBe(true);
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
        if (step) expect(await page.locator('.ob-back svg').evaluate(node => getComputedStyle(node).display)).toBe('block');
        expect(await page.locator('.ob-progress-label').first().evaluate(node => parseFloat(getComputedStyle(node).fontSize))).toBeGreaterThanOrEqual(11);
        await page.locator('.ob-primary').scrollIntoViewIfNeeded(); await expect(page.locator('.ob-primary')).toBeInViewport({ ratio: 1 });
        await page.screenshot({ path: info.outputPath(`onboarding-${theme}-${step + 1}-${width}.png`), animations: 'disabled', fullPage: true });
      }
    }
  });
}

test('light login, registration and legal documents use readable colors', async ({ page }, info) => {
  await page.addInitScript(() => localStorage.setItem('gena.theme.v1', 'light'));
  await page.emulateMedia({ reducedMotion: 'reduce' });
  for (const path of ['/auth', '/terms', '/privacy', '/personal-data-consent']) {
    await page.goto(path); await expect(page.locator('h1,h2').first()).toBeVisible();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
    const issues = await themeContrastIssues(page);
    expect(issues, path).toEqual([]);
    await page.screenshot({ path: info.outputPath(`light-${path.slice(1)}.png`), animations: 'disabled' });
  }
});
