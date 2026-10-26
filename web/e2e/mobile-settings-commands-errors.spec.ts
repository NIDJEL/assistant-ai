import { test, expect } from '@playwright/test';
import { mockSchedule } from './mock-workspace';
import { noOverflow } from './helpers';

// These tests stub API responses; worker-controlled requests bypass WebKit routes.
test.use({ serviceWorkers: 'block' });

test('mobile integration settings open on the first tap and account settings navigate correctly', async ({ page }) => {
  await mockSchedule(page); await page.emulateMedia({ reducedMotion: 'reduce' });
  for (const width of [320, 390, 768]) {
    await page.setViewportSize({ width, height: 844 }); await page.goto('/integrations');
    for (const name of ['Яндекс Календарь', 'Google Calendar', 'Zoom']) {
      const card = page.locator('.integration-card').filter({ has: page.getByRole('heading', { name, exact: true }) });
      await card.getByRole('button', { name: 'Настроить', exact: true }).click();
      const detail = page.getByRole('dialog', { name: 'Интеграция', exact: true });
      await expect(detail).toBeVisible(); await expect(detail).toContainText(name); await noOverflow(page);
      await detail.getByRole('button', { name: 'Закрыть', exact: true }).click(); await expect(detail).toHaveCount(0);
      await card.getByRole('button', { name: 'Настроить', exact: true }).click(); await expect(detail).toBeVisible();
      await page.keyboard.press('Escape'); await expect(detail).toHaveCount(0);
    }
    if (width < 761) {
      await page.getByRole('button', { name: 'Открыть навигацию' }).click();
      await page.getByRole('dialog', { name: 'Навигация' }).getByRole('link', { name: 'Настройки', exact: true }).click();
      await expect(page.getByRole('heading', { name: 'Настройки', exact: true })).toBeVisible();
      await expect(page.getByRole('dialog', { name: 'Навигация' })).toHaveCount(0);
    }
  }
});

test('plus menu works in both themes and sizes without sending a command', async ({ page }, info) => {
  await mockSchedule(page); await page.emulateMedia({ reducedMotion: 'reduce' }); let sent = 0;
  await page.goto('/');
  page.on('request', request => { if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/chat') sent++; });
  for (const theme of ['dark', 'light']) {
    await page.goto('/settings'); await page.getByRole('radio', { name: theme === 'light' ? 'Светлая' : 'Тёмная', exact: true }).check();
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
    for (const width of [320, 390, 1440]) {
      await page.evaluate(theme => localStorage.setItem('gena.theme.v1', theme), theme);
      await page.setViewportSize({ width, height: 900 }); await page.goto('/');
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
      const toggle = page.getByRole('button', { name: 'Быстрые команды', exact: true });
      const input = page.getByRole('textbox', { name: 'Сообщение ассистенту' });
      await toggle.click(); const menu = page.getByRole('group', { name: 'Быстрые команды', exact: true });
      await expect(menu).toBeVisible(); await expect(menu).toBeInViewport({ ratio: 1 }); await noOverflow(page);
      await page.screenshot({ path: info.outputPath(`commands-${theme}-${width}.png`), fullPage: true });
      await toggle.click(); await expect(menu).toHaveCount(0);
      for (const [label, template] of [['Напомнить', 'Напомни мне [о чём] [когда]'], ['Добавить в календарь', 'Добавь в календарь [событие] [дата и время]'], ['Создать задачу', 'Создай задачу [что нужно сделать]']]) {
        await toggle.click(); await menu.getByRole('button', { name: label, exact: true }).click();
        await expect(input).toHaveValue(template); await expect(input).toBeFocused(); await expect(menu).toHaveCount(0);
        expect(await input.evaluate((element: HTMLTextAreaElement) => element.value.slice(element.selectionStart, element.selectionEnd))).toMatch(/^\[.+\]$/);
      }
      await toggle.click(); await page.getByRole('heading', { name: 'Чем помочь?' }).click(); await expect(menu).toHaveCount(0);
      await expect(input).toHaveValue('Создай задачу [что нужно сделать]'); expect(sent).toBe(0);
    }
  }
});

test('error pages stay readable in both themes and recover from an API gateway failure', async ({ page }, info) => {
  await mockSchedule(page);
  await page.goto('/');
  for (const theme of ['dark', 'light']) {
    await page.evaluate(theme => localStorage.setItem('gena.theme.v1', theme), theme);
    await page.emulateMedia({ colorScheme: theme as 'dark' | 'light' });
    await page.setViewportSize({ width: 390, height: 844 });
    for (const status of [403, 404, 429, 500, 502, 503, 504]) {
      await page.goto(`/errors/${status}`);
      await expect(page.locator('.error-code')).toHaveText(String(status)); await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
      await expect(page.getByRole('link', { name: /На главную/ })).toBeInViewport(); await noOverflow(page);
      if ([404, 503].includes(status)) await page.screenshot({ path: info.outputPath(`error-${status}-${theme}.png`), fullPage: true });
    }
  }
  await page.route('**/api/auth/me', route => route.fulfill({ status: 502, contentType: 'text/html', body: '<html>Bad gateway</html>' }));
  await page.goto('/'); await expect(page.getByRole('heading', { name: 'Гена пока не отвечает' })).toBeVisible();
  await page.unroute('**/api/auth/me'); await page.getByRole('button', { name: 'Попробовать снова' }).click();
  await expect(page.getByRole('heading', { name: 'Чем помочь?' })).toBeVisible();
});
