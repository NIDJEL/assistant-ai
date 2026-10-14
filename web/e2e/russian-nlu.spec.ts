import { test, expect } from './fixtures';
import { ready } from './helpers';
import type { ChatResponse } from '../src/types/api';

for (const text of [
  'Напомни мне что через минуту и нужно снять чайник с плиты',
  'Напомни мне что через минуту нужно снять чайник с плиты',
]) {
  test(`Russian reminder creates immediately: ${text}`, async ({ page, request }) => {
    await ready(page);
    const input = page.getByRole('textbox', { name: 'Сообщение ассистенту' });
    await input.fill(text);
    const sent = page.waitForResponse(r => r.url().endsWith('/api/chat') && r.request().method() === 'POST');
    await input.press('Enter');
    const response = await (await sent).json() as ChatResponse;
    expect(response).toMatchObject({ action: 'create', pending: false, provider: 'rules' });
    expect(response.events).toHaveLength(1);
    const event = response.events[0];
    expect(event).toMatchObject({ kind: 'reminder', title: 'Снять чайник с плиты' });
    expect(event.date).toMatch(/^\d{4}-\d{2}-\d{2}$/);
    expect(event.time).toMatch(/^\d{2}:\d{2}$/);
    expect(await (await request.get(`/api/events/${event.id}`)).json()).toMatchObject({ ...event });
    await expect(page.locator('.reply-text').last()).toContainText('Напоминание сохранено');
  });
}

test('missing reminder time asks the reminder question and accepts a relative reply', async ({ page }) => {
  await ready(page);
  const input = page.getByRole('textbox', { name: 'Сообщение ассистенту' });
  const send = async (text: string) => {
    await input.fill(text);
    const sent = page.waitForResponse(r => r.url().endsWith('/api/chat') && r.request().method() === 'POST');
    await input.press('Enter');
    return await (await sent).json() as ChatResponse;
  };
  expect(await send('Напомни купить молоко')).toMatchObject({ action: 'clarify', pending: true, reply: 'Когда напомнить?', events: [] });
  await page.reload();
  const response = await send('через минуту');
  expect(response).toMatchObject({ action: 'create', pending: false });
  expect(response.events).toHaveLength(1);
  expect(response.events[0]).toMatchObject({ kind: 'reminder', title: 'Купить молоко' });
});
