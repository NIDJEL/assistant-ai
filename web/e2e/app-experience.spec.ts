import { test, expect } from './fixtures';
import { noOverflow, ready } from './helpers';

test('the unified composer grows with text, shrinks after editing and keeps all actions reachable', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await ready(page);
  const input = page.getByRole('textbox', { name: 'Сообщение ассистенту' });
  for (const viewport of [{ width: 1440, height: 900 }, { width: 390, height: 844 }, { width: 320, height: 740 }]) {
    await page.setViewportSize(viewport);
    await input.fill('Одна строка');
    const initial = (await input.boundingBox())!.height;
    expect(initial).toBeLessThanOrEqual(48);
    await expect(input).toHaveCSS('resize', 'none'); await expect(input).toHaveCSS('border-top-width', '0px');
    await input.fill('Первая строка\nВторая строка\nТретья строка');
    expect((await input.boundingBox())!.height).toBeGreaterThan(initial + 20);
    await input.fill(Array.from({ length: 18 }, (_, i) => `Строка ${i}`).join('\n'));
    expect((await input.boundingBox())!.height).toBeLessThanOrEqual(160);
    await expect(input).toHaveCSS('overflow-y', 'auto');
    await expect(page.getByRole('button', { name: 'Отправить сообщение' })).toBeInViewport({ ratio: 1 });
    await input.fill(''); expect((await input.boundingBox())!.height).toBe(initial);
    await expect(page.getByRole('button', { name: 'Отправить сообщение' })).toBeDisabled();
    expect(await page.evaluate(() => document.documentElement.scrollHeight <= innerHeight + 1)).toBe(true);
    await noOverflow(page);
  }
  await expect(page.getByText(/Enter — отправить/)).toHaveCount(0);
  await expect(page.locator('.quick-actions, .widget-drop-zone')).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Настройки виджетов', exact: true })).toHaveCount(0);
});

test('mobile workspace safe areas protect navigation and the composer in an installed app', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 }); await ready(page);
  await page.evaluate(() => { document.documentElement.style.setProperty('--safe-top', '47px'); document.documentElement.style.setProperty('--safe-bottom', '34px'); });
  const menu = (await page.getByRole('button', { name: 'Открыть навигацию' }).boundingBox())!;
  const composer = (await page.locator('.composer').boundingBox())!;
  expect(menu.y).toBeGreaterThanOrEqual(47); expect(composer.y + composer.height).toBeLessThanOrEqual(810);
  await noOverflow(page);
});
