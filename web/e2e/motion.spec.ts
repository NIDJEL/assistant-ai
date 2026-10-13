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
