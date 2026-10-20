import { legalAcceptance } from './legal-acceptance';
import { test, expect } from '@playwright/test';
test.beforeEach(async ({ context }) => {
  expect((await context.request.post('/api/auth/register', { data: { ...legalAcceptance, name: 'Знакомство', email: `onboarding-${crypto.randomUUID()}@example.test`, password: 'test password 12345', password_confirmation: 'test password 12345' } })).status()).toBe(201);
});
test('onboarding connect runs OAuth mock success and returns directly to chat', async ({ page, context }) => {
  let connected = false; const visits: string[] = [];
  page.on('framenavigated', frame => { if (frame === page.mainFrame()) visits.push(frame.url()); });
  await page.route('**/api/integrations', route => route.fulfill({ json: { yandex_configured: true, connections: connected ? [{ provider: 'yandex', status: 'connected' }] : [] } }));
  await page.route('**/api/auth/yandex/start?calendar=1&source=onboarding', route => route.fulfill({ contentType: 'text/html', body: '<!doctype html><title>OAuth mock</title><script>location.assign("/api/auth/yandex/callback?mock=success")</script>' }));
  await page.route('**/api/auth/yandex/callback?mock=success', async route => { connected = true; await context.request.post('/api/auth/onboarding', { data: {} }); await route.fulfill({ status: 303, headers: { Location: '/?calendar=connected' } }); });
  await page.goto('/onboarding'); await page.getByRole('button', { name: 'Начать', exact: true }).click(); await page.getByRole('button', { name: 'Подключить Яндекс Календарь', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Чем помочь?' })).toBeVisible(); expect(new URL(page.url()).pathname).toBe('/'); expect(visits.some(url => url.includes('/integrations'))).toBe(false); expect(connected).toBe(true);
});
test('skip goes directly to chat', async ({ page }) => { await page.goto('/onboarding'); await page.getByRole('button', { name: 'Начать', exact: true }).click(); await page.getByRole('button', { name: 'Сделать позже' }).click(); await expect(page.getByRole('heading', { name: 'Чем помочь?' })).toBeVisible(); expect(new URL(page.url()).pathname).toBe('/'); });
test('already connected calendar completes onboarding without asking again', async ({ page }) => { await page.route('**/api/integrations', route => route.fulfill({ json: { yandex_configured: true, connections: [{ provider: 'yandex', status: 'connected' }] } })); await page.goto('/onboarding'); await expect(page.getByRole('heading', { name: 'Чем помочь?' })).toBeVisible(); await expect(page.getByRole('button', { name: 'Подключить Яндекс Календарь' })).toHaveCount(0); });
test('cancelled OAuth returns to retry or skip in onboarding', async ({ page }) => { await page.goto('/onboarding?error=oauth_cancelled'); await expect(page.getByRole('alert')).toHaveText('Не удалось подключить Яндекс Календарь'); await expect(page.getByRole('button', { name: 'Попробовать ещё раз' })).toBeVisible(); await page.getByRole('button', { name: 'Сделать позже' }).click(); await expect(page.getByRole('heading', { name: 'Чем помочь?' })).toBeVisible(); });
