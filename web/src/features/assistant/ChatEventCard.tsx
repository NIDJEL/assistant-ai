import { Bell, CalendarDays, Clock3 } from 'lucide-react';
import { useData } from '../../app/DataContext';
import { useEventActions } from '../../app/EventActionsContext';
import { dateLabel } from '../calendar/dates';
import { EventRow } from '../events/EventRow';
import type { RhythmEvent } from '../../types/api';
import { reminderLabel } from '../events/ReminderSettings';
export function ChatEventCard({ event }: { event: RhythmEvent }) {
  const { today } = useData(); const actions = useEventActions();
  if (event.kind === 'task') return <EventRow event={event} compact/>;
  const Icon = event.kind === 'reminder' ? Bell : CalendarDays;
  return <button className={`chat-event-card kind-${event.kind}`} onClick={() => actions.edit(event)}><Icon size={24}/><span><strong>{event.title}</strong><small><Clock3 size={17}/>{dateLabel(event.date, today)}{event.time && `, ${event.time}`}</small>{event.reminder_enabled !== false && event.reminder_minutes !== undefined && <small><Bell size={16}/>{reminderLabel(event.reminder_minutes)}</small>}</span></button>;
}
