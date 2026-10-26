import { test, expect } from '@playwright/test';
import { mockSchedule } from './mock-workspace';

test('reported time choices send once immediately and preserve an unrelated draft', async ({ page }) => {
  await mockSchedule(page);
  const messages: string[] = [];
  await page.route('**/api/chat', async route => {
    const input = route.request().postDataJSON(); messages.push(input.message);
    await route.fulfill({ json: { reply: messages.length === 1 ? 'Вы имеете в виду 11:00 или 23:00?' : 'Встреча сохранена.', action: messages.length === 1 ? 'clarify' : 'create', pending: messages.length === 1, suggestions: ['11:00', '23:00'], events: [], candidates: [], conflicts: [], session_id: input.session_id, request_id: input.request_id, provider: 'simple_llm', timezone: 'Europe/Moscow' } });
  });
  await page.goto('/');
  const input = page.getByRole('textbox', { name: 'Сообщение ассистенту' });
  await input.fill('В четверг в 11 у меня встреча с дизайнером');
  await page.getByRole('button', { name: 'Отправить сообщение', exact: true }).click();
  await expect(page.getByRole('button', { name: '11:00', exact: true })).toBeVisible();
  await input.fill('Мой следующий запрос');
  await page.getByRole('button', { name: '11:00', exact: true }).click();
  await expect.poll(() => messages).toHaveLength(2);
  expect(messages[1]).toBe('11:00');
  await expect(page.getByText('Встреча сохранена.', { exact: true })).toBeVisible();
  await expect(input).toHaveValue('Мой следующий запрос');
});

test('a greeting does not show cancellation controls for an unrelated pending command', async ({ page }) => {
  await mockSchedule(page);
  await page.route('**/api/chat', async route => {
    const input = route.request().postDataJSON();
    await route.fulfill({ json: { reply: 'Привет! Прогноз погоды пока недоступен.', action: 'help', pending: true, events: [], candidates: [], conflicts: [], session_id: input.session_id, request_id: input.request_id, provider: 'simple_llm', timezone: 'Europe/Moscow' } });
  });
  await page.goto('/');
  await page.getByRole('textbox', { name: 'Сообщение ассистенту' }).fill('привет, как дела? какая завтра погода?');
  await page.getByRole('button', { name: 'Отправить сообщение', exact: true }).click();
  await expect(page.getByText('Привет! Прогноз погоды пока недоступен.')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Отмена', exact: true })).toHaveCount(0);
});

test('all ten weekly entries reach speech even while only four cards are expanded', async ({ page }) => {
  const events = (await mockSchedule(page)).slice(0, 10);
  let spoken = '';
  await page.route('**/api/chat', async route => {
    const input = route.request().postDataJSON();
    await route.fulfill({ json: { reply: 'На этой неделе — 10 дел:\n' + events.map(e => `#${e.id} «${e.title}» — ${e.date} в ${e.time}`).join('\n'), action: 'list', pending: false, events, candidates: [], conflicts: [], session_id: input.session_id, request_id: input.request_id, provider: 'simple_llm', timezone: 'Europe/Moscow' } });
  });
  await page.route('**/api/voice/synthesize', async route => { spoken = route.request().postDataJSON().text; await route.fulfill({ status: 503, json: { error: { message: 'Controlled speech transport' } } }); });
  await page.goto('/');
  await page.getByRole('textbox', { name: 'Сообщение ассистенту' }).fill('Что запланировано на эту неделю?');
  await page.getByRole('button', { name: 'Отправить сообщение', exact: true }).click();
  await expect(page.locator('.chat-event-card')).toHaveCount(4);
  await expect(page.locator('.reply-text')).toHaveText('На этой неделе — 10 дел:');
  await page.getByRole('button', { name: 'Озвучить ответ' }).click();
  await expect.poll(() => spoken).toContain('Встреча 10');
  await page.getByRole('button', { name: 'Ещё 6', exact: true }).click();
  await expect(page.locator('.chat-event-card')).toHaveCount(10);
});
