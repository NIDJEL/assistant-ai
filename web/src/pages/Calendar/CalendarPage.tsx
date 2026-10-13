import { useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { ChevronLeft, ChevronRight, CalendarDays } from 'lucide-react';
import { useData } from '../../app/DataContext';
import { useEventActions } from '../../app/EventActionsContext';
import { PageHeader, SearchBox, Segments, NewButton, IconButton, Empty, Alert, Button } from '../../components/UI';
import { EventRow } from '../../features/events/EventRow';
import { byMoment } from '../../features/events/presentation';
import { addDays, addMonths, formatDay, formatMonth, isCivilDay, monthDays, weekStart } from '../../features/calendar/dates';
import { useEventSearch } from '../../hooks/useEventSearch';
type View = 'month' | 'week' | 'day';
export function CalendarPage() {
  const { today } = useData(); const actions = useEventActions(); const [params, setParams] = useSearchParams();
  const selected = isCivilDay(params.get('date')) ? params.get('date')! : today;
  const [view, setView] = useState<View>('month'); const [query, setQuery] = useState(''); const result = useEventSearch(query);
  const events = [...result.events].sort(byMoment); const dayEvents = events.filter(event => event.date === selected);
  const select = (day: string) => setParams({ date: day }, { replace: true });
  const change = (direction: number) => select(view === 'month' ? addMonths(selected, direction) : addDays(selected, direction * (view === 'week' ? 7 : 1)));
  const canChange = (direction: number) => view === 'month' ? addMonths(selected, direction).slice(0, 7) !== selected.slice(0, 7) : view === 'week' ? weekStart(addDays(selected, direction * 7)) !== weekStart(selected) : addDays(selected, direction) !== selected;
  const days = view === 'month' ? monthDays(selected) : [...new Set(Array.from({ length: 7 }, (_, i) => addDays(weekStart(selected), i)))];
  return <div className="page"><PageHeader title="Календарь" subtitle="Планируйте день, неделю и месяц"/><div className="toolbar calendar-toolbar"><div className="month-switch"><IconButton icon={ChevronLeft} label="Предыдущий период" disabled={!canChange(-1)} onClick={() => change(-1)}/><strong>{view === 'day' ? formatDay(selected) : formatMonth(selected)}</strong><IconButton icon={ChevronRight} label="Следующий период" disabled={!canChange(1)} onClick={() => change(1)}/></div><SearchBox value={query} onChange={setQuery} label="Поиск событий"/><Segments value={view} onChange={setView} label="Вид календаря" options={[{ value: 'day', label: 'День' }, { value: 'week', label: 'Неделя' }, { value: 'month', label: 'Месяц' }]}/><NewButton onClick={() => actions.create('meeting', selected)}>Новое событие</NewButton></div>
    {result.error && <Alert>{result.error}</Alert>}
    {view === 'month' && <section className="month-calendar" aria-label="Календарь на месяц"><div className="calendar-weekdays">{['Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб', 'Вс'].map(day => <span key={day}>{day}</span>)}</div><div className="calendar-grid">{days.map((day, index) => {
      const items = events.filter(event => event.date === day); return <button type="button" key={day} className={`calendar-cell ${day.slice(0, 7) !== selected.slice(0, 7) ? 'outside' : ''} ${index % 7 >= 5 ? 'weekend' : ''} ${day === selected ? 'selected' : ''}`} aria-label={`${formatDay(day)}, событий: ${items.length}`} aria-current={day === today ? 'date' : undefined} onClick={() => select(day)}><span className={`day-number ${day === today ? 'today' : ''}`}>{Number(day.slice(-2))}</span><span className="cell-events">{items.slice(0, 2).map(event => <span className={`cell-event kind-${event.kind} ${event.status === 'done' ? 'done' : ''}`} key={event.id}><i className="dot"/>{event.title}</span>)}{items.length > 2 && <small>+{items.length - 2}</small>}</span></button>;
    })}</div></section>}
    {view === 'week' && <section className="week-grid" aria-label="Календарь на неделю">{days.map(day => <div className={`week-day ${day === selected ? 'selected' : ''}`} key={day}><button type="button" className="week-date" onClick={() => select(day)}>{formatDay(day, { weekday: 'short', day: 'numeric' })}</button>{events.filter(event => event.date === day).slice(0, 5).map(event => <button type="button" className={`week-event kind-${event.kind}`} key={event.id} onClick={() => actions.edit(event)}><small>{event.time || 'Без времени'}</small><span>{event.title}</span></button>)}{!events.some(event => event.date === day) && <small className="muted">Свободно</small>}</div>)}</section>}
    <section className="selected-day"><header><h2><CalendarDays size={21}/>{formatDay(selected, { day: 'numeric', month: 'long', weekday: 'long' })}</h2><span className="muted">{dayEvents.length} событий</span><Button variant="ghost" onClick={() => select(today)}>Сегодня</Button></header>{dayEvents.length ? dayEvents.map(event => <EventRow key={event.id} event={event}/>) : <Empty action={<Button variant="soft" onClick={() => actions.create('meeting', selected)}>Добавить событие</Button>}>На этот день ничего не запланировано</Empty>}</section>
  </div>;
}
