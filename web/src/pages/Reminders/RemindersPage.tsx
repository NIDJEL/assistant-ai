import { useState } from 'react';
import { Sunrise, Sun, Moon, CheckCircle2 } from 'lucide-react';
import { useData } from '../../app/DataContext';
import { useEventActions } from '../../app/EventActionsContext';
import { useEventSearch } from '../../hooks/useEventSearch';
import { PageHeader, SearchBox, NewButton, Segments, Empty, Alert } from '../../components/UI';
import { EventGroup } from '../../features/events/EventGroup';
type Filter = 'upcoming' | 'today' | 'all' | 'done';
export function RemindersPage() {
  const { today } = useData(); const actions = useEventActions(); const [query, setQuery] = useState(''); const [filter, setFilter] = useState<Filter>('upcoming');
  const result = useEventSearch(query); const items = result.events.filter(event => event.kind === 'reminder' && (filter === 'all' || filter === 'done' && event.status === 'done' || event.status === 'open' && event.date && (filter === 'today' ? event.date === today : event.date >= today)));
  const active = items.filter(event => event.status === 'open'); const hour = (time: string | null) => Number((time || '00:00').slice(0, 2));
  return <div className="page"><PageHeader title="Напоминания" subtitle="Ничего не забывайте"/><div className="toolbar"><SearchBox value={query} onChange={setQuery} label="Поиск напоминаний"/><NewButton onClick={() => actions.create('reminder')}>Новое напоминание</NewButton></div><Segments value={filter} onChange={setFilter} label="Фильтр напоминаний" options={[{ value: 'upcoming', label: 'Предстоящие' }, { value: 'today', label: 'Сегодня' }, { value: 'all', label: 'Все' }, { value: 'done', label: 'Завершённые' }]}/>{result.error && <Alert>{result.error}</Alert>}
    <div className="event-groups"><EventGroup title="Утро" icon={Sunrise} events={active.filter(event => hour(event.time) < 12)}/><EventGroup title="День" icon={Sun} events={active.filter(event => hour(event.time) >= 12 && hour(event.time) < 18)}/><EventGroup title="Вечер" icon={Moon} events={active.filter(event => hour(event.time) >= 18)}/><EventGroup title="Завершённые" icon={CheckCircle2} events={items.filter(event => event.status === 'done')}/></div>{!items.length && <Empty>Напоминаний на этот период нет</Empty>}
  </div>;
}
