import { Select } from '../../components/Select';
import { useRef, useState, type FormEvent } from 'react';
import { Dialog } from '../../components/Dialog';
import { Alert, Button, IconButton } from '../../components/UI';
import { Plus, X } from 'lucide-react';
import { useData } from '../../app/DataContext';
import { ApiError, errorMessage } from '../../api/client';
import { dateLabel } from '../calendar/dates';
import { kindLabels, priorityLabels } from './presentation';
import type { EventInput, EventKind, RhythmEvent } from '../../types/api';
import { RecurrenceEditor } from './RecurrenceEditor';
import { ReminderSettings } from './ReminderSettings';
import { useSocial } from '../social/SocialContext';
import { FriendPicker } from '../social/FriendPicker';
import { AvailabilityView } from '../social/AvailabilityView';
import { localSlot, type Availability } from '../social/types';
import { DatePicker } from '../../components/DatePicker';
import { AutoTextarea } from '../../components/AutoTextarea';
export interface EditorSpec { kind: EventKind; event?: RhythmEvent; date?: string; time?: string; title?: string; duration?: number; focusDate?: boolean; friendIds?: string[]; nonce: number }
export function EventEditor({ spec, onClose }: { spec: EditorSpec; onClose: () => void }) {
  const data = useData(); const event = spec.event; const single = !!event?.virtual_occurrence;
  const social = useSocial(); const [friendIds, setFriendIds] = useState<string[]>(spec.friendIds || []); const [availability, setAvailability] = useState<Availability | null>(null);
  const [input, setInput] = useState<EventInput>({ kind: spec.kind, title: event?.title || spec.title || '', description: event?.description || '', date: event ? event.date : spec.date || data.today, time: event ? event.time : spec.time || (spec.kind === 'task' ? null : '10:00'), duration_minutes: event?.duration_minutes || spec.duration || 60, priority: event?.priority || 'normal', status: event?.status || 'open', location: event?.location || '', meeting_url: event?.meeting_url || '', recurrence: single ? null : event?.recurrence || null });
  const [error, setError] = useState(''); const [conflicts, setConflicts] = useState<RhythmEvent[]>([]); const [busy, setBusy] = useState(false); const locked = useRef(false);
  const [tags, setTags] = useState((event?.tags || []).join(', '));
  const [subtasks, setSubtasks] = useState(event?.subtasks || []);
  const [participants, setParticipants] = useState(event?.participants || []);
  const [reminderEnabled, setReminderEnabled] = useState(event ? event.reminder_enabled ?? (event.kind === 'reminder' || event.reminder_minutes !== undefined) : spec.kind !== 'task');
  const [reminderMinutes, setReminderMinutes] = useState(event?.reminder_minutes ?? (event ? 0 : 60));
  const set = <K extends keyof EventInput>(key: K, value: EventInput[K]) => { setInput(current => ({ ...current, [key]: value, ...(key === 'date' && !value ? { time: null } : {}) })); setConflicts([]); setError(''); };
  async function save(force = false) {
    if (locked.current) return; locked.current = true; setBusy(true); setError('');
    try {
      const payload = { ...input, title: input.title.trim(), reminder_enabled: !!input.date && !!input.time && reminderEnabled, reminder_minutes: reminderMinutes, tags: [...new Set(tags.split(',').map(tag => tag.trim()).filter(Boolean))], ...(spec.kind === 'task' ? { subtasks } : {}), ...(spec.kind === 'meeting' ? { participants } : {}), force };
      if (event) { if (event.collaborative && input.date && input.time) { const checked = await social.check(friendIds, localSlot(input.date, input.time, input.duration_minutes || 60, data.health!.timezone), event.id); if (!checked.complete) social.inform('Доступность внешних календарей проверена не полностью. Проверьте время с участниками.'); } const { kind: _kind, ...patch } = payload; if (single) { delete patch.recurrence; } const updated = await data.update(event.id, { ...patch, ...(single ? { edit_occurrence_at: event.recurrence_id } : {}) }); if (friendIds.length) await social.perform({ action: 'invite', event_id: event.id, version: updated.meeting_version || 1, friend_ids: friendIds }); }
      else if (spec.kind === 'meeting' && friendIds.length) await social.perform({ action: 'create_meeting', event: payload, friend_ids: friendIds });
      else await data.create(payload);
      onClose();
    } catch (e) { setError(errorMessage(e)); setConflicts(e instanceof ApiError && e.code === 'meeting_conflict' ? e.conflicts : []); if (e instanceof ApiError && e.code === 'availability_conflict' && input.date && input.time) { try { setAvailability(await social.check(friendIds, localSlot(input.date, input.time, input.duration_minutes || 60, data.health!.timezone), event?.collaborative ? event.id : 0)); } catch (checkError) { setError(`${errorMessage(e)} ${errorMessage(checkError)}`); } } }
    finally { setBusy(false); locked.current = false; }
  }
  const submit = (e: FormEvent) => { e.preventDefault(); void save(); };
  async function checkAvailability() { if (locked.current || !input.date || !input.time) return; locked.current = true; setBusy(true); setError(''); try { setAvailability(await social.check(friendIds, localSlot(input.date, input.time, input.duration_minutes || 60, data.health!.timezone), event?.collaborative ? event.id : 0)); } catch (e) { setError(errorMessage(e)); } finally { locked.current = false; setBusy(false); } }
  const title = spec.focusDate ? 'Перенести встречу' : event ? `Редактировать: ${kindLabels[spec.kind].toLowerCase()}` : spec.kind === 'task' ? 'Новая задача' : spec.kind === 'meeting' ? 'Новая встреча' : 'Новое напоминание';
  return <Dialog title={title} onClose={onClose}><form onSubmit={submit} className="event-form">
    {single && <p role="status">Изменяется только повторение {dateLabel(event!.date, data.today)}. Остальная серия сохранится. Для изменения всей серии откройте её в списке встреч.</p>}
    <label>Название<input data-autofocus={!spec.focusDate || undefined} name="title" value={input.title} onChange={e => set('title', e.target.value)} required maxLength={500} placeholder="Что запланируем?" disabled={busy}/></label>
    <div className="form-grid"><label>{spec.kind === 'task' ? 'Срок' : 'Дата'}<DatePicker aria-label={spec.kind === 'task' ? 'Срок' : 'Дата'} data-autofocus={spec.focusDate || undefined} name="date" value={input.date || ''} onChange={value => set('date', value || null)} timezone={data.health!.timezone} required={spec.kind !== 'task'} disabled={busy}/></label>
      <label>Время<input type="time" name="time" value={input.time || ''} onChange={e => set('time', e.target.value || null)} required={spec.kind !== 'task'} disabled={busy || !input.date}/></label>
      <label>Длительность, мин<input type="number" name="duration" min={5} max={1440} value={input.duration_minutes} onChange={e => set('duration_minutes', Number(e.target.value))} required disabled={busy}/></label>
      <label>Приоритет<Select aria-label="Приоритет" name="priority" value={input.priority || 'normal'} onChange={value => set('priority', value as EventInput['priority'])} disabled={busy} options={Object.entries(priorityLabels).map(([value, label]) => ({ value, label }))}/></label></div>
    {spec.kind === 'meeting' && <><FriendPicker value={friendIds} onChange={ids => { setFriendIds(ids); setAvailability(null); }} disabled={busy} exclude={social.meetings.find(m => m.event.id === event?.id)?.members.map(p => p.id)}/>{(friendIds.length > 0 || event?.collaborative) && <><Button variant="soft" disabled={busy || !input.date || !input.time} onClick={() => void checkAvailability()}>Проверить общее свободное время</Button>{availability && <AvailabilityView value={availability} zone={data.health!.timezone} onSelect={slot => { const formatted = new Intl.DateTimeFormat('sv-SE', { timeZone: data.health!.timezone, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).format(new Date(slot.start)); const [date, time] = formatted.split(' '); setInput(current => ({ ...current, date, time })); setAvailability(null); }}/>}</>}<label>Место<input name="location" value={input.location} onChange={e => set('location', e.target.value)} maxLength={500} placeholder="Офис или переговорная" disabled={busy}/></label><label>Ссылка встречи<input name="meeting_url" type="url" value={input.meeting_url} onChange={e => set('meeting_url', e.target.value)} maxLength={2048} placeholder="https://…" disabled={busy}/></label></>}
    {!single && input.recurrence && <div className="form-grid"><label>Количество повторений<input aria-label="Количество повторений" type="number" min={1} max={1000} value={input.recurrence.count || ''} required={spec.kind === 'meeting' && !input.recurrence.until} disabled={busy} onChange={e => set('recurrence', { ...input.recurrence!, until: undefined, count: e.target.value ? Number(e.target.value) : undefined })}/></label><label>Или повторять до<DatePicker aria-label="Дата окончания серии" value={input.recurrence.until || ''} timezone={data.health!.timezone} disabled={busy} onChange={value => set('recurrence', { ...input.recurrence!, count: undefined, until: value || undefined })}/></label></div>}
    <label>Заметка<AutoTextarea name="description" value={input.description} onChange={e => set('description', e.target.value)} rows={3} maxLength={8000} placeholder="Необязательно" disabled={busy}/></label>
    {input.date && input.time && <ReminderSettings enabled={reminderEnabled} minutes={reminderMinutes} disabled={busy} onChange={(enabled, minutes) => { setReminderEnabled(enabled); setReminderMinutes(minutes); }}/>}
    {!single && !event?.collaborative && !friendIds.length && <RecurrenceEditor value={input.recurrence} date={input.date} time={input.time} timezone={data.health!.timezone} disabled={busy} onChange={value => set('recurrence', value)}/>}
    <label>Метки<input value={tags} onChange={e => setTags(e.target.value)} placeholder="Через запятую" maxLength={600} disabled={busy}/></label>
    {spec.kind === 'task' && <fieldset className="collection-editor"><legend>Подзадачи</legend>{subtasks.map((task, i) => <div key={task.id}><input aria-label={`Подзадача ${i + 1}`} value={task.title} required maxLength={500} disabled={busy} onChange={e => setSubtasks(current => current.map(t => t.id === task.id ? { ...t, title: e.target.value } : t))}/><IconButton icon={X} label={`Удалить подзадачу ${i + 1}`} disabled={busy} onClick={() => setSubtasks(current => current.filter(t => t.id !== task.id))}/></div>)}<Button variant="ghost" disabled={busy || subtasks.length >= 50} onClick={() => setSubtasks(current => [...current, { id: crypto.randomUUID(), title: '', done: false }])}><Plus size={16}/>Добавить подзадачу</Button></fieldset>}
    {spec.kind === 'meeting' && !friendIds.length && !event?.collaborative && <fieldset className="collection-editor"><legend>Участники</legend>{participants.map((person, i) => <div key={i}><input aria-label={`Участник ${i + 1}`} placeholder="Имя" value={person.name} required maxLength={100} disabled={busy} onChange={e => setParticipants(current => current.map((p, n) => n === i ? { ...p, name: e.target.value } : p))}/><input aria-label={`Email участника ${i + 1}`} type="email" placeholder="Email (необязательно)" value={person.email || ''} maxLength={254} disabled={busy} onChange={e => setParticipants(current => current.map((p, n) => n === i ? { ...p, email: e.target.value } : p))}/><IconButton icon={X} label={`Удалить участника ${i + 1}`} disabled={busy} onClick={() => setParticipants(current => current.filter((_, n) => n !== i))}/></div>)}<Button variant="ghost" disabled={busy || participants.length >= 50} onClick={() => setParticipants(current => [...current, { name: '' }])}><Plus size={16}/>Добавить участника</Button></fieldset>}
    {error && <Alert>{error}</Alert>}
    {conflicts.length > 0 && <div className="conflict-list"><h3>Пересечение со встречами</h3>{conflicts.map(item => <p key={item.id}>{item.title} · {dateLabel(item.date, data.today)}, {item.time}</p>)}<Button variant="danger" onClick={() => void save(true)} disabled={busy}>Сохранить с пересечением</Button></div>}
    <footer className="dialog-actions"><Button variant="ghost" onClick={onClose}>Отмена</Button><Button type="submit" disabled={busy}>{busy ? 'Сохраняю…' : 'Сохранить'}</Button></footer>
  </form></Dialog>;
}
