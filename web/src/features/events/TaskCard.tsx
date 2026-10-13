import { useState } from 'react';
import { ChevronUp, Pencil } from 'lucide-react';
import { CompleteCheckbox } from './EventRow';
import { EventMenu } from './EventMenu';
import { priorityLabels } from './presentation';
import { useData } from '../../app/DataContext';
import { useEventActions } from '../../app/EventActionsContext';
import { dateLabel } from '../calendar/dates';
import { IconButton, Alert } from '../../components/UI';
import { errorMessage } from '../../api/client';
import type { RhythmEvent } from '../../types/api';
export function Tags({ tags = [] }: { tags?: string[] }) { return <span className="tag-pills">{tags.map((tag, i) => <span key={`${tag}-${i}`} className={`tag-pill tone-${i % 4}`}>{tag}</span>)}</span>; }
export function TaskCard({ event, expanded, onSelect }: { event: RhythmEvent; expanded: boolean; onSelect: () => void }) {
  const data = useData(); const actions = useEventActions(); const [busy, setBusy] = useState(false); const [error, setError] = useState(''); const [pending, setPending] = useState<Record<string, boolean>>({});
  async function toggle(id: string) {
    setBusy(true); setError(''); setPending({ [id]: !event.subtasks?.find(task => task.id === id)?.done });
    try { await data.update(event.id, { subtasks: event.subtasks?.map(task => task.id === id ? { ...task, done: !task.done } : task) }); }
    catch (e) { setError(errorMessage(e)); } finally { setBusy(false); setPending({}); }
  }
  return <article className={`task-card ${expanded ? 'expanded' : ''} ${event.status === 'done' ? 'done' : ''}`}>
    <div className="task-row"><CompleteCheckbox event={event}/><button className="task-title" onClick={onSelect} aria-expanded={expanded}>{event.title}</button><i className={`dot priority-${event.priority}`} title={`Приоритет: ${priorityLabels[event.priority]}`}/><Tags tags={event.tags}/><span className={`task-due ${event.status === 'open' && event.date && event.date <= data.today ? 'overdue' : ''}`}>{dateLabel(event.date, data.today)}{event.time && `, ${event.time}`}</span>{expanded ? <IconButton icon={ChevronUp} label={`Свернуть «${event.title}»`} onClick={onSelect}/> : <EventMenu event={event}/>}</div>
    {expanded && <div className="task-expanded">{event.description && <p>{event.description}</p>}<div className="subtasks">{event.subtasks?.map(task => <label key={task.id} className={task.done ? 'done' : ''}><input type="checkbox" checked={pending[task.id] ?? task.done} disabled={busy} onChange={() => void toggle(task.id)}/><span>{task.title}</span></label>)}</div>{!event.subtasks?.length && <p className="muted">Подзадач пока нет</p>}<button className="text-action" onClick={() => actions.edit(event)}><Pencil size={15}/>Редактировать задачу и подзадачи</button>{error && <Alert>{error}</Alert>}</div>}
  </article>;
}
