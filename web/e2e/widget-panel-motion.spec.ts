import { test, expect, type Page } from './fixtures';
import { ready, noOverflow, screenshot } from './helpers';

async function settle(page: Page) {
  await page.evaluate(async () => { await Promise.allSettled(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished)); });
}

test('widget collapse button has its own space beside the title at every desktop width', async ({ page }, info) => {
  await ready(page, '/calendar');
  for (const width of [1180, 1280, 1440, 1920, 2560]) {
    await page.setViewportSize({ width, height: 900 });
    const heading = await page.locator('.widgets-heading').evaluate(node => {
      const title = node.querySelector('h2')!.getBoundingClientRect(); const toggle = node.querySelector('.widget-panel-toggle')!.getBoundingClientRect();
      const controls = [...node.querySelectorAll('button')].map(button => button.getBoundingClientRect());
      return { title: { left: title.left, right: title.right }, controls: controls.map(rect => ({ left: rect.left, right: rect.right })), toggleRight: toggle.right };
    });
    expect(heading.toggleRight).toBeLessThan(heading.title.left);
    expect(heading.controls.slice(1).every(rect => rect.left >= heading.title.right)).toBe(true);
    await noOverflow(page);
    if (width === 1440) await screenshot(page, 'widget-heading', info);
  }
});

for (const fallback of [false, true]) test(`whole widget panel slides with unchanged card dimensions${fallback ? ' without native snapshots and with explicit Smooth mode' : ''}`, async ({ page }, info) => {
  if (fallback) {
    await page.addInitScript(() => { Object.defineProperty(document, 'startViewTransition', { value: undefined, configurable: true }); });
    await page.emulateMedia({ reducedMotion: 'reduce' }); await ready(page, '/settings');
    await page.getByLabel('Анимации интерфейса', { exact: true }).selectOption('full');
  }
  await ready(page, '/calendar'); await settle(page);
  const before = await page.locator('.widgets-panel').evaluate(node => ({ left: node.getBoundingClientRect().left, width: node.getBoundingClientRect().width, card: node.querySelector('.widget')!.getBoundingClientRect().width, navigation: document.querySelector('.sidebar')!.getBoundingClientRect().left }));
  const closing = await page.locator('.app-shell').evaluate(async shell => {
    (shell.querySelector('.widget-panel-toggle') as HTMLButtonElement).click();
    await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    const animation = shell.getAnimations().find(animation => animation instanceof CSSTransition && animation.transitionProperty === 'grid-template-columns');
    if (!animation) throw new Error('Widget region changed without a layout animation');
    animation.pause(); animation.currentTime = Number(animation.effect!.getTiming().duration) * .15;
    Object.assign(window, { panelAnimation: animation });
    await new Promise(resolve => requestAnimationFrame(resolve));
    const panel = shell.querySelector<HTMLElement>('.widgets-panel')!; const region = shell.querySelector('.widgets-region')!;
    return { left: panel.getBoundingClientRect().left, width: panel.getBoundingClientRect().width, card: panel.querySelector('.widget')!.getBoundingClientRect().width, viewport: region.getBoundingClientRect().width, opacity: getComputedStyle(panel).opacity, inert: panel.inert, navigation: shell.querySelector('.sidebar')!.getBoundingClientRect().left };
  });
  expect(closing.left).toBeGreaterThan(before.left + 10); expect(closing.viewport).toBeGreaterThan(10); expect(closing.viewport).toBeLessThan(before.width - 10);
  expect(closing.width).toBe(before.width); expect(closing.card).toBe(before.card); expect(closing.opacity).toBe('1'); expect(closing.inert).toBe(true); expect(closing.navigation).toBe(before.navigation);
  if (!fallback) { const path = info.outputPath('widgets-closing.png'); await page.screenshot({ path }); await info.attach('widgets-closing', { path, contentType: 'image/png' }); }
  await page.evaluate(() => (window as unknown as { panelAnimation: Animation }).panelAnimation.play());
  await expect(page.locator('.widgets-panel')).toHaveCount(0); await settle(page);
  const opening = await page.getByRole('button', { name: 'Показать виджеты', exact: true }).evaluate(async button => {
    (button as HTMLButtonElement).click(); await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    const shell = document.querySelector('.app-shell')!;
    const animation = shell.getAnimations().find(animation => animation instanceof CSSTransition && animation.transitionProperty === 'grid-template-columns');
    if (!animation) throw new Error('Widget region opened without a layout animation');
    animation.pause(); animation.currentTime = Number(animation.effect!.getTiming().duration) * .15;
    await new Promise(resolve => requestAnimationFrame(resolve));
    const panel = shell.querySelector<HTMLElement>('.widgets-panel')!;
    const geometry = { width: panel.getBoundingClientRect().width, viewport: shell.querySelector('.widgets-region')!.getBoundingClientRect().width, opacity: getComputedStyle(panel).opacity, inert: panel.inert };
    animation.play(); return geometry;
  });
  expect(opening.viewport).toBeGreaterThan(10); expect(opening.viewport).toBeLessThan(before.width - 10); expect(opening.width).toBe(before.width); expect(opening.opacity).toBe('1'); expect(opening.inert).toBe(false);
  await settle(page);
  expect(await page.locator('.widgets-panel').evaluate(node => node.getBoundingClientRect().width)).toBe(before.width);
  await noOverflow(page);
  // Reverse a closing panel before its retained content is removed.
  await page.locator('.app-shell').evaluate(async shell => {
    (shell.querySelector('.widget-panel-toggle') as HTMLButtonElement).click();
    await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    (shell.querySelector('.desktop-widget-button') as HTMLButtonElement).click();
  });
  await settle(page); await expect(page.getByRole('button', { name: 'Скрыть виджеты', exact: true })).toBeVisible(); await noOverflow(page);
  if (fallback) {
    await ready(page, '/settings'); await page.getByLabel('Анимации интерфейса', { exact: true }).selectOption('reduced');
    await page.getByRole('button', { name: 'Скрыть виджеты', exact: true }).click(); await expect(page.locator('.widgets-panel')).toHaveCount(0);
    expect(await page.locator('.app-shell').evaluate(node => node.getAnimations().length)).toBe(0);
  }
});
