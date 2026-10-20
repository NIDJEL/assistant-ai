import { test, expect } from '@playwright/test';
import { noOverflow } from './helpers';

test('confirmations are separate, unchecked, and required for email and Yandex', async ({ page, context }) => {
  await page.goto('/auth');
  const terms = page.getByRole('checkbox', { name: /Принимаю/ });
  const consent = page.getByRole('checkbox', { name: /Даю/ });
  const email = page.getByRole('button', { name: 'Войти', exact: true });
  const oauth = page.getByRole('button', { name: 'Войти через Яндекс' });
  await expect(terms).not.toBeChecked(); await expect(consent).not.toBeChecked();
  await expect(email).toBeDisabled(); await expect(oauth).toBeDisabled();
  await terms.check(); await expect(email).toBeDisabled(); await expect(oauth).toBeDisabled();
  const tabPromise = context.waitForEvent('page');
  await page.getByRole('link', { name: 'согласие на обработку персональных данных' }).click();
  const document = await tabPromise;
  await expect(document.getByRole('heading', { level: 1 })).toHaveText('Согласие на обработку персональных данных');
  await expect(consent).not.toBeChecked(); await document.close();
  await consent.check(); await expect(email).toBeEnabled(); await expect(oauth).toBeEnabled();
  await consent.uncheck(); await expect(email).toBeDisabled();
  await page.getByRole('tab', { name: 'Создать аккаунт' }).click();
  await expect(page.getByRole('button', { name: 'Создать аккаунт', exact: true })).toBeDisabled();
  await page.reload(); await expect(terms).not.toBeChecked(); await expect(consent).not.toBeChecked();
});

test('legal documents stay public during auth errors and readable on desktop and mobile', async ({ page }, info) => {
  await page.route('**/api/auth/me', route => route.fulfill({ status: 503, json: { error: { message: 'Сервис временно недоступен' } } }));
  for (const viewport of [{ width: 1440, height: 1000 }, { width: 390, height: 844 }, { width: 320, height: 740 }]) {
    await page.setViewportSize(viewport);
    for (const [path, title] of [['/terms', 'Условия использования'], ['/privacy', 'Политика обработки персональных данных'], ['/personal-data-consent', 'Согласие на обработку персональных данных'], ['/privacy-policy', 'Политика обработки персональных данных']]) {
      const response = await page.goto(path); expect(response?.status()).toBe(200);
      await expect(page.getByRole('heading', { level: 1 })).toHaveText(title);
      await expect(page.locator('.legal-article')).toContainText('525821206600');
      await expect(page.locator('.legal-article')).toContainText('Рябцов Никита Алексеевич');
      expect(await page.locator('.legal-article').evaluate(element => parseFloat(getComputedStyle(element).fontSize))).toBeGreaterThanOrEqual(16);
      await noOverflow(page);
      if (path === '/privacy' && viewport.width >= 390) await page.screenshot({ path: info.outputPath(`privacy-${viewport.width}.png`) });
      if (viewport.width <= 760) await page.locator('.legal-mobile-toc summary').click();
      const toc = page.getByRole('navigation', { name: 'Содержание' });
      await expect(toc).toBeVisible(); await toc.getByRole('link').last().click();
      await expect(page.locator('.legal-section').last().getByRole('heading')).toBeInViewport();
    }
  }
});

test('auth layouts show the new confirmations at desktop and mobile sizes', async ({ page }, info) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  for (const viewport of [{ width: 1440, height: 1000 }, { width: 390, height: 844 }]) {
    await page.setViewportSize(viewport); await page.goto('/auth');
    await expect(page.getByRole('checkbox', { name: /Принимаю/ })).toBeVisible();
    await settle(page);
    await page.screenshot({ path: info.outputPath(`auth-login-${viewport.width}.png`), fullPage: true });
    await page.getByRole('tab', { name: 'Создать аккаунт' }).click();
    await expect(page.getByLabel('Повторите пароль')).toBeVisible();
    await settle(page);
    await page.screenshot({ path: info.outputPath(`auth-register-${viewport.width}.png`), fullPage: true });
    await noOverflow(page);
  }
});

async function settle(page: import('@playwright/test').Page) {
  await page.evaluate(async () => { await Promise.allSettled(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished)); });
}
