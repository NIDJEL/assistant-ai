import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { ArrowRight, ChevronLeft, ChevronRight, GripVertical, Plus, type LucideIcon, CalendarDays, Bell, CheckSquare, Users, BarChart3 } from 'lucide-react';
import { useData } from '../../app/DataContext';
import { useEventActions } from '../../app/EventActionsContext';
import { Button, Empty, IconButton } from '../../components/UI';
import { monthDays, monthStart, addMonths, formatMonth, dateLabel, durationLabel } from '../calendar/dates';
import { byMoment } from '../events/presentation';
import { CompleteCheckbox } from '../events/EventRow';
import type { RhythmEvent } from '../../types/api';
function Frame({ title, children, onAdd }: { title: string; children: React.ReactNode; onAdd?: () => void }) {
  return <section className="widget"><header><GripVertical size={18} aria-hidden="true"/><h3>{title}</h3>{onAdd && <IconButton icon={Plus} label={`Добавить: ${title}`} onClick={onAdd}/>}</header>{children}</section>;
}
export function CalendarWidget() {
  const { today, events } = useData(); const [month, setMonth] = useState(monthStart(today)); const navigate = useNavigate();
  return <Frame title="Календарь"><div className="mini-month-heading"><strong>{formatMonth(month)}</strong><div><IconButton icon={ChevronLeft} label="Предыдущий месяц в виджете" onClick={() => setMonth(current => addMonths(current, -1))}/><IconButton icon={ChevronRight} label="Следующий месяц в виджете" onClick={() => setMonth(current => addMonths(current, 1))}/></div></div>
    <div className="mini-calendar">{['Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб', 'Вс'].map(day => <span className="weekday" key={day}>{day}</span>)}{monthDays(month).map(day => <button type="button" key={day} className={`${day === today ? 'today' : ''} ${day.slice(0, 7) !== month.slice(0, 7) ? 'outside' : ''}`} aria-label={`Календарь: ${dateLabel(day, today)}`} aria-current={day === today ? 'date' : undefined} onClick={() => navigate(`/calendar?date=${day}`)}>{Number(day.slice(-2))}{events.some(event => event.date === day && event.status === 'open') && <i aria-hidden="true"/>}</button>)}</div>
  </Frame>;
}
const icons: Record<RhythmEvent['kind'], LucideIcon> = { task: CheckSquare, reminder: Bell, meeting: Users };
function WidgetEvent({ event }: { event: RhythmEvent }) {
  const actions = useEventActions(); const { today } = useData(); const Icon = icons[event.kind];
  return <div className="widget-event"><Icon size={15} className={`kind-${event.kind}`} aria-hidden="true"/><span className="widget-time">{event.date === today ? event.time || '—' : dateLabel(event.date, today)}</span><button type="button" onClick={() => actions.edit(event)}>{event.title}</button><small>{durationLabel(event.duration_minutes)}</small></div>;
}
export function TodayWidget() {
  const { events, today } = useData(); const actions = useEventActions(); const items = events.filter(event => event.date === today && event.status === 'open').sort(byMoment);
  return <Frame title="Сегодня" onAdd={() => actions.create('meeting')}>{items.length ? items.slice(0, 5).map(event => <WidgetEvent key={event.id} event={event}/>) : <p className="widget-empty">Сегодня свободно</p>}{items.length > 5 && <Link className="widget-link" to={`/calendar?date=${today}`}>Все события <ArrowRight size={16}/></Link>}</Frame>;
}
export function TasksWidget() {
  const { events, today } = useData(); const actions = useEventActions(); const items = events.filter(event => event.kind === 'task' && event.status === 'open' && (!event.date || event.date >= today)).sort(byMoment);
  return <Frame title="Задачи" onAdd={() => actions.create('task')}>{items.slice(0, 5).map(event => <div className="widget-task" key={event.id}><CompleteCheckbox event={event} label={`Завершить в виджете «${event.title}»`}/><button type="button" onClick={() => actions.edit(event)}>{event.title}</button>{event.priority === 'high' && <span className="priority-dot" title="Высокий приоритет" aria-label="Высокий приоритет"/>}</div>)}{!items.length && <p className="widget-empty">Все задачи выполнены</p>}<Link className="widget-link" to="/tasks">Все задачи <ArrowRight size={16}/></Link></Frame>;
}
export function UpcomingMeetingsWidget() {
  const { events, today } = useData(); const actions = useEventActions(); const items = events.filter(event => event.kind === 'meeting' && event.status === 'open' && event.date && event.date >= today).sort(byMoment);
  return <Frame title="Ближайшие встречи" onAdd={() => actions.create('meeting')}>{items.length ? items.slice(0, 4).map(event => <WidgetEvent key={event.id} event={event}/>) : <p className="widget-empty">Встреч пока нет</p>}<Link className="widget-link" to="/meetings">Все встречи <ArrowRight size={16}/></Link></Frame>;
}
export function ProgressWidget() {
  const { events, today } = useData(); const tasks = events.filter(event => event.kind === 'task');
  const done = tasks.filter(event => event.status === 'done').length; const overdue = tasks.filter(event => event.status === 'open' && event.date && event.date < today).length; const open = tasks.length - done - overdue;
  const doneAngle = tasks.length ? 360 * done / tasks.length : 0; const openAngle = tasks.length ? 360 * (done + open) / tasks.length : 0;
  return <Frame title="Мой прогресс"><div className="progress-body"><div className="progress-ring" style={{ background: tasks.length ? `conic-gradient(#13bc95 0deg ${doneAngle}deg, #0875ff ${doneAngle}deg ${openAngle}deg, #f26f78 ${openAngle}deg 360deg)` : '#eaf1fc' }} role="img" aria-label={`Выполнено ${done} из ${tasks.length}`}><div><strong>{done}</strong><small>из {tasks.length}</small></div></div><dl><div><dt><i className="dot green"/>Выполнено</dt><dd>{done}</dd></div><div><dt><i className="dot blue"/>В работе</dt><dd>{open}</dd></div><div><dt><i className="dot red"/>Просрочено</dt><dd>{overdue}</dd></div></dl></div><Link className="widget-link" to="/tasks">Посмотреть все задачи <ArrowRight size={16}/></Link></Frame>;
}
export const widgetRegistry = { calendar: CalendarWidget, today: TodayWidget, tasks: TasksWidget, meetings: UpcomingMeetingsWidget, progress: ProgressWidget };
export type WidgetKey = keyof typeof widgetRegistry;
export function WidgetList({ keys }: { keys: WidgetKey[] }) { return <div className="widget-list">{keys.map(key => { const Component = widgetRegistry[key]; return <Component key={key}/>; })}</div>; }
