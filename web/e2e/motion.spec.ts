import { test, expect } from '@playwright/test';
import { ready, noOverflow } from './helpers';
import { dayKey } from '../src/features/calendar/dates';
import type { Health, RhythmEvent } from '../src/types/api';

test('animated navigation captures the destination and rapid navigation settles on the final route', async ({ page }) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  await ready(page);
  const widths = await page.locator('.main-panel').evaluate(node => node.clientWidth);
  await page.evaluate(() => {
    const original = document.startViewTransition.bind(document);
    const snapshots: string[] = []; const started: string[] = []; Object.assign(window, { motionSnapshots: snapshots, motionStarted: started });
    document.startViewTransition = update => {
      const transition = original(async () => {
        if (typeof update === 'function') await update(); else await update?.update?.();
        snapshots.push(document.querySelector('h1')?.textContent || '');
      });
      void transition.ready.then(() => started.push(document.querySelector('h1')?.textContent || '')).catch(() => {});
      return transition;
    };
  });
  await page.getByRole('link', { name: 'Календарь', exact: true }).click();
  await expect(page.locator('h1')).toHaveText('Календарь');
  await expect.poll(() => page.evaluate(() => (window as unknown as { motionSnapshots: string[] }).motionSnapshots)).toEqual(['Календарь']);
  await expect.poll(() => page.evaluate(() => (window as unknown as { motionStarted: string[] }).motionStarted)).toEqual(['Календарь']);
  await page.evaluate(() => {
    for (const name of ['Встречи', 'Напоминания', 'Задачи']) (document.querySelector(`nav a[aria-label="${name}"]`) as HTMLElement).click();
  });
  await expect(page.locator('h1')).toHaveText('Задачи');
  await expect.poll(() => page.evaluate(() => (window as unknown as { motionSnapshots: string[] }).motionSnapshots.at(-1))).toBe('Задачи');
  await page.getByRole('button', { name: 'Скрыть виджеты' }).click();
  await expect(page.locator('.widgets-panel')).toHaveCount(0);
  await expect.poll(() => page.locator('.main-panel').evaluate(node => node.clientWidth)).toBeGreaterThan(widths);
  await page.getByRole('button', { name: 'Показать виджеты' }).click();
  await expect(page.locator('.widgets-panel')).toBeVisible();
  await page.evaluate(async () => { await Promise.allSettled(document.getAnimations().map(animation => animation.finished)); });
  await expect(page.locator('.main-panel')).toHaveJSProperty('clientWidth', widths);
  await noOverflow(page); expect(errors).toEqual([]);
});

test('reminder close can be reversed without losing the draft, and an exiting dialog cannot submit', async ({ page, request }) => {
  const health = await (await request.get('/health')).json() as Health;
  const response = await request.post('/api/events', { data: { kind: 'reminder', title: `Плавный переход ${Date.now()}`, date: dayKey(health.now, health.timezone), time: '09:00' } });
  expect(response.status()).toBe(201); const event = await response.json() as RhythmEvent;
  try {
    await ready(page, '/reminders'); await page.getByRole('textbox', { name: 'Поиск напоминаний' }).fill(event.title);
    const title = page.getByRole('main').getByRole('button', { name: event.title, exact: true }); await title.click();
    await page.getByLabel('Название напоминания').fill('Несохранённый черновик');
    const result = await title.evaluate(async node => {
      (node as HTMLElement).click(); await new Promise(resolve => requestAnimationFrame(resolve));
      const surface = node.closest('article')!.querySelector<HTMLElement>('.motion-collapse')!;
      const closing = surface.inert && surface.getAnimations().length > 0;
      (node as HTMLElement).click(); return closing;
    });
    expect(result).toBe(true); await expect(page.getByLabel('Название напоминания')).toHaveValue('Несохранённый черновик');
    await expect.poll(() => page.locator('.motion-collapse').evaluate(node => node.getAnimations().length)).toBe(0);
    await page.getByRole('form').getByRole('button', { name: 'Отмена' }).click(); await expect(page.locator('.reminder-inline')).toHaveCount(0);
    const button = page.getByRole('button', { name: 'Новое напоминание' }); await button.click();
    let writes = 0; page.on('request', request => { if (request.method() === 'POST' && request.url().endsWith('/api/events')) writes++; });
    await page.getByLabel('Название', { exact: true }).fill('Форма уже закрывается');
    expect(await page.getByRole('dialog').evaluate(async node => {
      node.dispatchEvent(new Event('cancel', { cancelable: true }));
      await new Promise(resolve => requestAnimationFrame(resolve));
      node.querySelector('form')!.requestSubmit();
      return node.classList.contains('closing') && node.getAnimations().length > 0;
    })).toBe(true);
    await expect(page.getByRole('dialog')).toHaveCount(0); await expect(button).toBeFocused(); expect(writes).toBe(0);
  } finally { await request.delete(`/api/events/${event.id}`); }
});

test('reduced motion keeps navigation and forms immediate and functional', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' }); await ready(page);
  await page.evaluate(() => { document.startViewTransition = () => { throw new Error('Motion must be disabled'); }; });
  await page.getByRole('link', { name: 'Задачи', exact: true }).click(); await expect(page.locator('h1')).toHaveText('Задачи');
  await page.getByRole('button', { name: 'Новая задача' }).click();
  expect(await page.getByRole('dialog').evaluate(node => node.getAnimations().length)).toBe(0);
  await page.keyboard.press('Escape'); await expect(page.getByRole('dialog')).toHaveCount(0);
  await page.getByRole('link', { name: 'Календарь', exact: true }).click(); await expect(page.locator('h1')).toHaveText('Календарь');
  await page.getByRole('button', { name: 'Неделя', exact: true }).click(); await expect(page.getByRole('region', { name: 'Календарь на неделю' })).toBeVisible();
  await noOverflow(page); expect(await page.evaluate(() => document.getAnimations().length)).toBe(0);
});

