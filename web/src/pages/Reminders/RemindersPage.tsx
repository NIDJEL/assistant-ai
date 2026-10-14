import { useEffect, useRef, useState } from 'react';
import { Sunrise, Sun, Moon, Droplets, Dumbbell, BriefcaseBusiness, Mail, BookOpen, Bell, Clock3, Music2, ListPlus, type LucideIcon } from 'lucide-react';
import { useData } from '../../app/DataContext';
import { useEventActions } from '../../app/EventActionsContext';
import { useEventSearch } from '../../hooks/useEventSearch';
import { PageHeader, SearchBox, NewButton, Segments, Empty, Alert, Button, IconButton } from '../../components/UI';
import { CompleteCheckbox } from '../../features/events/EventRow';
import { EventMenu } from '../../features/events/EventMenu';
import { errorMessage } from '../../api/client';
import { dateLabel } from '../../features/calendar/dates';
import type { RhythmEvent } from '../../types/api';
import { AnimatedList } from '../../components/motion/AnimatedList';
import { Collapsible } from '../../components/motion/Collapsible';
import { recurrenceLabel } from '../../features/events/recurrence';
import { RecurrenceEditor } from '../../features/events/RecurrenceEditor';
import { ReminderSettings } from '../../features/events/ReminderSettings';
function category(title: string): { icon: LucideIcon; title: string; tone: string } {
  if (/вод[ау]|пить/i.test(title)) return { icon: Droplets, title: 'Забота о себе', tone: 'blue' };
  if (/спорт|трениров|бег|зарядк/i.test(title)) return { icon: Dumbbell, title: 'Спорт и здоровье', tone: 'pink' };
  if (/письм|почт/i.test(title)) return { icon: Mail, title: 'Коммуникации', tone: 'purple' };
  if (/книг|читать|прочита|руководств/i.test(title)) return { icon: BookOpen, title: 'Личное развитие', tone: 'green' };
  if (/работ|рабоч|отч[её]т|звон|проект/i.test(title)) return { icon: BriefcaseBusiness, title: 'Работа', tone: 'blue' };
  return { icon: Bell, title: 'Личное', tone: 'purple' };
}
function InlineEditor({ event, close }: { event: RhythmEvent; close: () => void }) {
  const [recurrence, setRecurrence] = useState(event.recurrence || null);
  const [reminderEnabled, setReminderEnabled] = useState(event.reminder_enabled !== false); const [reminderMinutes, setReminderMinutes] = useState(event.reminder_minutes || 0);
  useEffect(() => setReminderEnabled(event.reminder_enabled !== false), [event.reminder_enabled]);
  useEffect(() => setReminderMinutes(event.reminder_minutes || 0), [event.reminder_minutes]);
  const data = useData(); const [title, setTitle] = useState(event.title); const [date, setDate] = useState(event.date || ''); const [time, setTime] = useState(event.time || ''); const [note, setNote] = useState(event.description); const [showNote, setShowNote] = useState(!!note); const [busy, setBusy] = useState(false); const [error, setError] = useState(''); const lock = useRef(false);
  async function save() { if (lock.current) return; lock.current = true; setBusy(true); try { await data.update(event.id, { title: title.trim(), date, time, description: note, recurrence, reminder_enabled: reminderEnabled, reminder_minutes: reminderMinutes }); close(); } catch (e) { setError(errorMessage(e)); } finally { lock.current = false; setBusy(false); } }
  return <form className="reminder-inline" aria-label={`Редактор: ${event.title}`} onSubmit={e => { e.preventDefault(); void save(); }}>
    <input aria-label="Название напоминания" value={title} onChange={e => setTitle(e.target.value)} required maxLength={500}/>
    <div className="inline-fields"><label><span className="sr-only">Дата напоминания</span><input type="date" value={date} onChange={e => setDate(e.target.value)} required/></label><label><Clock3 size={17}/><input aria-label="Время напоминания" type="time" value={time} onChange={e => setTime(e.target.value)} required/></label><RecurrenceEditor value={recurrence} date={date} time={time} timezone={data.health!.timezone} disabled={busy} onChange={setRecurrence}/><label><Music2 size={18}/><select aria-label="Звук напоминания" disabled value="none"><option value="none">Звук · Скоро</option></select></label></div>
    <ReminderSettings enabled={reminderEnabled} minutes={reminderMinutes} disabled={busy} onChange={(enabled, minutes) => { setReminderEnabled(enabled); setReminderMinutes(minutes); }}/>
    {showNote ? <textarea aria-label="Заметка напоминания" value={note} onChange={e => setNote(e.target.value)} maxLength={8000} placeholder="Добавить заметку..."/> : <button type="button" className="text-action" onClick={() => setShowNote(true)}><ListPlus size={18}/>Добавить заметку...</button>}{error && <Alert>{error}</Alert>}<footer><Button variant="ghost" onClick={close}>Отмена</Button><Button type="submit" disabled={busy}>{busy ? 'Сохраняю…' : 'Сохранить'}</Button></footer>
  </form>;
}
function ReminderCard({ event, selected, select }: { event: RhythmEvent; selected: boolean; select: () => void }) {
  const data = useData(); const [busy, setBusy] = useState(false); const [error, setError] = useState(''); const [enabled, setEnabled] = useState<boolean | null>(null); const kind = category(event.title); const Icon = kind.icon;
  async function toggle(enabled: boolean) { setBusy(true); setEnabled(enabled); try { await data.update(event.id, { reminder_enabled: enabled }); } catch (e) { setError(errorMessage(e)); } finally { setBusy(false); setEnabled(null); } }
  return <article className={`reminder-card ${selected ? 'expanded' : ''}`}><div className="reminder-row"><CompleteCheckbox event={event}/><span className={`reminder-icon ${kind.tone}`}><Icon size={23}/></span><button aria-label={event.title} className="reminder-title" onClick={select} aria-expanded={selected}><strong>{event.title}</strong><small><i className={`dot ${kind.tone}`}/>{event.tags?.[0] || kind.title}</small></button><div className="reminder-time"><span><Clock3 size={17}/>{event.time}</span><small>{dateLabel(event.date, data.today)} · {recurrenceLabel(event.recurrence)}</small></div><label className="switch-row"><span className="sr-only">Включить «{event.title}»</span><input type="checkbox" role="switch" checked={enabled ?? event.reminder_enabled !== false} disabled={busy} onChange={e => void toggle(e.target.checked)}/></label><EventMenu event={event}/></div><Collapsible open={selected}><InlineEditor key={event.id} event={event} close={select}/></Collapsible>{error && <Alert>{error}</Alert>}</article>;
}
export function RemindersPage() {
  const { today } = useData(); const actions = useEventActions(); const [query, setQuery] = useState(''); const [filter, setFilter] = useState<'all' | 'today' | 'repeat' | 'important'>('all'); const [selected, setSelected] = useState<number | null>(null); const [limit, setLimit] = useState(2);
  const result = useEventSearch(query); const items = result.events.filter(e => e.kind === 'reminder' && e.status === 'open' && e.date && e.date >= today && (filter === 'all' || filter === 'today' && e.date === today || filter === 'important' && e.priority === 'high' || filter === 'repeat' && !!e.recurrence));
  const hour = (e: RhythmEvent) => Number((e.time || '00:00').slice(0, 2)); const groups = [{ title: 'Утро', icon: Sunrise, items: items.filter(e => hour(e) < 12) }, { title: 'День', icon: Sun, items: items.filter(e => hour(e) >= 12 && hour(e) < 18) }, { title: 'Вечер', icon: Moon, items: items.filter(e => hour(e) >= 18) }];
  return <div className="page reminders-page"><PageHeader title="Напоминания" subtitle="Ничего не забывайте"/><div className="toolbar reminders-toolbar"><SearchBox value={query} onChange={setQuery} label="Поиск напоминаний" placeholder="Поиск по напоминаниям..."/><Segments value={filter} onChange={setFilter} label="Фильтр напоминаний" options={[{ value: 'all', label: 'Все' }, { value: 'today', label: 'Сегодня' }, { value: 'repeat', label: 'Повторяющиеся' }, { value: 'important', label: 'Важные' }]}/><NewButton onClick={() => actions.create('reminder')}>Новое напоминание</NewButton></div>{result.error && <Alert>{result.error}</Alert>}<AnimatedList className="reminder-groups motion-content" items={groups.filter(g => g.items.length)} itemKey={group => group.title} renderItem={({ title, icon: Icon, items }) => <section><h2><Icon size={23}/>{title}<span className="count">{items.length}</span></h2><AnimatedList items={items.slice(0, limit)} itemKey={event => event.id} renderItem={e => <ReminderCard event={e} selected={e.id === selected} select={() => setSelected(current => current === e.id ? null : e.id)}/>}/>{items.length > limit && <Button variant="ghost" onClick={() => setLimit(limit + 20)}>Показать ещё</Button>}</section>}/>{!items.length && <Empty>Напоминаний на этот период нет</Empty>}</div>;
}
