import { test, expect } from './fixtures';
import { noOverflow, ready, screenshot } from './helpers';

test('Yandex calendar offers explicit permission and handles connected filters', async ({ page }) => {
  await page.route('**/api/integrations', route => route.fulfill({ json: { connections: [], yandex_configured: true } }));
  await ready(page, '/integrations?service=yandex');
  const detail = page.getByRole('complementary', { name: 'Интеграция', exact: true });
  await expect(detail.getByRole('link', { name: 'Подключить Яндекс Календарь' })).toHaveAttribute('href', '/api/auth/yandex/start?calendar=1&source=integrations');
  await expect(detail.getByRole('button', { name: 'Синхронизировать сейчас' })).toBeDisabled();
  await expect(detail).toContainText('Все дела с датой, включая уже созданные');
  await page.getByRole('group', { name: 'Фильтр интеграций' }).getByRole('button', { name: 'Подключено', exact: true }).click();
  await expect(page.getByText('Подключённых интеграций пока нет')).toBeVisible();
  await noOverflow(page);
});

test('Yandex calendar retries failed sync and disconnects with current status', async ({ page }, info) => {
  let connection = { provider: 'yandex', status: 'error', last_sync: null as string | null, last_error: 'Изменения сохранены и будут отправлены повторно.', pending_events: 2 };
  const state = () => ({ connections: [connection], yandex_configured: true });
  const writes: string[] = [];
  await page.route('**/api/integrations**', async route => {
    const request = route.request();
    if (request.method() === 'POST') {
      writes.push(request.url());
      expect(request.postDataJSON()).toEqual({});
      connection = { ...connection, status: 'connected', last_sync: '2026-10-14T09:00:00Z', last_error: '', pending_events: 0 };
    }
    if (request.method() === 'DELETE') { writes.push(request.url()); connection = { ...connection, status: 'disconnected' }; await route.fulfill({ status: 204 }); return; }
    await route.fulfill({ json: state() });
  });
  await ready(page, '/integrations?service=yandex');
  const detail = page.getByRole('complementary', { name: 'Интеграция', exact: true });
  await expect(detail).toContainText('Ожидают отправки: 2');
  await detail.getByRole('button', { name: 'Синхронизировать сейчас' }).click();
  await expect(detail).toContainText('Автоматическая синхронизация');
  await expect(detail).toContainText('Последняя синхронизация:');
  await expect(detail).not.toContainText('Ожидают отправки:');
  await page.getByRole('group', { name: 'Фильтр интеграций' }).getByRole('button', { name: 'Подключено', exact: true }).click();
  await expect(page.locator('.integration-card')).toHaveCount(1);
  await noOverflow(page); await screenshot(page, 'yandex-calendar-connected', info);
  await detail.getByRole('button', { name: 'Отключить интеграцию' }).click();
  await expect(detail).toContainText('Не подключено');
  await expect(detail.getByRole('link', { name: 'Подключить Яндекс Календарь' })).toBeVisible();
  await expect(page.getByText('Подключённых интеграций пока нет')).toBeVisible();
  expect(writes).toHaveLength(2);
});

test('Yandex consent errors and reconnect remain usable on mobile', async ({ page }, info) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.route('**/api/integrations', route => route.fulfill({ json: { connections: [{ provider: 'yandex', status: 'reauthorization_required', last_sync: null, last_error: 'Подключите календарь снова.', pending_events: 1 }], yandex_configured: true } }));
  await ready(page, '/integrations?service=yandex&error=calendar_permission');
  await page.getByRole('button', { name: 'Открыть виджеты' }).click();
  const detail = page.getByRole('dialog', { name: 'Интеграция', exact: true });
  await expect(detail.getByRole('alert')).toHaveText('Разрешите доступ к календарю на странице Яндекса.');
  await expect(detail.getByRole('link', { name: 'Подключить заново' })).toBeVisible();
  await expect(detail.getByRole('link', { name: 'Подключить заново' })).toBeInViewport();
  await expect(detail.getByRole('button', { name: 'Синхронизировать сейчас' })).toBeDisabled();
  await noOverflow(page); await screenshot(page, 'yandex-calendar-mobile', info);
});
