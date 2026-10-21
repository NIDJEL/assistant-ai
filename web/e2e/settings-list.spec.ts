import { test, expect, type Page } from '@playwright/test';
import { mockSchedule } from './mock-workspace';

async function settingsAccount(page: Page, providers: string[], email = 'alex@yandex.ru') {
  await mockSchedule(page);
  let user = { id: '11111111-1111-4111-8111-111111111111', name: 'Алексей Тест', email, avatar_url: '', onboarding_completed: true, providers };
  const requests: Record<string, string>[] = []; let fail = false;
  await page.route('**/api/auth/me', async route => {
    if (route.request().method() === 'PATCH') user = { ...user, ...route.request().postDataJSON() };
    await route.fulfill({ json: { user } });
  });
  await page.route('**/api/auth/email/enable', async route => {
    requests.push(route.request().postDataJSON());
    if (fail) { await route.fulfill({ status: 503, json: { error: { message: 'Не удалось сохранить пароль. Повторите попытку.' } } }); return; }
    user = { ...user, providers: [...user.providers, 'email'] }; await route.fulfill({ json: { user } });
  });
  await page.route('**/api/auth/yandex/start?link=1', async route => {
    user = { ...user, providers: [...user.providers, 'yandex'] };
    await route.fulfill({ status: 303, headers: { location: '/settings?linked=yandex' } });
  });
  return { requests, fail: (value: boolean) => { fail = value; } };
}

test('settings is a compact divided list with working name, voice and interface controls', async ({ page }, info) => {
  const errors: string[] = []; page.on('pageerror', error => errors.push(error.message));
  await settingsAccount(page, ['email']); await page.goto('/settings');
  await expect(page.locator('.settings-list > section')).toHaveCount(5);
  for (const text of ['Настройте Гену под себя', 'Голосовой ввод', 'Распознавание под вас', 'Данные', 'Интеграции']) await expect(page.locator('.settings-page').getByText(text, { exact: true })).toHaveCount(0);
  await expect(page.getByText('Гена, мужской', { exact: true })).toBeVisible(); await expect(page.getByText('Дмитрий, мужской')).toHaveCount(0);
  const title = (await page.locator('h1').boundingBox())!; const account = (await page.getByRole('heading', { name: 'Аккаунт', exact: true }).boundingBox())!;
  expect(account.y - title.y - title.height).toBeLessThanOrEqual(26);
  await page.getByRole('textbox', { name: 'Ваше имя', exact: true }).fill('Новое имя'); await page.getByRole('button', { name: 'Сохранить имя', exact: true }).click(); await expect(page.getByRole('status').filter({ hasText: 'Имя сохранено' })).toBeVisible();
  const volume = page.getByRole('slider', { name: 'Громкость голоса', exact: true }); await volume.fill('40'); await volume.blur(); await expect(page.locator('output[for="assistant-voice-volume"]')).toHaveText('40%');
  const widgets = page.getByRole('switch', { name: 'Показывать панель на компьютере' });
  for (const visible of [false, true]) {
    await widgets.setChecked(visible);
    for (const width of [1920, 1440, 1180, 1024, 760, 390, 320]) {
      await page.setViewportSize({ width, height: 1000 });
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      for (const section of await page.locator('.settings-list > section').all()) {
        expect(await section.evaluate(node => getComputedStyle(node).borderRadius)).toBe('0px');
        expect(await section.evaluate(node => node.scrollWidth <= node.clientWidth + 1)).toBe(true);
      }
      if ([1440, 390].includes(width) && !visible) await page.locator('.settings-page').screenshot({ path: info.outputPath(`settings-list-${width}.png`) });
    }
  }
  expect(errors).toEqual([]);
});

test('Yandex email is connected and can gain a Gena password without replacing the account', async ({ page }, info) => {
  const state = await settingsAccount(page, ['yandex']); await page.goto('/settings');
  const email = page.locator('.account-methods > .settings-row').filter({ hasText: 'Email' });
  await expect(email).toContainText('alex@yandex.ru'); await expect(email).toContainText('Подключена'); await expect(email).not.toContainText('Не подключён');
  await expect(page.getByRole('link', { name: 'Подключить Яндекс' })).toHaveCount(0);
  await email.getByRole('button', { name: 'Задать пароль' }).click(); const form = page.getByRole('form', { name: 'Пароль для входа по почте' });
  await form.getByLabel('Новый пароль', { exact: true }).fill('my Gena password 123'); await form.getByLabel('Повторите новый пароль', { exact: true }).fill('mismatching password');
  await form.getByRole('button', { name: 'Сохранить пароль' }).click(); await expect(form.getByRole('alert')).toHaveText('Пароли не совпадают'); expect(state.requests).toHaveLength(0);
  await form.getByLabel('Повторите новый пароль', { exact: true }).fill('my Gena password 123'); state.fail(true);
  await form.getByRole('button', { name: 'Сохранить пароль' }).click(); await expect(form.getByRole('alert')).toContainText('Не удалось сохранить'); await expect(form.getByLabel('Новый пароль', { exact: true })).toHaveValue('my Gena password 123');
  await page.setViewportSize({ width: 320, height: 900 }); expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await form.screenshot({ path: info.outputPath('set-email-password-mobile.png') }); state.fail(false);
  await form.getByRole('button', { name: 'Сохранить пароль' }).click(); await expect(form).toHaveCount(0); await expect(page.getByRole('status').filter({ hasText: 'Вход по почте подключён' })).toBeVisible();
  expect(state.requests).toHaveLength(2); expect(state.requests[1]).toEqual({ password: 'my Gena password 123', password_confirmation: 'my Gena password 123' });
  await expect(email.getByRole('button', { name: 'Задать пароль' })).toHaveCount(0); await page.reload(); await expect(email).toContainText('Подключена'); await expect(page.getByText('Для входа по почте задайте пароль.')).toHaveCount(0);
});

test('email accounts can explicitly link Yandex and accounts without an email do not invent one', async ({ page }) => {
  await settingsAccount(page, ['email']); await page.goto('/settings');
  const yandex = page.getByRole('link', { name: 'Подключить Яндекс', exact: true }); await expect(yandex).toHaveAttribute('href', '/api/auth/yandex/start?link=1');
  await yandex.click(); await expect(page).toHaveURL(/settings\?linked=yandex/); await expect(page.getByRole('status').filter({ hasText: 'Способ входа подключён' })).toBeVisible(); await expect(yandex).toHaveCount(0);
  await expect(page.locator('.account-methods')).toContainText('Подключён'); await expect(page.locator('.account-methods')).toContainText('Подключена');
  await settingsAccount(page, ['yandex'], ''); await page.goto('/settings');
  const email = page.locator('.account-methods > .settings-row').filter({ hasText: 'Email' }); await expect(email).toContainText('Почта не передана Яндексом'); await expect(email).not.toContainText('Подключена'); await expect(page.getByRole('button', { name: 'Задать пароль' })).toHaveCount(0);
});
