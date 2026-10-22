import { test, expect } from '@playwright/test';
import { mockSchedule } from './mock-workspace';
import { themeContrastIssues } from './theme-contrast';

for (const theme of ['dark', 'light']) {
  test(`registration explains weak passwords without sending them: ${theme}`, async ({ page }, info) => {
    await page.addInitScript(theme => localStorage.setItem('gena.theme.v1', theme), theme);
    await page.setViewportSize({ width: 390, height: 844 }); await page.goto('/auth');
    await page.getByRole('tab', { name: 'Создать аккаунт' }).click();
    await page.getByLabel('Имя', { exact: true }).fill('Анна'); await page.getByLabel('Почта', { exact: true }).fill(`password-${crypto.randomUUID()}@example.test`);
    await page.locator('#auth-terms').check(); await page.locator('#auth-data-consent').check();
    let attempts = 0; page.on('request', request => { if (request.url().endsWith('/api/auth/register')) attempts++; });
    for (const password of [' '.repeat(10), '\u00a0'.repeat(12), 'abcabcabcabc', 'password12345', '1234567890', 'Onlyletters']) {
      await page.getByLabel('Пароль', { exact: true }).fill(password); await page.getByLabel('Повторите пароль').fill(password);
      await expect(page.locator('#auth-password-strength')).toHaveAttribute('data-level', '1');
      await page.getByRole('button', { name: 'Создать аккаунт', exact: true }).click(); await expect(page.getByRole('alert')).toBeVisible(); expect(attempts).toBe(0);
    }
    await page.screenshot({ path: info.outputPath(`password-error-${theme}.png`), fullPage: true });
    await page.getByLabel('Пароль', { exact: true }).fill('Лампа светит возле окна'); await page.getByLabel('Повторите пароль').fill('Лампа светит возле окна');
    await expect(page.locator('#auth-password-strength')).toHaveAttribute('data-level', '3');
    // The submitted value, including spaces, must survive without trimming.
    await page.getByLabel('Пароль', { exact: true }).fill('  River!coast7  '); await page.getByLabel('Повторите пароль').fill('  River!coast7  ');
    await expect(page.locator('#auth-password-strength')).toHaveAttribute('data-level', '2');
    const sent = page.waitForRequest(request => request.url().endsWith('/api/auth/register'));
    await page.getByRole('button', { name: 'Создать аккаунт', exact: true }).click(); expect((await sent).postDataJSON().password).toBe('  River!coast7  ');
    await expect(page).toHaveURL(/\/onboarding$/);
  });
}

test('email password setup uses the same live meter and rejects whitespace before API calls', async ({ page }, info) => {
  await mockSchedule(page);
  const user = { id: '11111111-1111-4111-8111-111111111111', name: 'Анна', email: 'anna@yandex.ru', avatar_url: '', onboarding_completed: true, providers: ['yandex'] };
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user } }));
  let attempts = 0; await page.route('**/api/auth/email/enable', route => { attempts++; return route.fulfill({ status: 503, json: { error: { message: 'Тестовый ответ' } } }); });
  await page.goto('/settings'); await page.getByRole('button', { name: 'Задать пароль' }).click();
  const form = page.getByRole('form', { name: 'Пароль для входа по почте' });
  await form.getByLabel('Новый пароль', { exact: true }).fill(' '.repeat(10)); await form.getByLabel('Повторите новый пароль').fill(' '.repeat(10));
  await form.getByRole('button', { name: 'Сохранить пароль' }).click(); await expect(form.getByRole('alert')).toContainText('без учёта пробелов'); expect(attempts).toBe(0);
  await form.getByLabel('Новый пароль', { exact: true }).fill('River!coast7'); await form.getByLabel('Повторите новый пароль').fill('River!coast7');
  for (const theme of ['Тёмная', 'Светлая']) {
    await page.getByRole('radio', { name: theme, exact: true }).check();
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme === 'Тёмная' ? 'dark' : 'light');
    await page.evaluate(async () => { await Promise.allSettled(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished)); });
    expect(await themeContrastIssues(page)).toEqual([]);
    await page.setViewportSize({ width: 320, height: 900 }); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await form.screenshot({ path: info.outputPath(`password-settings-${theme}.png`) });
  }
  await form.getByRole('button', { name: 'Сохранить пароль' }).click(); await expect.poll(() => attempts).toBe(1);
});
