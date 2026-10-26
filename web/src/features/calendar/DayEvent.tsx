import { EventMenu } from '../events/EventMenu';
import { useEventActions } from '../../app/EventActionsContext';
import { timeRange } from '../../pages/Meetings/MeetingsPage';
import { recurrenceLabel } from '../events/recurrence';
import { kindLabels } from '../events/presentation';
import type { RhythmEvent } from '../../types/api';
export function DayEvent({ event }: { event: RhythmEvent }) {
  const actions = useEventActions();
  return <article className={`day-event kind-${event.kind}`}><i className="dot"/><time>{timeRange(event.time, event.duration_minutes)}</time><button aria-label={event.display_title || event.title} onClick={() => actions.edit(event)}><strong>{event.display_title || event.title}</strong><small>{event.location || (event.meeting_url ? 'Онлайн' : kindLabels[event.kind])}{event.description && ` · ${event.description}`}{event.collaborative && event.organizer_name && ` · Организатор: ${event.organizer_name}`}{event.recurrence && ` · ${recurrenceLabel(event.recurrence)}`}{event.occurrences_in_day && event.occurrences_in_day > 1 ? ` · ${event.occurrences_in_day} повторений за день` : ""}</small></button><EventMenu event={event}/></article>;
}
