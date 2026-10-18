import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ZoomMeetingCard } from './ZoomMeetingCard';
import type { ZoomMeeting } from '../../types/api';

const meeting: ZoomMeeting = { event_id: 1, title: 'Обсуждение проекта', scheduled_at: '2026-10-19T12:00:00Z', timezone: 'Europe/Moscow', status: 'scheduled', join_url: 'https://us06web.zoom.us/j/123?pwd=opaque' };
it('opens the exact join URL, formats user timezone, and copies it with confirmation', async () => {
  const user = userEvent.setup(); render(<ZoomMeetingCard meeting={meeting}/>);
  expect(screen.getByRole('link', { name: 'Подключиться' })).toHaveAttribute('href', meeting.join_url);
  expect(screen.getByRole('link', { name: 'Подключиться' })).toHaveAttribute('target', '_blank');
  expect(screen.getByText(/15:00/)).toBeVisible(); expect(screen.getByText('Запланирована')).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'Скопировать ссылку' }));
  expect(await navigator.clipboard.readText()).toBe(meeting.join_url); expect(screen.getByText('Ссылка скопирована')).toBeVisible();
});
it('keeps distinct links for different meetings', () => {
  render(<><ZoomMeetingCard meeting={meeting}/><ZoomMeetingCard meeting={{ ...meeting, event_id: 2, title: 'Команда', join_url: 'https://zoom.us/j/456' }}/></>);
  expect(within(screen.getByRole('region', { name: 'Zoom: Команда' })).getByRole('link')).toHaveAttribute('href', 'https://zoom.us/j/456');
  expect(within(screen.getByRole('region', { name: `Zoom: ${meeting.title}` })).getByRole('link')).toHaveAttribute('href', meeting.join_url);
});
it('does not infer live status from a scheduled time in the past and disables ended joining', () => {
  const { rerender } = render(<ZoomMeetingCard meeting={{ ...meeting, scheduled_at: '2020-01-01T12:00:00Z' }}/>);
  expect(screen.getByText('Запланирована')).toBeVisible();
  rerender(<ZoomMeetingCard meeting={{ ...meeting, status: 'started' }}/>); expect(screen.getByText('Идёт')).toBeVisible(); expect(screen.getByRole('link')).toHaveClass('primary');
  rerender(<ZoomMeetingCard meeting={{ ...meeting, status: 'ended' }}/>); expect(screen.getByText('Встреча завершилась.')).toBeVisible(); expect(screen.queryByRole('link')).toBeNull(); expect(screen.getByRole('button', { name: 'Подключиться' })).toBeDisabled();
});
it('rejects host-only and foreign links and reports clipboard failure', async () => {
  const user = userEvent.setup(); const { rerender } = render(<ZoomMeetingCard meeting={{ ...meeting, join_url: 'https://zoom.us/s/123?zak=private' }}/>);
  expect(screen.queryByRole('link')).toBeNull(); expect(screen.getByRole('button', { name: 'Скопировать ссылку' })).toBeDisabled();
  rerender(<ZoomMeetingCard meeting={meeting}/>); vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValueOnce(new Error('denied'));
  await user.click(screen.getByRole('button', { name: 'Скопировать ссылку' })); expect(screen.getByRole('status')).toHaveTextContent('Не удалось скопировать ссылку');
});
