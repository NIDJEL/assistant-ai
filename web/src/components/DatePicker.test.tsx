import { useState } from 'react';
import { fireEvent, render, screen, within } from '@testing-library/react';
import { DatePicker, displayDate, parseDate } from './DatePicker';
import { dayKey } from '../features/calendar/dates';

function Field(props: Partial<React.ComponentProps<typeof DatePicker>> = {}) {
  const [value, setValue] = useState(props.value || '2024-02-28');
  return <form aria-label="Форма"><label>Дата<DatePicker {...props} name="date" value={value} onChange={setValue}/></label><button type="submit">Сохранить</button></form>;
}
it('round-trips civil dates without accepting impossible days or year zero', () => {
  expect(parseDate('29.02.2024')).toBe('2024-02-29');
  expect(parseDate('29022024')).toBe('2024-02-29');
  expect(parseDate('2024-02-29')).toBe('2024-02-29');
  expect(displayDate('0001-01-01')).toBe('01.01.0001');
  for (const value of ['29.02.2023', '31.04.2026', '2026-13-01', '0000-01-01', '20.10.26']) expect(parseDate(value)).toBe('');
});
it('supports keyboard leap-day selection and keeps ISO form submission', () => {
  render(<Field/>); fireEvent.click(screen.getByRole('button', { name: 'Выбрать дату' }));
  const calendar = screen.getByRole('dialog', { name: 'Выбор даты' });
  const day = calendar.querySelector('[data-date="2024-02-28"]')!;
  expect(day).toHaveFocus(); fireEvent.keyDown(day, { key: 'ArrowRight' });
  const leap = calendar.querySelector('[data-date="2024-02-29"]')!; expect(leap).toHaveFocus(); fireEvent.click(leap);
  expect(screen.getByLabelText('Дата')).toHaveValue('29.02.2024');
  expect(new FormData(screen.getByRole('form') as HTMLFormElement).get('date')).toBe('2024-02-29');
});
it('enforces min/max and blocks submission of a partial or invalid typed date', () => {
  render(<Field min="2024-02-28" max="2024-03-02" required/>);
  const field = screen.getByLabelText('Дата') as HTMLInputElement;
  fireEvent.change(field, { target: { value: '31.02.2024' } }); expect(field.checkValidity()).toBe(false);
  fireEvent.change(field, { target: { value: '2024-03-01' } }); expect(field.checkValidity()).toBe(true); expect(field).toHaveValue('01.03.2024');
  fireEvent.click(screen.getByRole('button', { name: 'Выбрать дату' }));
  const calendar = screen.getByRole('dialog'); expect(calendar.querySelector('[data-date="2024-02-27"]')).toBeDisabled(); expect(calendar.querySelector('[data-date="2024-03-03"]')).toBeDisabled();
  fireEvent.keyDown(calendar.querySelector('[data-date="2024-03-01"]')!, { key: 'ArrowDown' }); expect(calendar.querySelector('[data-date="2024-03-02"]')).toHaveFocus();
});
it('selects month and year with Russian Monday-first weeks', () => {
  render(<Field/>); fireEvent.click(screen.getByRole('button', { name: 'Выбрать дату' }));
  const calendar = screen.getByRole('dialog');
  expect(within(calendar).getAllByRole('columnheader').map(item => item.textContent)).toEqual(['Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб', 'Вс']);
  fireEvent.click(within(calendar).getByRole('button', { name: 'Выбрать месяц' })); fireEvent.click(within(calendar).getByRole('button', { name: 'март' }));
  fireEvent.click(within(calendar).getByRole('button', { name: 'Выбрать год' })); fireEvent.click(within(calendar).getByRole('button', { name: /^2025$/ }));
  expect(within(calendar).getByRole('grid')).toHaveAccessibleName(/март 2025/);
  fireEvent.click(calendar.querySelector('[data-date="2025-03-28"]')!); expect(screen.getByLabelText('Дата')).toHaveValue('28.03.2025');
});
it('keeps a keyboard entry point in the day grid after switching months', () => {
  render(<Field value="2024-01-31"/>); fireEvent.click(screen.getByRole('button', { name: 'Выбрать дату' }));
  const calendar = screen.getByRole('dialog'); fireEvent.click(within(calendar).getByRole('button', { name: 'Следующий период выбора даты' }));
  const day = calendar.querySelector('[data-date="2024-02-29"]')!; expect(day).toHaveAttribute('tabindex', '0');
  fireEvent.keyDown(day, { key: 'ArrowRight' }); expect(calendar.querySelector('[data-date="2024-03-01"]')).toHaveFocus();
});
it('calculates Today in the account timezone and restores focus on Escape', async () => {
  render(<Field value="" timezone="Pacific/Kiritimati"/>); fireEvent.click(screen.getByRole('button', { name: 'Выбрать дату' }));
  const calendar = screen.getByRole('dialog'); fireEvent.click(within(calendar).getByRole('button', { name: 'Сегодня' }));
  expect(screen.getByLabelText('Дата')).toHaveValue(displayDate(dayKey(new Date().toISOString(), 'Pacific/Kiritimati')));
  await new Promise(resolve => setTimeout(resolve, 220));
  fireEvent.click(screen.getByRole('button', { name: 'Выбрать дату' })); fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' });
  await new Promise(resolve => setTimeout(resolve, 220)); expect(screen.queryByRole('dialog')).not.toBeInTheDocument(); expect(screen.getByLabelText('Дата')).toHaveFocus();
});
