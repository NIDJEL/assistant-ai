import { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { MotionLink as Link } from '../../components/motion/MotionNavLink';
import { motionUpdate } from '../../components/motion/motion';
import { ArrowRight, ChevronLeft, ChevronRight, GripVertical, Plus, Settings } from 'lucide-react';
import { useData } from '../../app/DataContext';
import { useEventActions } from '../../app/EventActionsContext';
import { Button, IconButton } from '../../components/UI';
import { monthDays, monthStart, addMonths, formatMonth, dateLabel, durationLabel, isCivilDay } from '../calendar/dates';
import { byMoment } from '../events/presentation';
import { CompleteCheckbox } from '../events/EventRow';
import { useWidgetFrame } from './WidgetPanel';
import { Tags } from '../events/TaskCard';
import type { RhythmEvent } from '../../types/api';
function Frame({ title, children, onAdd }: { title: string; children: React.ReactNode; onAdd?: () => void }) {
  const { handle, configure } = useWidgetFrame();
  return <section className="widget"><header>{handle || <GripVertical size={18}/>}<h3>{title}</h3>{onAdd && <IconButton icon={Plus} label={`Добавить: ${title}`} onClick={onAdd}/>}<IconButton icon={Settings} className="widget-configure" label={`Настройки: ${title}`} onClick={configure}/></header>{children}</section>;
}
export function CalendarWidget() {
  const { today, events } = useData(); const [params] = useSearchParams(); const selected = isCivilDay(params.get('date')) ? params.get('date')! : today; const [month, setMonth] = useState(monthStart(selected)); const navigate = useNavigate();
  useEffect(() => setMonth(monthStart(selected)), [selected]);
  return <Frame title="Календарь"><div className="mini-month-heading"><strong>{formatMonth(month)}</strong><div><IconButton icon={ChevronLeft} label="Предыдущий месяц в виджете" disabled={addMonths(month, -1) === month} onClick={() => motionUpdate(() => setMonth(current => addMonths(current, -1)))}/><IconButton icon={ChevronRight} label="Следующий месяц в виджете" disabled={addMonths(month, 1) === month} onClick={() => motionUpdate(() => setMonth(current => addMonths(current, 1)))}/></div></div>
    <div className="mini-calendar">{['Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб', 'Вс'].map(day => <span className="weekday" key={day}>{day}</span>)}{monthDays(month).map(day => <button type="button" key={day} className={`${day === selected ? 'today' : ''} ${day.slice(0, 7) !== month.slice(0, 7) ? 'outside' : ''}`} aria-label={`Календарь: ${dateLabel(day, today)}`} aria-current={day === today ? 'date' : undefined} onClick={() => motionUpdate(() => navigate(`/calendar?date=${day}`))}>{Number(day.slice(-2))}{events.some(event => event.date === day && event.status === 'open') && <i aria-hidden="true"/>}</button>)}</div>
  </Frame>;
}
function WidgetEvent({ event }: { event: RhythmEvent }) {
  const actions = useEventActions(); const { today } = useData();
  return <div className="widget-event"><i className={`dot kind-${event.kind}`} aria-hidden="true"/><span className="widget-time">{event.date === today ? event.time || '—' : dateLabel(event.date, today)}</span><button type="button" onClick={() => actions.edit(event)}>{event.title}</button><small>{durationLabel(event.duration_minutes)}</small></div>;
}
export function TodayWidget() {
  const { events, today } = useData(); const actions = useEventActions(); const { limit, taskTitle } = useWidgetFrame(); const items = events.filter(event => event.date === today && event.status === 'open').sort(byMoment);
  return <Frame title="Сегодня" onAdd={() => actions.create('meeting')}>{items.length ? items.slice(0, limit).map(event => <WidgetEvent key={event.id} event={event}/>) : <p className="widget-empty">Сегодня свободно</p>}{items.length > limit && <Link className="widget-link" to={`/calendar?date=${today}`}>Все события <ArrowRight size={16}/></Link>}</Frame>;
}
export function TasksWidget() {
  const { events, today } = useData(); const actions = useEventActions(); const { limit, taskTitle } = useWidgetFrame(); const items = events.filter(event => event.kind === 'task' && event.status === 'open' && (!event.date || event.date >= today)).sort(byMoment);
  return <Frame title={taskTitle || "Задачи"} onAdd={() => actions.create('task')}>{items.slice(0, limit).map(event => <div className="widget-task" key={event.id}><CompleteCheckbox event={event} label={`Завершить в виджете «${event.title}»`}/><button type="button" onClick={() => actions.edit(event)}>{event.title}</button><i className={`dot priority-${event.priority}`}/></div>)}{!items.length && <p className="widget-empty">Все задачи выполнены</p>}<Link className="widget-link" to="/tasks">Все задачи <ArrowRight size={16}/></Link></Frame>;
}
export function UpcomingMeetingsWidget() {
  const { events, today } = useData(); const actions = useEventActions(); const { limit, taskTitle } = useWidgetFrame(); const items = events.filter(event => event.kind === 'meeting' && event.status === 'open' && event.date && event.date >= today).sort(byMoment);
  return <Frame title="Ближайшие встречи" onAdd={() => actions.create('meeting')}>{items.length ? items.slice(0, limit).map(event => <WidgetEvent key={event.id} event={event}/>) : <p className="widget-empty">Встреч пока нет</p>}<Link className="widget-link" to="/meetings">Все встречи <ArrowRight size={16}/></Link></Frame>;
}
export function ProgressWidget() {
  const { events, today } = useData(); const tasks = events.filter(event => event.kind === 'task');
  const done = tasks.filter(event => event.status === 'done').length; const overdue = tasks.filter(event => event.status === 'open' && event.date && event.date < today).length; const open = tasks.length - done - overdue;
  const doneAngle = tasks.length ? 360 * done / tasks.length : 0; const openAngle = tasks.length ? 360 * (done + open) / tasks.length : 0;
  return <Frame title="Мой прогресс"><div className="progress-body"><div className="progress-ring" style={{ background: tasks.length ? `conic-gradient(#13bc95 0deg ${doneAngle}deg, #0875ff ${doneAngle}deg ${openAngle}deg, #f26f78 ${openAngle}deg 360deg)` : '#eaf1fc' }} role="img" aria-label={`Выполнено ${done} из ${tasks.length}`}><div><strong>{done}</strong><small>из {tasks.length}</small></div></div><dl><div><dt><i className="dot green"/>Выполнено</dt><dd>{done}</dd></div><div><dt><i className="dot blue"/>В работе</dt><dd>{open}</dd></div><div><dt><i className="dot red"/>Просрочено</dt><dd>{overdue}</dd></div></dl></div><Link className="widget-link" to="/tasks">Посмотреть все задачи <ArrowRight size={16}/></Link></Frame>;
}
export function QuickWidget() {
  const actions = useEventActions(); const { today } = useData(); const [title, setTitle] = useState(''); const [date, setDate] = useState(today); const [time, setTime] = useState('10:00'); const [duration, setDuration] = useState(30);
  return <Frame title="Быстрое планирование"><form className="quick-planning" onSubmit={e => { e.preventDefault(); actions.create('meeting', date, time, { title, duration }); }}><input aria-label="Название для быстрого планирования" placeholder="Опишите встречу..." value={title} maxLength={500} onChange={e => setTitle(e.target.value)}/><div><input aria-label="Дата быстрого планирования" type="date" value={date} onChange={e => setDate(e.target.value)} required/><input aria-label="Время быстрого планирования" type="time" value={time} onChange={e => setTime(e.target.value)} required/><select aria-label="Длительность быстрого планирования" value={duration} onChange={e => setDuration(Number(e.target.value))}><option value={30}>30 мин</option><option value={60}>1 ч</option></select><Button type="submit" aria-label="Создать из быстрого планирования"><ArrowRight size={18}/></Button></div></form></Frame>;
}
export function TagsWidget() {
  const { events } = useData(); const { limit } = useWidgetFrame();
  const tags = [...new Set(events.filter(e => e.kind === 'task').flatMap(e => e.tags || []))];
  return <Frame title="Метки"><div className="widget-tags">{tags.slice(0, limit).map(tag => <div key={tag}><Tags tags={[tag]}/><span>{events.filter(e => e.tags?.includes(tag)).length}</span></div>)}</div>{!tags.length && <p className="widget-empty">Добавляйте метки в редакторе задачи</p>}</Frame>;
}
export const widgetRegistry = { calendar: CalendarWidget, today: TodayWidget, tasks: TasksWidget, meetings: UpcomingMeetingsWidget, progress: ProgressWidget, quick: QuickWidget, tags: TagsWidget };
