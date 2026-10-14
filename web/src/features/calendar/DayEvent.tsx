import { EventMenu } from '../events/EventMenu';
import { useEventActions } from '../../app/EventActionsContext';
import { timeRange } from '../../pages/Meetings/MeetingsPage';
import { recurrenceLabel } from '../events/recurrence';
import { kindLabels } from '../events/presentation';
import type { RhythmEvent } from '../../types/api';
export function DayEvent({ event }: { event: RhythmEvent }) {
  const actions = useEventActions();
  return <article className={`day-event kind-${event.kind}`}><i className="dot"/><time>{timeRange(event.time, event.duration_minutes)}</time><button aria-label={event.title} onClick={() => actions.edit(event)}><strong>{event.title}</strong><small>{event.location || (event.meeting_url ? 'Онлайн' : kindLabels[event.kind])}{event.description && ` · ${event.description}`}{event.recurrence && ` · ${recurrenceLabel(event.recurrence)}`}</small></button><EventMenu event={event}/></article>;
}
