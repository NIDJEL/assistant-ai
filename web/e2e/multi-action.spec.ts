import { test, expect } from './fixtures';
import { ready, noOverflow } from './helpers';
import type { ChatInput } from '../src/types/api';

test('multi-action controls fit phones and send a chosen step with the keyboard', async ({ page }, info) => {
  const submissions: ChatInput[] = [];
  await page.route('**/api/chat', async route => {
    const body = route.request().postDataJSON() as ChatInput;
    submissions.push(body);
    await route.fulfill({ json: {
      reply: 'Выполнено одно действие. Уточните время встречи или напоминания.', action: 'create', pending: true,
      events: [], candidates: null, conflicts: null, session_id: body.session_id, request_id: body.request_id,
      provider: 'simple_llm', timezone: 'Europe/Moscow',
      operations: [
        { key: 'meeting', status: 'waiting', reply: 'Встреча с Колей: вы имеете в виду 08:00 или 20:00?', active: !body.choice, control_token: '00000000-0000-4000-8000-000000000001' },
        { key: 'task', status: 'done', reply: 'Задача «Купить продукты» сохранена.' },
        { key: 'reminder', status: 'waiting', reply: 'Позвонить маме: 10:00 или 22:00?', active: !!body.choice, control_token: '00000000-0000-4000-8000-000000000001' },
      ],
    } });
  });
  await ready(page);
  await page.getByRole('textbox', { name: 'Сообщение ассистенту' }).fill('Создай встречу с Колей в 8, купи продукты и напомни позвонить маме в 10');
  await page.getByRole('button', { name: 'Отправить сообщение' }).click();
  const card = page.getByRole('region', { name: 'Действия из запроса' }).last();
  await expect(card.getByRole('listitem')).toHaveCount(3);
  for (const width of [1440, 390, 320]) {
    await page.setViewportSize({ width, height: 844 });
    await noOverflow(page);
    await expect(card.getByRole('button', { name: 'Уточнить это действие' })).toBeVisible();
    if (width === 390) await page.screenshot({ path: info.outputPath('multi-action-390.png') });
  }
  const focus = card.getByRole('button', { name: 'Уточнить это действие' });
  await focus.focus(); await page.keyboard.press('Enter');
  await expect.poll(() => submissions.length).toBe(2);
  expect(submissions[1].choice).toEqual({ token: '00000000-0000-4000-8000-000000000001', key: 'reminder', action: 'action_focus' });
  await expect(page.getByRole('region', { name: 'Действия из запроса' }).first().getByRole('button')).toHaveCount(0);
});
