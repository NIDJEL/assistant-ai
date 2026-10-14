import { CalendarDays, Bell, CheckSquare, Pencil, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { useData } from '../../app/DataContext';
import { useEventActions } from '../../app/EventActionsContext';
import { dateLabel, durationLabel } from '../calendar/dates';
import { recurrenceLabel } from './recurrence';
import { priorityLabels } from './presentation';
import { IconButton } from '../../components/UI';
import type { RhythmEvent } from '../../types/api';
export function CompleteCheckbox({ event, label }: { event: RhythmEvent; label?: string }) {
  const { events } = useData(); event = events.find(current => current.id === event.id) || event;
  const actions = useEventActions(); const [pending, setPending] = useState<boolean | null>(null);
  async function toggle(value: boolean) { setPending(value); try { await actions.complete(event); } finally { setPending(null); } }
  return <input className="check" type="checkbox" checked={pending ?? event.status === 'done'} disabled={pending !== null} aria-busy={pending !== null} aria-label={label || `${event.status === 'done' ? 'Возобновить' : 'Завершить'} «${event.title}»`} onChange={e => void toggle(e.target.checked)}/>;
}
export function EventRow({ event, compact = false }: { event: RhythmEvent; compact?: boolean }) {
  const { today } = useData(); const actions = useEventActions(); const Icon = event.kind === 'meeting' ? CalendarDays : event.kind === 'reminder' ? Bell : CheckSquare;
  return <article className={`event-row ${compact ? 'compact' : ''} ${event.status === 'done' ? 'done' : ''}`} data-kind={event.kind}>
    {event.kind !== 'meeting' ? <CompleteCheckbox event={event}/> : <span className="event-symbol"><Icon size={21} aria-hidden="true"/></span>}
    <button type="button" className="event-body" onClick={() => actions.edit(event)}><span className="event-title">{event.title}</span>{!compact && event.description && <span className="event-note">{event.description}</span>}</button>
    {!compact && event.priority !== 'normal' && <span className={`priority ${event.priority}`}>{priorityLabels[event.priority]}</span>}
    <span className={`event-date ${event.date && event.date < today && event.status === 'open' ? 'overdue' : ''}`}>{dateLabel(event.date, today)}{event.time && <><br/>{event.time}</>}{event.recurrence && <small>{recurrenceLabel(event.recurrence)}</small>}{event.kind === 'meeting' && !compact && <small>{durationLabel(event.duration_minutes)}</small>}</span>
    {!compact && <div className="row-actions"><IconButton icon={Pencil} label={`Редактировать «${event.title}»`} onClick={() => actions.edit(event)}/><IconButton icon={Trash2} label={`Удалить «${event.title}»`} onClick={() => actions.remove(event)}/></div>}
  </article>;
}
