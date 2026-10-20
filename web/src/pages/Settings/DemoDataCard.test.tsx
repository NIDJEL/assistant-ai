import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { DemoDataCard } from './DemoDataCard';
import { importDemo } from '../../api/events';

const { refresh } = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock('../../app/DataContext', () => ({ useData: () => ({ refresh }) }));
vi.mock('../../api/events', () => ({ importDemo: vi.fn() }));

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
