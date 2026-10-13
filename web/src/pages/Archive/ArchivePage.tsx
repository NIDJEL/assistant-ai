import { useState } from 'react';
import { Archive } from 'lucide-react';
import { useData } from '../../app/DataContext';
import { useEventSearch } from '../../hooks/useEventSearch';
import { PageHeader, SearchBox, Empty, Alert, Button } from '../../components/UI';
import { EventRow } from '../../features/events/EventRow';
import { isArchived, byMoment } from '../../features/events/presentation';
import type { EventKind, EventStatus } from '../../types/api';
export function ArchivePage() {
  const { today } = useData(); const [query, setQuery] = useState(''); const [kind, setKind] = useState<EventKind | ''>(''); const [status, setStatus] = useState<EventStatus | ''>(''); const [start, setStart] = useState(''); const [end, setEnd] = useState(''); const [limit, setLimit] = useState(30);
  const result = useEventSearch(query); const items = result.events.filter(event => isArchived(event, today) && (!kind || event.kind === kind) && (!status || event.status === status) && (!start || !!event.date && event.date >= start) && (!end || !!event.date && event.date <= end)).sort((a, b) => byMoment(b, a));
  const reset = () => { setKind(''); setStatus(''); setStart(''); setEnd(''); setQuery(''); };
  return <div className="page"><PageHeader title="Архив" subtitle="Прошлые события и выполненные дела"/><div className="toolbar"><SearchBox value={query} onChange={setQuery} label="Поиск в архиве"/></div><div className="archive-filters"><label>Тип<select aria-label="Тип в архиве" value={kind} onChange={e => setKind(e.target.value as EventKind | '')}><option value="">Все записи</option><option value="task">Задачи</option><option value="meeting">Встречи</option><option value="reminder">Напоминания</option></select></label><label>Статус<select aria-label="Статус в архиве" value={status} onChange={e => setStatus(e.target.value as EventStatus | '')}><option value="">Любой</option><option value="done">Выполнено</option><option value="open">Не завершено</option></select></label><label>С даты<input type="date" value={start} onChange={e => setStart(e.target.value)}/></label><label>По дату<input type="date" value={end} min={start || undefined} onChange={e => setEnd(e.target.value)}/></label>{(kind || status || start || end || query) && <Button variant="ghost" onClick={reset}>Сбросить</Button>}</div>
    {result.error && <Alert>{result.error}</Alert>}<section className="archive-results"><h2><Archive size={22}/>Записи<span className="count">{items.length}</span></h2>{items.slice(0, limit).map(event => <EventRow key={event.id} event={event}/>)}{items.length > limit && <Button variant="ghost" onClick={() => setLimit(current => current + 30)}>Показать ещё ({items.length - limit})</Button>}{!items.length && <Empty>Подходящих записей нет</Empty>}</section>
  </div>;
}
