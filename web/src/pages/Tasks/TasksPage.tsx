import { useState } from 'react';
import { Sun, CalendarDays, Clock3, CheckCircle2, AlertCircle } from 'lucide-react';
import { useData } from '../../app/DataContext';
import { useEventActions } from '../../app/EventActionsContext';
import { useEventSearch } from '../../hooks/useEventSearch';
import { PageHeader, SearchBox, NewButton, Segments, Empty, Alert } from '../../components/UI';
import { EventGroup } from '../../features/events/EventGroup';
import { addDays, weekStart } from '../../features/calendar/dates';
import type { Priority } from '../../types/api';
type Filter = 'all' | 'today' | 'soon' | 'done';
export function TasksPage() {
  const { today } = useData(); const actions = useEventActions(); const [query, setQuery] = useState(''); const [filter, setFilter] = useState<Filter>('all'); const [priority, setPriority] = useState<Priority | ''>('');
  const result = useEventSearch(query); const all = result.events.filter(event => event.kind === 'task' && (!priority || event.priority === priority));
  const done = all.filter(event => event.status === 'done'); const open = all.filter(event => event.status === 'open');
  const items = filter === 'done' ? done : filter === 'today' ? open.filter(event => event.date === today) : filter === 'soon' ? open.filter(event => event.date && event.date >= today && event.date <= addDays(today, 7)) : all;
  const weekEnd = addDays(weekStart(today), 7); const pending = items.filter(event => event.status === 'open');
  return <div className="page"><PageHeader title="Задачи" subtitle="Все задачи под контролем"/><div className="toolbar"><SearchBox value={query} onChange={setQuery} label="Поиск задач"/><label className="inline-filter"><span className="sr-only">Фильтр приоритета</span><select aria-label="Фильтр приоритета" value={priority} onChange={e => setPriority(e.target.value as Priority | '')}><option value="">Все приоритеты</option><option value="high">Высокий</option><option value="normal">Обычный</option><option value="low">Низкий</option></select></label><NewButton onClick={() => actions.create('task')}>Новая задача</NewButton></div>
    <Segments value={filter} onChange={setFilter} label="Фильтр задач" options={[{ value: 'all', label: 'Все', count: all.length }, { value: 'today', label: 'Сегодня', count: open.filter(event => event.date === today).length }, { value: 'soon', label: 'Скоро', count: open.filter(event => event.date && event.date >= today && event.date <= addDays(today, 7)).length }, { value: 'done', label: 'Выполнено', count: done.length }]}/>
    {result.error && <Alert>{result.error}</Alert>}{result.loading && <p className="muted" role="status">Ищу…</p>}
    <div className="event-groups"><EventGroup title="Сегодня" icon={Sun} events={pending.filter(event => event.date === today)}/><EventGroup title="На неделе" icon={CalendarDays} events={pending.filter(event => event.date && event.date > today && event.date < weekEnd)}/><EventGroup title="Позже" icon={Clock3} events={pending.filter(event => !event.date || event.date >= weekEnd)}/><EventGroup title="Просрочено" icon={AlertCircle} events={pending.filter(event => event.date && event.date < today)}/><EventGroup title="Выполнено" icon={CheckCircle2} events={items.filter(event => event.status === 'done')}/></div>
    {!items.length && !result.loading && <Empty>Подходящих задач нет</Empty>}
  </div>;
}
