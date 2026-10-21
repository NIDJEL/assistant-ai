import { assistantReplyText } from '../src/features/assistant/replyText';
import { test, expect, type Page } from './fixtures';
import { ready, screenshot, noOverflow } from './helpers';
import { addDays, dayKey } from '../src/features/calendar/dates';
import type { ChatResponse, Health } from '../src/types/api';

async function send(page: Page, message: string) {
  const draft = page.getByRole('textbox', { name: 'Сообщение ассистенту' });
  await draft.fill(message);
  const result = page.waitForResponse(r => r.url().endsWith('/api/chat') && r.request().method() === 'POST');
  await draft.press('Enter');
  const response = await (await result).json() as ChatResponse;
  await expect(page.locator('.reply-text').last()).toHaveText(assistantReplyText(response));
  return response;
}

test('delete all tomorrow previews the actual date scope and requires confirmation after reload', async ({ page, request }, info) => {
  const health = await (await request.get('/health')).json() as Health;
  const tomorrow = addDays(dayKey(health.now, health.timezone), 1), after = addDays(tomorrow, 1);
  const prior = await (await request.get(`/api/events?start=${tomorrow}&end=${after}&limit=100`)).json();
  for (const [title, kind, time] of [['Пара для проверки выбора', 'reminder', '12:50'], ['Забрать заказ для проверки выбора', 'task', null]]) {
    const created = await request.post('/api/events', { data: { title, kind, date: tomorrow, time } }); expect(created.status()).toBe(201);
  }
  const keep = await (await request.post('/api/events', { data: { title: 'Пара на другой день', kind: 'reminder', date: after, time: '12:50' } })).json();
  await ready(page);
  const question = await send(page, 'Удали пожалуйста все записи на завтра');
  expect(question.action).toBe('clarify'); expect(question.pending).toBe(true); expect(question.total).toBe(prior.total + 2);
  expect(question.events.every(event => event.date === tomorrow)).toBe(true);
  expect((await (await request.get(`/api/events?start=${tomorrow}&end=${after}&limit=100`)).json()).total).toBe(prior.total + 2);
  await screenshot(page, 'delete-tomorrow-preview', info);
  await page.reload(); await expect(page.getByRole('button', { name: 'Да', exact: true })).toBeVisible();
  const late = await (await request.post('/api/events', { data: { title: 'Появилась после вопроса', kind: 'task', date: tomorrow } })).json();
  const done = await send(page, 'Да, удали все пожалуйста');
  expect(done.action).toBe('delete'); expect(done.total).toBe(prior.total + 2); expect(done.pending).toBe(false);
  expect((await request.get(`/api/events/${keep.id}`)).status()).toBe(200);
  expect((await request.get(`/api/events/${late.id}`)).status()).toBe(200);
  await expect(page.getByRole('button', { name: 'Да', exact: true })).toHaveCount(0);
});

test('duplicate names can be distinguished by a spoken date or by labelled buttons without remembering IDs', async ({ page, request }, info) => {
  const health = await (await request.get('/health')).json() as Health;
  const tomorrow = addDays(dayKey(health.now, health.timezone), 1), after = addDays(tomorrow, 1);
  const records = [];
  for (const date of [tomorrow, after]) {
    records.push(await (await request.post('/api/events', { data: { title: 'Позвонить Алисе по билету', kind: 'reminder', date, time: '15:00' } })).json());
  }
  await ready(page);
  const question = await send(page, 'Удали напоминание позвонить Алисе по билету');
  expect(question.candidates).toHaveLength(2); expect(question.pending).toBe(true);
  const choices = page.getByLabel('Выбор записи'); await expect(choices.getByRole('button')).toHaveCount(2);
  await expect(choices).toContainText('Позвонить Алисе по билету'); await expect(choices).toContainText('15:00');
  expect(await choices.innerText()).not.toContain('#');
  await noOverflow(page); await screenshot(page, 'choose-name-and-date', info);
  await page.reload();
  const chosen = await send(page, 'ту, что завтра в 15:00');
  expect(chosen.reply).toContain(`Удалить #${records[0].id}`); expect(chosen.action).toBe('clarify');
  const done = await send(page, 'Да'); expect(done.action).toBe('delete');
  expect((await request.get(`/api/events/${records[0].id}`)).status()).toBe(404);
  expect((await request.get(`/api/events/${records[1].id}`)).status()).toBe(200);
  // A fresh request still offers the remaining record as a safe stable button.
  await send(page, 'Удали напоминание позвонить Алисе');
  await expect(page.getByRole('button', { name: 'Да', exact: true })).toHaveCount(1);
});

test('a changed record invalidates deletion confirmation instead of deleting different content', async ({ page, request }) => {
  const created = await (await request.post('/api/events', { data: { title: 'Проверить договор с Василием', kind: 'task' } })).json();
  await ready(page);
  const question = await send(page, 'Удали дело проверить договор с Василием'); expect(question.pending).toBe(true);
  expect((await request.patch(`/api/events/${created.id}`, { data: { title: 'Согласовать новую версию договора' } })).status()).toBe(200);
  const done = await send(page, 'Да'); expect(done.action).toBe('clarify'); expect(done.pending).toBe(false); expect(done.reply).toContain('Ничего не удалено');
  expect((await request.get(`/api/events/${created.id}`)).status()).toBe(200);
});
