import { render, screen } from '@testing-library/react';
import { ExecutionStatuses } from './ExecutionStatuses';

test('delivery and sync status show the user-facing step number', () => {
  render(<ExecutionStatuses steps={[{ key: 'opaque_server_key', status: 'done', reply: 'Сохранено' }]}
    statuses={[{ event_id: 1, provider: 'google', state: 'pending', message: 'Сохранено в Гене. Отправка во внешний сервис ожидается.', step_keys: ['opaque_server_key'] }]}/>);
  expect(screen.getByText('Google Calendar')).toBeInTheDocument();
  expect(screen.getByText(/Действие 1/)).toBeInTheDocument();
  expect(screen.queryByText('opaque_server_key')).toBeNull();
  expect(screen.getByText(/Отправка во внешний сервис ожидается/)).toBeInTheDocument();
});