test('explicit smooth mode overrides system reduced motion and persists after reload', async ({ page, request }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' }); await ready(page, '/settings');
  const setting = page.getByLabel('Анимации интерфейса', { exact: true }); await expect(setting).toHaveValue('system');
  await setting.selectOption('full');
  const profile = await page.evaluate(() => localStorage.getItem('rhythm.profile.v1'));
  await expect.poll(async () => (await (await request.get(`/api/preferences/${profile}`)).json()).motion).toBe('full');
  expect(await page.locator('.switch-row input').first().evaluate(node => getComputedStyle(node, '::after').transitionDuration)).toContain('0.44s');
  await page.reload(); await expect(setting).toHaveValue('full');
  await page.getByRole('link', { name: 'Задачи', exact: true }).click(); await page.getByRole('button', { name: 'Новая задача' }).click();
  expect(await page.getByRole('dialog').evaluate(node => node.getAnimations().length)).toBeGreaterThan(0);
  await page.keyboard.press('Escape'); await expect(page.getByRole('dialog')).toHaveCount(0);
  await page.getByRole('link', { name: 'Настройки', exact: true }).click(); await setting.selectOption('reduced');
  expect(await page.locator('.switch-row input').first().evaluate(node => getComputedStyle(node, '::after').transitionDuration)).toBe('0s');
  await page.getByRole('link', { name: 'Задачи', exact: true }).click(); await page.getByRole('button', { name: 'Новая задача' }).click();
  expect(await page.getByRole('dialog').evaluate(node => node.getAnimations().length)).toBe(0);
  expect(await page.getByRole('dialog').evaluate(node => getComputedStyle(node, '::backdrop').animationName)).toBe('none');
});

test('widgets slide to their new positions without View Transitions, including add/remove and drag', async ({ page }) => {
  await page.addInitScript(() => { Object.defineProperty(document, 'startViewTransition', { value: undefined, configurable: true }); });
  await page.emulateMedia({ reducedMotion: 'reduce' }); await ready(page, '/settings');
  await page.getByLabel('Анимации интерфейса', { exact: true }).selectOption('full');
  await page.getByRole('link', { name: 'Календарь', exact: true }).click();
  await page.evaluate(async () => { await Promise.allSettled(document.getAnimations().map(animation => animation.finished)); });
  const moving = page.locator('[data-widget="tasks"] .widget');
  await page.getByRole('link', { name: 'Задачи', exact: true }).click();
  const routeFrames = await moving.evaluate(node => node.getAnimations().map(animation => (animation.effect as KeyframeEffect).getKeyframes()));
  expect(routeFrames.some(frames => String(frames[0].transform).startsWith('translate(') && frames[0].transform !== frames.at(-1)?.transform)).toBe(true);
  await page.getByRole('link', { name: 'Календарь', exact: true }).click();
  await page.evaluate(async () => { await Promise.allSettled(document.getAnimations().map(animation => animation.finished)); });
  await page.getByRole('button', { name: 'Добавить виджет', exact: true }).click();
  const picker = page.getByRole('dialog', { name: 'Ваши виджеты' });
  await picker.locator('.widget-picker > div').filter({ hasText: 'Календарь' }).getByRole('button', { name: 'Убрать' }).click();
  expect(await page.locator('[data-widget="today"] .widget').evaluate(node => node.getAnimations().length)).toBeGreaterThan(0);
  await picker.locator('.widget-picker > div').filter({ hasText: 'Календарь' }).getByRole('button', { name: 'Добавить', exact: true }).click();
  await page.keyboard.press('Escape'); await expect(picker).toHaveCount(0);
  const handle = page.getByRole('button', { name: 'Переместить: Календарь', exact: true }); await handle.focus();
  await page.keyboard.press('Space'); await page.keyboard.press('ArrowDown');
  const duration = await page.locator('[data-widget="today"]').evaluate(node => getComputedStyle(node).transitionDuration);
  expect(duration).toBe('0.44s'); await page.keyboard.press('Escape');
  await noOverflow(page);
});

test('narrow browser animates navigation beneath closing navigation and widget drawers', async ({ page }) => {
  await page.setViewportSize({ width: 593, height: 932 }); await page.emulateMedia({ reducedMotion: 'reduce' });
  await ready(page, '/settings'); await page.getByLabel('Анимации интерфейса', { exact: true }).selectOption('full');
  await page.getByRole('button', { name: 'Открыть навигацию' }).click();
  await page.getByRole('dialog', { name: 'Навигация' }).getByRole('link', { name: 'Календарь', exact: true }).click();
  await expect(page.locator('h1')).toHaveText('Календарь');
  expect(await page.locator('.page').evaluate(node => node.getAnimations().length)).toBeGreaterThan(0);
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await page.getByRole('button', { name: 'Открыть виджеты', exact: true }).click();
  await page.getByRole('dialog', { name: 'Виджеты' }).getByRole('link', { name: 'Все задачи', exact: true }).click();
  await expect(page.locator('h1')).toHaveText('Задачи');
  expect(await page.locator('.page').evaluate(node => node.getAnimations().length)).toBeGreaterThan(0);
  await expect(page.getByRole('dialog')).toHaveCount(0); await noOverflow(page);
});
