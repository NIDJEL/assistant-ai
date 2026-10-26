import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ActionSteps } from './ActionSteps';
import type { ActionResult } from '../../types/api';

const steps: ActionResult[] = [
  { key: 'meeting', status: 'waiting', reply: 'Во сколько встреча?', active: true, control_token: 'persisted-token' },
  { key: 'reminder', status: 'waiting', reply: 'Во сколько напомнить?', control_token: 'persisted-token' },
  { key: 'task', status: 'done', reply: 'Продукты добавлены' },
];

test('focus and cancel submit the persisted token and exact step key', async () => {
  const onChoose = vi.fn();
  render(<ActionSteps steps={steps} active busy={false} onChoose={onChoose}/>);
  await userEvent.setup().click(screen.getByRole('button', { name: 'Уточнить это действие' }));
  expect(onChoose).toHaveBeenLastCalledWith('Уточнить действие 2', { token: 'persisted-token', key: 'reminder', action: 'action_focus' });
  await userEvent.setup().click(screen.getAllByRole('button', { name: 'Отменить это действие' })[1]);
  expect(onChoose).toHaveBeenLastCalledWith('Отменить действие 2', { token: 'persisted-token', key: 'reminder', action: 'action_cancel' });
});

test('old responses have no action controls and busy controls are disabled', () => {
  const { rerender } = render(<ActionSteps steps={steps} active={false} busy={false} onChoose={vi.fn()}/>);
  expect(screen.queryByRole('button')).toBeNull();
  rerender(<ActionSteps steps={steps} active busy onChoose={vi.fn()}/>);
  for (const button of screen.getAllByRole('button')) expect(button).toBeDisabled();
});

test('atomic preview explicitly confirms or cancels the whole group', () => {
  render(<ActionSteps steps={[{ ...steps[0], atomic_group: 'all', can_confirm: true }]} active busy={false} onChoose={vi.fn()}/>);
  expect(screen.getByRole('button', { name: 'Подтвердить всю группу' })).toBeEnabled();
  expect(screen.getByRole('button', { name: 'Отменить группу' })).toBeEnabled();
});
