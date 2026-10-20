import { legalAcceptance } from './legal-acceptance';
import { test, expect } from '@playwright/test';
import { randomUUID } from 'node:crypto';
import { noOverflow } from './helpers';
test('live simple_llm conflict card survives reload and sends an exact slot', async ({ browser }, info) => {
  test.skip(process.env.LIVE_SIMPLE_LLM !== '1', 'Explicit live GigaChat test server required');
  test.setTimeout(90000);
  const baseURL = process.env.P1_BASE_URL!;
  const a = await browser.newContext({ baseURL }); const b = await browser.newContext({ baseURL });
  const register = async (context: typeof a, name: string) => { const password = randomUUID(); const r = await context.request.post('/api/auth/register', { data: { ...legalAcceptance, name, email: `scheduling-ui-${randomUUID()}@example.test`, password, password_confirmation: password } }); expect(r.status()).toBe(201); const user = (await r.json()).user; expect((await context.request.post('/api/auth/onboarding', { data: {} })).status()).toBe(200); await context.request.put(`/api/preferences/${user.id}`, { data: { widgets: false, revision: 1 } }); return user; };
  const op = async (context: typeof a, action: string, fields: object) => { const r = await context.request.post('/api/social/operations', { data: { action, request_id: randomUUID(), ...fields } }); expect(r.status()).toBe(200); return r.json(); };
  try {
    const au = await register(a, 'Алиса Проверка'); const bu = await register(b, 'Борис Проверка');
    const people = await (await a.request.get('/api/social/people')).json(); const request = await op(b, 'request', { code: people.profile.code }); await op(a, 'accept_request', { id: request.id });
    const health = await (await a.request.get('/health')).json(); const day = new Date(`${health.now.slice(0, 10)}T12:00:00Z`); day.setUTCDate(day.getUTCDate() + 1); const tomorrow = day.toISOString().slice(0, 10);
    expect((await b.request.post('/api/events', { data: { kind: 'meeting', title: 'PRIVATE EVENT', date: tomorrow, time: '16:00', duration_minutes: 60 } })).status()).toBe(201);
    const page = await a.newPage(); await page.goto('/'); await page.getByRole('textbox', { name: 'Сообщение ассистенту' }).fill('Назначь встречу с Борисом завтра в 16:00'); await page.getByRole('button', { name: 'Отправить сообщение' }).click();
    let card = page.getByRole('region', { name: 'Выбор времени встречи' });
    // section has an accessible name and therefore a region landmark.
    await expect(card).toBeVisible(); await expect(card).not.toContainText('PRIVATE');
    await noOverflow(page); await page.screenshot({ path: info.outputPath('scheduling-desktop.png'), fullPage: true });
    await page.reload(); card = page.getByRole('region', { name: 'Выбор времени встречи' }); await expect(card).toBeVisible();
    await page.setViewportSize({ width: 390, height: 844 }); await noOverflow(page); await card.scrollIntoViewIfNeeded(); await page.screenshot({ path: info.outputPath('scheduling-mobile.png'), fullPage: true });
    const chosen = page.waitForRequest(r => r.url().endsWith('/api/chat') && r.method() === 'POST');
    await card.getByRole('button', { name: /^Завтра, 17:00/ }).click(); const payload = (await chosen).postDataJSON();
    expect(payload.choice).toMatchObject({ action: 'slot', index: 1 }); expect(payload.choice.slot.start).toBeTruthy();
    await expect(page.locator('.reply-text').last()).toContainText('приглашения');
    const meetings = (await (await a.request.get('/api/social/meetings')).json()).meetings; expect(meetings).toHaveLength(1); expect(meetings[0].organizer.id).toBe(au.id); expect(meetings[0].members[0]).toMatchObject({ id: bu.id, status: 'pending' }); expect(meetings[0].event.time).toBe('17:00');
    for (const button of await card.getByRole('button').all()) await expect(button).toBeDisabled(); await noOverflow(page);
    await page.getByRole('textbox', { name: 'Сообщение ассистенту' }).fill('Когда мы с Борисом оба свободны послезавтра?'); await page.getByRole('button', { name: 'Отправить сообщение' }).click();
    const preview = page.getByRole('region', { name: 'Выбор времени встречи' }).last(); await expect(preview.getByRole('button').first()).toBeEnabled(); await preview.getByRole('button').first().click();
    const confirmation = page.getByRole('region', { name: 'Подтверждение встречи' }); await expect(confirmation).toBeVisible();
    expect((await (await a.request.get('/api/social/meetings')).json()).meetings).toHaveLength(1);
    const invited = page.waitForResponse(r => r.url().endsWith('/api/chat') && r.request().method() === 'POST' && r.request().postDataJSON().choice?.action === 'confirm');
    await confirmation.getByRole('button', { name: 'Отправить приглашения' }).click(); const invitationResponse = await invited;
    expect(invitationResponse.status()).toBe(200); expect((await invitationResponse.json()).action).toBe('create');
    expect((await (await a.request.get('/api/social/meetings')).json()).meetings).toHaveLength(2);
  } finally { await a.close(); await b.close(); }
});
