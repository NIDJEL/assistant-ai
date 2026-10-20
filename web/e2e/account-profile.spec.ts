import { test, expect } from './fixtures';
import { ready, noOverflow } from './helpers';

async function settle(page: import('@playwright/test').Page) {
  await page.evaluate(async () => { await Promise.allSettled(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished)); });
}

test('chat and other pages stay centered with collapsed navigation and either widget state', async ({ page }) => {
  await page.setViewportSize({ width: 2560, height: 1080 }); await ready(page, '/');
  await page.getByRole('button', { name: 'Свернуть навигацию', exact: true }).click(); await settle(page);
  for (const widgets of [true, false]) {
    if (!widgets) await page.getByRole('button', { name: 'Скрыть виджеты', exact: true }).click();
    for (const path of ['/', '/calendar', '/tasks', '/settings']) {
      await ready(page, path); await settle(page);
      const geometry = await page.locator('.main-panel > .page').evaluate(node => {
        const page = node.getBoundingClientRect(); const main = node.parentElement!.getBoundingClientRect();
        return { offset: Math.abs(page.left + page.width / 2 - main.left - main.width / 2), width: page.width };
      });
      expect(geometry.offset).toBeLessThan(1); expect(geometry.width).toBeLessThanOrEqual(path === '/' ? 1100 : 1280); await noOverflow(page);
    }
  }
});

test('avatar menu opens settings and logs out, while account persists names and photos across tabs', async ({ page, context }, info) => {
  await ready(page, '/');
  const trigger = page.getByRole('button', { name: 'Меню аккаунта', exact: true });
  await trigger.click(); const menu = page.getByRole('menu', { name: 'Аккаунт' });
  await expect(menu.getByRole('menuitem', { name: 'Настройки' })).toBeFocused();
  await page.keyboard.press('ArrowDown'); await expect(menu.getByRole('menuitem', { name: 'Выйти' })).toBeFocused();
  await page.keyboard.press('Escape'); await expect(menu).toHaveCount(0); await expect(trigger).toBeFocused();
  await trigger.click(); await menu.getByRole('menuitem', { name: 'Настройки' }).click();
  await expect(page.locator('h1')).toHaveText('Настройки'); await expect(page.getByRole('heading', { name: 'Общие', exact: true })).toHaveCount(0);
  await expect(page.locator('.account-card')).not.toContainText('VK ID');
  const other = await context.newPage();
  try {
    await ready(other, '/');
    await page.getByLabel('Ваше имя', { exact: true }).fill('Новый профиль'); await page.getByRole('button', { name: 'Сохранить имя', exact: true }).click();
    await expect(page.locator('.account-card').getByRole('status')).toContainText('Имя сохранено'); await expect(other.locator('.profile-name')).toHaveText('Новый профиль');
    const base64 = await page.evaluate(() => { const canvas = document.createElement('canvas'); canvas.width = 400; canvas.height = 300; const ctx = canvas.getContext('2d')!; ctx.fillStyle = '#1982ff'; ctx.fillRect(0, 0, 400, 300); ctx.fillStyle = '#ffffff'; ctx.fillRect(150, 100, 100, 100); return canvas.toDataURL('image/png').split(',')[1]; });
    await page.getByLabel('Фото профиля', { exact: true }).setInputFiles({ name: 'profile.png', mimeType: 'image/png', buffer: Buffer.from(base64, 'base64') });
    await expect(page.locator('.account-card').getByRole('status')).toContainText('Фото сохранено'); await expect(page.locator('.account-avatar img')).toBeVisible(); await expect(other.locator('.profile-trigger img')).toBeVisible();
    await page.reload(); await expect(page.getByLabel('Ваше имя', { exact: true })).toHaveValue('Новый профиль'); await expect(page.locator('.account-avatar img')).toBeVisible();
    await settle(page); await page.screenshot({ path: info.outputPath('account-profile.png'), animations: 'disabled' });
    await page.getByLabel('Фото профиля', { exact: true }).setInputFiles({ name: 'invalid.svg', mimeType: 'image/svg+xml', buffer: Buffer.from('<svg/>') });
    await expect(page.locator('.account-card [role="alert"]')).toContainText('Выберите изображение'); await expect(page.locator('.account-avatar img')).toBeVisible();
    await page.getByRole('button', { name: 'Удалить фото', exact: true }).click(); await expect(page.locator('.account-card').getByRole('status')).toContainText('Фото удалено'); await expect(page.locator('.account-avatar img')).toHaveCount(0); await expect(other.locator('.profile-trigger img')).toHaveCount(0);
    await page.getByRole('button', { name: 'Меню аккаунта', exact: true }).click();
    const logout = page.getByRole('menuitem', { name: 'Выйти', exact: true }); await expect(logout).toHaveCSS('color', 'rgb(255, 148, 164)');
    await page.screenshot({ path: info.outputPath('profile-menu.png'), animations: 'disabled' });
    await logout.click(); await expect(page).toHaveURL(/\/auth$/); await expect(other).toHaveURL(/\/auth$/);
  } finally { await other.close(); }
});

test('avatar menu fits collapsed navigation and mobile drawer', async ({ page }) => {
  await ready(page, '/'); await page.getByRole('button', { name: 'Свернуть навигацию', exact: true }).click(); await settle(page);
  await page.getByRole('button', { name: 'Меню аккаунта', exact: true }).click(); await expect(page.getByRole('menuitem', { name: 'Настройки' })).toBeVisible(); await noOverflow(page);
  await page.locator('h1').click(); await expect(page.getByRole('menu')).toHaveCount(0);
  await page.setViewportSize({ width: 390, height: 844 }); await page.getByRole('button', { name: 'Открыть навигацию' }).click(); await settle(page);
  const drawer = page.getByRole('dialog', { name: 'Навигация', exact: true }); await drawer.getByRole('button', { name: 'Меню аккаунта', exact: true }).click();
  await expect(drawer.getByRole('menuitem', { name: 'Настройки' })).toBeVisible(); await noOverflow(page);
  await drawer.getByRole('menuitem', { name: 'Настройки' }).click(); await expect(page.locator('h1')).toHaveText('Настройки'); await expect(drawer).toHaveCount(0); await noOverflow(page);
});
