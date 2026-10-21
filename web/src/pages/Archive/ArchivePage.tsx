import { Select } from '../../components/Select';
import { useEffect, useRef, useState } from 'react';
import { DatePicker } from '../../components/DatePicker';
import { useSearchParams } from 'react-router-dom';
import { Archive, RotateCcw, CheckCircle2 } from 'lucide-react';
import { useData } from '../../app/DataContext';
import { useEventActions } from '../../app/EventActionsContext';
import { useEventSearch } from '../../hooks/useEventSearch';
import { useDebounced } from '../../hooks/useDebounced';
import { PageHeader, SearchBox, Empty, Alert, Button, Segments } from '../../components/UI';
import { AnimatedList } from '../../components/motion/AnimatedList';
import { EventRow } from '../../features/events/EventRow';
import { isArchived, byMoment } from '../../features/events/presentation';
import { listEvents } from '../../api/events';
import { ApiError, errorMessage } from '../../api/client';
import type { EventKind, EventStatus, RhythmEvent } from '../../types/api';

function DeletedRow({ event, onRestored }: { event: RhythmEvent; onRestored: (event: RhythmEvent) => void }) {
  const data = useData(); const lock = useRef(false); const [busy, setBusy] = useState(false); const [error, setError] = useState(''); const [conflicts, setConflicts] = useState<RhythmEvent[]>([]);
  async function restore(force = false) {
    if (lock.current) return; lock.current = true; setBusy(true); setError('');
    try { onRestored(await data.restore(event.id, force)); }
    catch (error) { setError(errorMessage(error)); setConflicts(error instanceof ApiError ? error.conflicts : []); }
    finally { lock.current = false; setBusy(false); }
  }
  const deleted = event.deleted_at ? new Intl.DateTimeFormat('ru-RU', { timeZone: data.health?.timezone, day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' }).format(new Date(event.deleted_at)) : '';
  return <article className="archive-deleted"><div><strong>{event.title}</strong><small>{event.kind === 'task' ? 'Задача' : event.kind === 'meeting' ? 'Встреча' : 'Напоминание'} · Удалено {deleted}</small>{event.date && <small>Дата: {event.date}{event.time && `, ${event.time}`} · {event.status === 'done' ? 'Выполнено' : 'Не завершено'}</small>}</div><Button variant="soft" aria-label={`Восстановить «${event.title}»`} disabled={busy} onClick={() => void restore()}><RotateCcw size={17}/>{busy ? 'Восстанавливаю…' : 'Восстановить'}</Button>{error && <Alert action={conflicts.length ? <Button disabled={busy} onClick={() => void restore(true)}>Восстановить с пересечением</Button> : undefined}>{error}{conflicts.length > 0 && `: ${conflicts.map(item => item.title).join(', ')}`}</Alert>}</article>;
}

export function ArchivePage() {
  const data = useData(); const { today, revision } = data; const actions = useEventActions(); const [params, setParams] = useSearchParams(); const tab = params.get('view') === 'deleted' ? 'deleted' : 'history';
  const [query, setQuery] = useState(''); const [kind, setKind] = useState<EventKind | ''>(''); const [status, setStatus] = useState<EventStatus | ''>(''); const [start, setStart] = useState(''); const [end, setEnd] = useState(''); const [limit, setLimit] = useState(30);
  const result = useEventSearch(query); const debounced = useDebounced(query.trim()); const [deleted, setDeleted] = useState<RhythmEvent[]>([]); const [error, setError] = useState(''); const [loading, setLoading] = useState(true); const [restored, setRestored] = useState<RhythmEvent | null>(null);
  useEffect(() => {
    const controller = new AbortController(); setLoading(true);
    listEvents({ deleted: 'true', q: debounced }, controller.signal).then(items => { if (!controller.signal.aborted) { setDeleted(items.filter(event => event.deleted_at)); setError(''); } }).catch(error => { if (!controller.signal.aborted) setError(errorMessage(error)); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [revision, debounced]);
  const source = tab === 'deleted' ? deleted : result.events.filter(event => isArchived(event, today));
  const items = source.filter(event => (!kind || event.kind === kind) && (!status || event.status === status) && (!start || !!event.date && event.date >= start) && (!end || !!event.date && event.date <= end)).sort((a, b) => tab === 'deleted' ? (b.deleted_at || '').localeCompare(a.deleted_at || '') : byMoment(b, a));
  const reset = () => { setKind(''); setStatus(''); setStart(''); setEnd(''); setQuery(''); };
  return <div className="page archive-page"><PageHeader title="Архив"/><div className="archive-tabs"><Segments animated value={tab} onChange={value => { setParams(value === 'deleted' ? { view: 'deleted' } : {}); setLimit(30); }} label="Раздел архива" options={[{ value: 'history', label: 'История' }, { value: 'deleted', label: 'Удалённые' }]}/></div><div className="toolbar"><SearchBox value={query} onChange={setQuery} label="Поиск в архиве"/></div><div className="archive-filters"><label>Тип<Select aria-label="Тип в архиве" value={kind} onChange={value => setKind(value as EventKind | '')} options={[{ value: '', label: 'Все записи' }, { value: 'task', label: 'Задачи' }, { value: 'meeting', label: 'Встречи' }, { value: 'reminder', label: 'Напоминания' }]}/></label><label>Статус<Select aria-label="Статус в архиве" value={status} onChange={value => setStatus(value as EventStatus | '')} options={[{ value: '', label: 'Любой' }, { value: 'done', label: 'Выполнено' }, { value: 'open', label: 'Не завершено' }]}/></label><label>С даты<DatePicker aria-label="С даты" value={start} onChange={setStart} timezone={data.health!.timezone}/></label><label>По дату<DatePicker aria-label="По дату" value={end} min={start || undefined} onChange={setEnd} timezone={data.health!.timezone}/></label>{(kind || status || start || end || query) && <Button variant="ghost" onClick={reset}>Сбросить</Button>}</div>
    {(result.error || error) && <Alert>{result.error || error}</Alert>}{restored && <div className="restore-notice" role="status"><CheckCircle2 size={20}/><span>Восстановлено: {restored.title}. Прежняя дата и статус сохранены.</span><Button variant="ghost" onClick={() => actions.edit(restored)}>Открыть</Button></div>}<section className="archive-results motion-content" aria-busy={tab === 'deleted' ? loading || query.trim() !== debounced : result.loading}><h2><Archive size={22}/>{tab === 'deleted' ? 'Удалённые записи' : 'Записи'}<span className="count">{items.length}</span></h2><AnimatedList items={items.slice(0, limit)} itemKey={event => event.id} renderItem={event => tab === 'deleted' ? <DeletedRow event={event} onRestored={setRestored}/> : <EventRow event={event}/>}/>{items.length > limit && <Button variant="ghost" onClick={() => setLimit(current => current + 30)}>Показать ещё ({items.length - limit})</Button>}{!items.length && <Empty>{tab === 'deleted' && loading ? 'Загружаю удалённые записи…' : tab === 'deleted' ? 'Удалённых записей нет' : 'Подходящих записей нет'}</Empty>}</section>
  </div>;
}
