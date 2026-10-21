import { useState } from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Select } from './Select';

const options = [{ value: '', label: 'Все' }, { value: 'high', label: 'Высокий' }, { value: 'disabled', label: 'Недоступный', disabled: true }, { value: 'normal', label: 'Обычный' }, { value: 'low', label: 'Низкий' }];
function Field({ change = () => {}, required = false }: { change?: (value: string) => void; required?: boolean }) {
  const [value, setValue] = useState('');
  return <form aria-label="Форма"><Select aria-label="Приоритет" name="priority" value={value} options={options} required={required} onChange={value => { setValue(value); change(value); }}/><button type="button">Дальше</button></form>;
}
it('selects with the keyboard, skips disabled choices and submits the actual value', async () => {
  const user = userEvent.setup(); const change = vi.fn(); render(<Field change={change}/>);
  const field = screen.getByRole('combobox'); await user.tab(); expect(field).toHaveFocus();
  await user.keyboard('{ArrowDown}{ArrowDown}{ArrowDown}{Enter}');
  expect(field).toHaveTextContent('Обычный'); expect(change).toHaveBeenCalledExactlyOnceWith('normal');
  expect(new FormData(screen.getByRole('form') as HTMLFormElement).get('priority')).toBe('normal');
  await waitFor(() => expect(screen.queryByRole('listbox')).not.toBeInTheDocument());
  expect(field).toHaveFocus();
});
it('supports type ahead and Escape without changing the saved selection', async () => {
  const user = userEvent.setup(); const change = vi.fn(); render(<Field change={change}/>);
  const field = screen.getByRole('combobox'); await user.click(field); await user.keyboard('ни');
  expect(document.getElementById(field.getAttribute('aria-activedescendant')!)).toHaveTextContent('Низкий');
  await user.keyboard('{Escape}'); expect(change).not.toHaveBeenCalled(); expect(field).toHaveTextContent('Все');
  await waitFor(() => expect(screen.queryByRole('listbox')).not.toBeInTheDocument());
  await user.keyboard('{End}{Enter}'); expect(field).toHaveTextContent('Низкий');
});
it('closes on Tab and outside clicks without stealing focus or submitting a form', async () => {
  const user = userEvent.setup(); render(<Field/>); const field = screen.getByRole('combobox');
  await user.click(field); await user.keyboard('{Tab}'); expect(screen.getByRole('button', { name: 'Дальше' })).toHaveFocus();
  await waitFor(() => expect(screen.queryByRole('listbox')).not.toBeInTheDocument());
  await user.click(field); await user.click(screen.getByRole('button', { name: 'Дальше' }));
  await waitFor(() => expect(screen.queryByRole('listbox')).not.toBeInTheDocument());
  expect(screen.getByRole('button', { name: 'Дальше' })).toHaveFocus();
});
it('supports pointer selection and required validation, while disabled controls stay closed', async () => {
  const user = userEvent.setup(); const { rerender } = render(<Field required/>);
  const form = screen.getByRole('form') as HTMLFormElement; expect(form.checkValidity()).toBe(false);
  expect(screen.getByRole('combobox')).toHaveFocus(); await user.click(screen.getByRole('combobox'));
  await user.click(screen.getByRole('option', { name: 'Высокий' })); expect(form.checkValidity()).toBe(true);
  rerender(<Select aria-label="Недоступно" value="" options={options} disabled onChange={() => {}}/>);
  await user.click(screen.getByRole('combobox')); expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
});
