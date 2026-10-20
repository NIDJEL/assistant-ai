import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { DemoDataCard } from './DemoDataCard';
import { importCSV, importDemo } from '../../api/events';

const { refresh } = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock('../../app/DataContext', () => ({ useData: () => ({ refresh }) }));
vi.mock('../../api/events', () => ({ importDemo: vi.fn(), importCSV: vi.fn() }));

beforeEach(() => { vi.clearAllMocks(); });

it('imports only on request and explains an idempotent repeat', async () => {
  vi.mocked(importDemo).mockResolvedValueOnce({ imported: 1000, skipped: 0, total_rows: 1000 }).mockResolvedValueOnce({ imported: 0, skipped: 1000, total_rows: 1000 });
  render(<DemoDataCard/>);
  expect(importDemo).not.toHaveBeenCalled();
  await userEvent.click(screen.getByRole('button', { name: 'Загрузить примеры' }));
  expect(await screen.findByRole('status')).toHaveTextContent('Добавлено записей: 1000');
  expect(refresh).toHaveBeenCalledTimes(1);
  await userEvent.click(screen.getByRole('button', { name: 'Загрузить примеры' }));
  expect(await screen.findByRole('status')).toHaveTextContent('Повторных записей не добавлено');
});

it('allows retry after a failed import without claiming success', async () => {
  vi.mocked(importDemo).mockRejectedValueOnce(new Error('Сеть недоступна')).mockResolvedValueOnce({ imported: 1000, skipped: 0, total_rows: 1000 });
  render(<DemoDataCard/>);
  await userEvent.click(screen.getByRole('button', { name: 'Загрузить примеры' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Сеть недоступна');
  expect(refresh).not.toHaveBeenCalled();
  await userEvent.click(screen.getByRole('button', { name: 'Загрузить примеры' }));
  expect(await screen.findByRole('status')).toHaveTextContent('Добавлено записей: 1000');
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
});

it('uploads a selected file only after confirmation and allows a retry', async () => {
  vi.mocked(importCSV).mockRejectedValueOnce(new Error('Ошибка CSV в строке 3')).mockResolvedValueOnce({ imported: 2, skipped: 0, total_rows: 2 });
  render(<DemoDataCard/>);
  const button = screen.getByRole('button', { name: 'Загрузить свой CSV' });
  expect(button).toBeDisabled();
  const file = new File(['id,date,time,text\n1,2026-12-01,10:00,Отчёт'], 'my-events.csv', { type: 'text/csv' });
  await userEvent.upload(screen.getByLabelText('Свой CSV-файл'), file);
  expect(importCSV).not.toHaveBeenCalled();
  await userEvent.click(button);
  expect(importCSV).toHaveBeenCalledWith(file);
  expect(await screen.findByRole('alert')).toHaveTextContent('Ошибка CSV в строке 3');
  expect(refresh).not.toHaveBeenCalled();
  await userEvent.click(button);
  expect(await screen.findByRole('status')).toHaveTextContent('Добавлено записей: 2');
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  expect(importDemo).not.toHaveBeenCalled();
});

it('rejects an empty or oversized file before sending it', async () => {
  render(<DemoDataCard/>);
  const input = screen.getByLabelText('Свой CSV-файл');
  await userEvent.upload(input, new File([], 'empty.csv', { type: 'text/csv' }));
  expect(screen.getByRole('alert')).toHaveTextContent('CSV-файл пуст');
  const large = new File(['x'], 'large.csv', { type: 'text/csv' });
  Object.defineProperty(large, 'size', { value: 5 * 1024 * 1024 + 1 });
  await userEvent.upload(input, large);
  expect(screen.getByRole('alert')).toHaveTextContent('не больше 5 МБ');
  expect(importCSV).not.toHaveBeenCalled();
  expect(screen.getByRole('button', { name: 'Загрузить свой CSV' })).toBeDisabled();
});
