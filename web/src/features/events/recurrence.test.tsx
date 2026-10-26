import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { RecurrenceEditor } from './RecurrenceEditor';
import { recurrenceLabel } from './recurrence';

test('bounded recurrence labels expose the count and inclusive end day', () => {
  expect(recurrenceLabel({ frequency: 'day', interval: 1, timezone: 'Europe/Moscow', count: 3 })).toBe('Каждый день, всего 3');
  expect(recurrenceLabel({ frequency: 'day', interval: 1, timezone: 'Europe/Moscow', until: '2026-10-31' })).toBe('Каждый день, до 31.10.2026 включительно');
});
import type { Recurrence } from '../../types/api';

it.each([
  [{ frequency: 'day', interval: 1, local_time: '09:00', timezone: 'Europe/Moscow' }, 'Каждый день, 09:00'],
  [{ frequency: 'week', interval: 1, weekdays: [1, 2, 3, 4, 5], local_time: '08:30', timezone: 'Europe/Moscow' }, 'По будням, 08:30'],
  [{ frequency: 'week', interval: 1, weekdays: [0, 6], local_time: '11:00', timezone: 'Europe/Moscow' }, 'По выходным, 11:00'],
  [{ frequency: 'hour', interval: 2, timezone: 'Europe/Moscow' }, 'Каждые 2 ч'],
  [{ frequency: 'month', interval: 1, day_of_month: 15, timezone: 'Europe/Moscow' }, '15 числа каждого месяца'],
] as [Recurrence, string][])('formats structured recurrence %j', (rule, label) => expect(recurrenceLabel(rule)).toBe(label));

it('preserves a custom rule until the user changes it and permits removing it', async () => {
  const user = userEvent.setup(); const change = vi.fn();
  const rule: Recurrence = { frequency: 'week', interval: 2, weekdays: [2, 4], local_time: '18:00', timezone: 'Europe/Moscow' };
  render(<RecurrenceEditor value={rule} date="2026-10-14" time="18:00" timezone="Europe/Moscow" onChange={change}/>);
  expect(change).not.toHaveBeenCalled(); expect(screen.getByRole('combobox')).toHaveTextContent(recurrenceLabel(rule));
  await user.click(screen.getByRole('combobox')); await user.click(screen.getByRole('option', { name: 'Без повтора' })); expect(change).toHaveBeenLastCalledWith(null);
  await user.click(screen.getByRole('combobox')); await user.click(screen.getByRole('option', { name: 'По будням' })); expect(change).toHaveBeenLastCalledWith({ frequency: 'week', interval: 1, weekdays: [1, 2, 3, 4, 5], local_time: '18:00', timezone: 'Europe/Moscow' });
});
