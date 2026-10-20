import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { SchedulingCard, planningSlotLabel } from './SchedulingCard';
import type { SchedulingCard as Card } from '../../types/api';
const card: Card = { token: 'offer-token', kind: 'slots', title: 'Встреча с другом', complete: true, timezone: 'Europe/Moscow', today: '2026-10-20', slots: [{ start: '2026-10-20T14:00:00Z', end: '2026-10-20T15:00:00Z' }, { start: '2026-10-21T08:00:00Z', end: '2026-10-21T09:00:00Z' }] };
test('time choice submits the exact structured offer without drafting text', async () => {
  const onChoose = vi.fn(); render(<SchedulingCard card={card} active busy={false} onChoose={onChoose}/>);
  await userEvent.setup().click(screen.getByRole('button', { name: 'Завтра, 11:00–12:00' }));
  expect(onChoose).toHaveBeenCalledWith('Завтра, 11:00–12:00', { token: card.token, action: 'slot', index: 2, slot: card.slots![1] });
});
test('stale and busy cards disable actions and expose partial availability', () => {
  const { rerender } = render(<SchedulingCard card={{ ...card, complete: false }} active busy onChoose={vi.fn()}/>);
  expect(screen.getByRole('status')).toHaveTextContent('не полностью');
  for (const button of screen.getAllByRole('button')) expect(button).toBeDisabled();
  rerender(<SchedulingCard card={card} active={false} busy={false} onChoose={vi.fn()}/>);
  for (const button of screen.getAllByRole('button')) expect(button).toBeDisabled();
});
test('relative labels use the offer timezone across the device midnight', () => {
  expect(planningSlotLabel({ start: '2026-10-20T22:00:00Z', end: '2026-10-20T23:00:00Z' }, 'Europe/Moscow', '2026-10-20')).toBe('Завтра, 01:00–02:00');
});
test('availability preview confirms invitations separately', async () => {
  const onChoose = vi.fn(); render(<SchedulingCard card={{ ...card, kind: 'confirm', slots: [] }} active busy={false} onChoose={onChoose}/>);
  expect(screen.queryByRole('button', { name: 'Создать отдельную' })).toBeNull();
  await userEvent.setup().click(screen.getByRole('button', { name: 'Отправить приглашения' }));
  expect(onChoose).toHaveBeenCalledWith('Подтверждаю создание встречи и отправку приглашений', { token: card.token, action: 'confirm', index: undefined, slot: undefined });
});
