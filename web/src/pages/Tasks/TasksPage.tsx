import { useState } from 'react';
import { Sun, CalendarDays, Clock3, CheckCircle2, AlertCircle, SlidersHorizontal, type LucideIcon } from 'lucide-react';
import { useData } from '../../app/DataContext';
import { useEventActions } from '../../app/EventActionsContext';
import { useEventSearch } from '../../hooks/useEventSearch';
import { PageHeader, SearchBox, NewButton, Segments, Empty, Alert, IconButton, Button } from '../../components/UI';
import { TaskCard } from '../../features/events/TaskCard';
import { addDays, weekStart } from '../../features/calendar/dates';
import type { Priority, RhythmEvent } from '../../types/api';
type Filter = 'all' | 'today' | 'soon' | 'done';
function TaskGroup({ title, icon: Icon, events, selected, select }: { title: string; icon: LucideIcon; events: RhythmEvent[]; selected: number | null; select: (id: number) => void }) {
  const [limit, setLimit] = useState(5); if (!events.length) return null;
  return <section className="event-group task-group"><h2><Icon size={23}/>{title}<span className="count">{events.length}</span></h2>{events.slice(0, limit).map(event => <TaskCard key={event.id} event={event} expanded={selected === event.id} onSelect={() => select(event.id)}/>)}{events.length > limit && <Button variant="ghost" onClick={() => setLimit(limit + 20)}>Показать ещё</Button>}</section>;
}
export function TasksPage() {
  const { today } = useData(); const actions = useEventActions(); const [query, setQuery] = useState(''); const [filter, setFilter] = useState<Filter>('all'); const [priority, setPriority] = useState<Priority | ''>(''); const [filters, setFilters] = useState(false); const [selected, setSelected] = useState<number | null>(null); const [collapsed, setCollapsed] = useState(false);
  const result = useEventSearch(query); const all = result.events.filter(event => event.kind === 'task' && (!priority || event.priority === priority));
  const done = all.filter(event => event.status === 'done'); const open = all.filter(event => event.status === 'open');
  const items = filter === 'done' ? done : filter === 'today' ? open.filter(event => event.date === today) : filter === 'soon' ? open.filter(event => event.date && event.date > today && event.date <= addDays(today, 7)) : all;
  const weekEnd = addDays(weekStart(today), 7); const pending = items.filter(event => event.status === 'open');
  const select = (id: number) => { const current = selected ?? (collapsed ? null : pending.find(event => event.subtasks?.length)?.id ?? null); setSelected(current === id ? null : id); setCollapsed(current === id); };
  const expanded = selected ?? (collapsed ? null : pending.find(event => event.subtasks?.length)?.id ?? null);
  const groups = [{ title: 'Сегодня', icon: Sun, events: pending.filter(e => e.date === today) }, { title: 'На неделе', icon: CalendarDays, events: pending.filter(e => e.date && e.date > today && e.date < weekEnd) }, { title: 'Позже', icon: Clock3, events: pending.filter(e => !e.date || e.date >= weekEnd) }, { title: 'Просрочено', icon: AlertCircle, events: pending.filter(e => e.date && e.date < today) }, { title: 'Выполнено', icon: CheckCircle2, events: items.filter(e => e.status === 'done') }];
  return <div className="page tasks-page"><div className="tasks-top"><PageHeader title="Задачи" subtitle="Все задачи под контролем" mascot={false}/><SearchBox value={query} onChange={setQuery} label="Поиск задач" placeholder="Поиск задач, проектов или меток..."/><IconButton icon={SlidersHorizontal} label="Фильтр приоритета" aria-expanded={filters} onClick={() => setFilters(!filters)}/></div><div className="toolbar tasks-toolbar"><Segments value={filter} onChange={setFilter} label="Фильтр задач" options={[{ value: 'all', label: 'Все', count: all.length }, { value: 'today', label: 'Сегодня', count: open.filter(e => e.date === today).length }, { value: 'soon', label: 'Скоро', count: open.filter(e => e.date && e.date > today && e.date <= addDays(today, 7)).length }, { value: 'done', label: 'Выполнено', count: done.length }]}/><NewButton onClick={() => actions.create('task')}>Новая задача</NewButton></div>
    {filters && <label className="priority-filter">Приоритет<select value={priority} onChange={e => setPriority(e.target.value as Priority | '')}><option value="">Все</option><option value="high">Высокий</option><option value="normal">Обычный</option><option value="low">Низкий</option></select></label>}{result.error && <Alert>{result.error}</Alert>}
    <div className="event-groups">{groups.map(group => <TaskGroup key={group.title} {...group} selected={expanded} select={select}/>)}</div>{!items.length && !result.loading && <Empty>Подходящих задач нет</Empty>}
  </div>;
}
