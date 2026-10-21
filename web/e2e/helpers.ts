import { expect, type Page, type TestInfo } from '@playwright/test';
import { mkdir } from 'node:fs/promises';
import { resolve } from 'node:path';
export async function ready(page: Page, path = '/') {
  await page.goto(path);
  if (new URL(page.url()).pathname === '/') await expect(page.locator('.today-page')).toHaveAttribute('aria-busy', 'false');
  else await expect(page.locator('h1')).toBeVisible();
}
async function settleMotion(page: Page) {
  await page.evaluate(async () => {
    await Promise.allSettled(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished));
  });
}
export async function screenshot(page: Page, name: string, info: TestInfo) {
  await settleMotion(page);
  const directory = process.env.P1_CAPTURE_SCREENSHOTS ? resolve('../docs/screenshots/p1') : info.outputDir;
  await mkdir(directory, { recursive: true }); const path = resolve(directory, `${name}.png`);
  await page.screenshot({ path, fullPage: false }); await info.attach(name, { path, contentType: 'image/png' });
}
export async function noOverflow(page: Page) {
  await settleMotion(page);
  const problems = await page.evaluate(() => {
    const viewport = document.documentElement.clientWidth; const failures: string[] = [];
    if (document.documentElement.scrollWidth > viewport + 1) failures.push(`page: ${document.documentElement.scrollWidth}/${viewport}`);
    for (const element of document.querySelectorAll('.widgets-panel, .dialog, .main-panel')) {
      if (element.getClientRects().length && element.scrollWidth > element.clientWidth + 1) failures.push(`${element.className}: ${element.scrollWidth}/${element.clientWidth}`);
    }
    for (const button of document.querySelectorAll('button')) {
      const rect = button.getBoundingClientRect(); if (rect.width && rect.height && (rect.left < -1 || rect.right > viewport + 1)) failures.push(`button: ${button.getAttribute('aria-label') || button.textContent}`);
    }
    return failures;
  });
  expect(problems).toEqual([]);
}
