import { test, expect } from '@playwright/test';

test('a new account imports history from settings and can safely repeat it', async ({ page, context }) => {
  const email = `demo-import-${Date.now()}@example.test`;
  const registration = await context.request.post('/api/auth/register', { data: { name: 'Проверка истории', email, password: 'test password 12345', password_confirmation: 'test password 12345' } });
  expect(registration.status()).toBe(201);
  const onboarding = await context.request.post('/api/auth/onboarding', { data: {} });
  expect(onboarding.status()).toBe(200);
  await page.goto('/settings');
  const button = page.getByRole('button', { name: 'Загрузить примеры', exact: true });
  await button.click();
  await expect(page.getByRole('status').filter({ hasText: 'Добавлено записей: 1000' })).toBeVisible();
  await expect(button).toBeEnabled();
  await button.click();
  await expect(page.getByRole('status').filter({ hasText: 'Повторных записей не добавлено' })).toBeVisible();
  await page.reload();
  const found = await context.request.get('/api/tasks?q=контракт&limit=10');
  expect(found.status()).toBe(200);
  expect((await found.json()).events.length).toBeGreaterThan(0);
});
