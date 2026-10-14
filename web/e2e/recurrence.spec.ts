import { test, expect } from './fixtures';
import { ready } from './helpers';
import type { ChatResponse, RhythmEvent } from '../src/types/api';

for (const [text, title, reply, frequency] of [
  ['через час и 20 минут напомни проверить духовку', 'Проверить духовку', '', ''],
  ['через два дня и три часа напомни проверить сервер', 'Проверить сервер', '', ''],
  ['через месяц напомни оплатить подписку', 'Оплатить подписку', '', ''],
  ['каждый день в 9 напомни проверить почту', 'Проверить почту', '', 'day'],
  ['по будням в 8:30 напомни выйти из дома', 'Выйти из дома', '', 'week'],
  ['каждые два часа напомни размяться', 'Размяться', '', 'hour'],
  ['15 числа каждого месяца напомни оплатить интернет', 'Оплатить интернет', 'в 9', 'month'],
]) {
  test(`extended NLU creation, API and reload: ${text}`, async ({ page, request }) => {
    await ready(page);
    const send = async (message: string) => {
      const input = page.getByRole('textbox', { name: 'Сообщение ассистенту' });
      await input.fill(message);
      const sent = page.waitForResponse(r => r.url().endsWith('/api/chat') && r.request().method() === 'POST');
      await input.press('Enter');
      return await (await sent).json() as ChatResponse;
    };
    let result = await send(text);
    if (reply) { expect(result).toMatchObject({ pending: true, reply: 'Во сколько напомнить?' }); await page.reload(); result = await send(reply); }
    expect(result).toMatchObject({ action: 'create', pending: false, provider: 'rules' });
    expect(result.events).toHaveLength(1);
    const event = result.events[0]; expect(event.title).toBe(title);
    expect(event.recurrence?.frequency || '').toBe(frequency);
    expect(await (await request.get(`/api/events/${event.id}`)).json()).toMatchObject({ ...event });
    await page.reload(); await expect(page.locator('.response-events').getByText(title, { exact: true }).last()).toBeVisible();
    if (frequency) {
      await page.locator('.response-events').getByText(title, { exact: true }).last().click();
      await expect(page.getByRole('combobox', { name: 'Повтор напоминания' })).toBeEnabled();
      await page.getByRole('button', { name: 'Сохранить', exact: true }).click();
      await expect(page.getByRole('dialog')).toHaveCount(0);
      const saved = await (await request.get(`/api/events/${event.id}`)).json() as RhythmEvent;
      expect(saved.recurrence).toEqual(event.recurrence);
    }
  });
}

test('recurring record supports API update, complete, delete and restore', async ({ page, request }) => {
  const created = await request.post('/api/events', { data: { kind: 'reminder', title: 'Проверить воду', date: '2026-12-15', time: '09:00', recurrence: { frequency: 'day', interval: 1, local_time: '09:00', timezone: 'Europe/Moscow' } } });
  expect(created.status()).toBe(201); const event = await created.json() as RhythmEvent;
  const path = `/api/events/${event.id}`;
  const done = await request.patch(path, { data: { status: 'done' } }); expect((await done.json()).recurrence).toEqual(event.recurrence);
  const resumed = await request.patch(path, { data: { status: 'open' } }); expect((await resumed.json()).recurrence).toEqual(event.recurrence);
  expect((await request.delete(path)).status()).toBe(204); expect((await request.get(path)).status()).toBe(404);
  const restored = await request.post(`${path}/restore`, { data: {} }); expect(restored.status()).toBe(200); expect((await restored.json()).recurrence).toEqual(event.recurrence);
  await ready(page, '/reminders'); await page.getByRole('button', { name: 'Повторяющиеся', exact: true }).click(); await expect(page.getByRole('button', { name: 'Проверить воду', exact: true })).toBeVisible();
  const removedRule = await request.patch(path, { data: { recurrence: null } }); expect((await removedRule.json()).recurrence).toBeUndefined();
});

test('recurring task keeps explicit intent and shows its rule in tasks and calendar', async ({ page, request }) => {
  const response = await request.post('/api/chat', { data: { message: 'создай задачу каждый день в 9 проверить встречи', session_id: crypto.randomUUID(), request_id: crypto.randomUUID() } });
  const result = await response.json() as ChatResponse;
  expect(result).toMatchObject({ action: 'create', pending: false });
  expect(result.events[0]).toMatchObject({ kind: 'task', title: 'Проверить встречи' });
  const event = result.events[0];
  await ready(page, '/tasks');
  const card = page.locator('.task-card').filter({ hasText: 'Проверить встречи' });
  await expect(card).toContainText('Каждый день, 09:00');
  await ready(page, `/calendar?date=${event.date}`);
  await page.getByRole('button', { name: 'День', exact: true }).click();
  await expect(page.locator('.day-event').filter({ hasText: 'Проверить встречи' })).toContainText('Каждый день, 09:00');
});
