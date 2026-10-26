import { test, expect } from './fixtures';
import { ready } from './helpers';

test('calendar shows future meeting occurrences and edits only the selected one', async ({ page, request }) => {
  await ready(page);
  const created = await request.post('/api/events', { data: { kind: 'meeting', title: 'Обсуждение серии качества', date: '2026-12-01', time: '09:00', duration_minutes: 30, recurrence: { frequency: 'week', interval: 1, weekdays: [2], local_time: '09:00', timezone: 'Europe/Moscow', count: 3 } } });
  expect(created.status()).toBe(201); const master = await created.json();
  await page.goto('/calendar?date=2026-12-08');
  const agenda = page.getByRole('region', { name: 'События выбранного дня' });
  await expect(agenda.getByText('Обсуждение серии качества')).toBeVisible();
  await agenda.getByText('Обсуждение серии качества').click();
  const editor = page.getByRole('dialog');
  await expect(editor.getByText(/Изменяется только повторение/)).toBeVisible();
  await editor.locator('input[name="time"]').fill('11:00');
  const saved = page.waitForResponse(r => r.url().endsWith(`/api/events/${master.id}`) && r.request().method() === 'PATCH');
  await editor.getByRole('button', { name: 'Сохранить', exact: true }).click();
  expect((await saved).status()).toBe(200);
  await expect(editor).not.toBeVisible();
  await expect(agenda.getByText(/11:00/)).toBeVisible();
  const response = await request.get('/api/events?start=2026-12-01&end=2026-12-22&expand=true&kind=meeting');
  const rows = (await response.json()).events.filter((e: { title: string }) => e.title === master.title);
  expect(rows.map((e: { date: string; time: string }) => [e.date, e.time])).toEqual([['2026-12-01', '09:00'], ['2026-12-08', '11:00'], ['2026-12-15', '09:00']]);
});
