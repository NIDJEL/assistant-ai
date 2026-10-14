import { test, expect, type Page } from './fixtures';
import { ready } from './helpers';

async function settled(page: Page) {
  await page.evaluate(async () => { await Promise.allSettled(document.getAnimations().filter(a => a.effect?.getComputedTiming().iterations !== Infinity).map(a => a.finished)); });
}

for (const mode of ['desktop', 'scrolled', 'drawer'] as const) test(`dragged widget follows the pointer in ${mode} and retains the drop order`, async ({ page }, info) => {
  if (mode === 'drawer') await page.setViewportSize({ width: 700, height: 850 });
  else await page.setViewportSize({ width: 1440, height: mode === 'scrolled' ? 580 : 900 });
  await ready(page); await settled(page);
  if (mode === 'drawer') { await page.getByRole('button', { name: 'Открыть виджеты' }).click(); await settled(page); }
  const panel = mode === 'drawer' ? page.getByRole('dialog', { name: 'Виджеты', exact: true }) : page.locator('.widgets-panel');
  if (mode === 'scrolled') await panel.evaluate(node => { node.scrollTop = 150; });
  const handle = panel.getByRole('button', { name: 'Переместить: Сегодня', exact: true });
  await handle.scrollIntoViewIfNeeded();
  const grip = (await handle.boundingBox())!;
  const card = (await panel.locator('[data-widget="today"]').boundingBox())!;
  const start = { x: grip.x + grip.width / 2, y: grip.y + grip.height / 2 };
  const offset = { x: start.x - card.x, y: start.y - card.y };
  await page.mouse.move(start.x, start.y); await page.mouse.down();
  for (const delta of [{ x: -20, y: -20 }, { x: -75, y: -65 }]) {
    const point = { x: start.x + delta.x, y: start.y + delta.y };
    await page.mouse.move(point.x, point.y, { steps: 8 });
    const overlay = page.locator('.widget-drag-overlay'); await expect(overlay).toBeVisible();
    await expect.poll(async () => { const box = (await overlay.boundingBox())!; return Math.abs(box.x + offset.x - point.x) + Math.abs(box.y + offset.y - point.y); }).toBeLessThan(3);
    expect(Math.abs((await overlay.boundingBox())!.width - card.width)).toBeLessThan(1);
    const visible = await overlay.evaluate(node => {
      const rect = node.getBoundingClientRect();
      // The visual layer escapes panel transforms and modal top-layer clipping.
      return node.closest('.widget-drag-layer')?.matches(':popover-open') && rect.right > 0 && rect.left < innerWidth;
    });
    expect(visible).toBe(true);
  }
  await page.screenshot({ path: info.outputPath(`widget-drag-${mode}.png`) });
  const target = (await panel.locator('[data-widget="calendar"]').boundingBox())!;
  await page.mouse.move(start.x, Math.max(40, target.y + 35), { steps: 12 });
  const saved = page.waitForResponse(r => r.request().method() === 'PUT' && r.url().includes('/api/preferences/'));
  await page.mouse.up(); await saved;
  await expect(panel.locator('.widget-list > [data-widget]').first()).toHaveAttribute('data-widget', 'today');
  await expect(page.locator('.widget-drag-overlay')).toHaveCount(0);
  await page.reload(); await expect(page.locator('.widgets-panel .widget-list > [data-widget]').first()).toHaveAttribute('data-widget', 'today');
});
