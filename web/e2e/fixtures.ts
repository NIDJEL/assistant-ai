import { test as base, expect } from '@playwright/test';
export { expect };
export type { Page, TestInfo } from '@playwright/test';
// Existing product flows now run with a real, isolated PostgreSQL account.
export const test = base.extend<{ signedIn: void }>({
 request: async ({ context }, use) => { await use(context.request); },
 signedIn: [async ({ context }, use, info) => {
  const email = `p12-${Date.now()}-${info.testId.replace(/[^a-z0-9]/gi, '').slice(-15)}@example.test`;
  const response = await context.request.post('/api/auth/register', { data: { name: 'Тест Гена', email, password: 'test password 12345', password_confirmation: 'test password 12345' } });
  expect(response.status()).toBe(201);
  const done = await context.request.post('/api/auth/onboarding', { data: {} }); expect(done.status()).toBe(200);
  const imported = await context.request.post('/api/import-demo', { data: {} }); expect(imported.status()).toBe(200);
  await use();
 }, { auto: true }],
});
