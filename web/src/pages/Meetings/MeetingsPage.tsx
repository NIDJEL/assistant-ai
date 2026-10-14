import { useState } from 'react';
import { Clock3, MapPin, Link as LinkIcon, Pencil, Trash2, CalendarDays } from 'lucide-react';
import { useData } from '../../app/DataContext';
import { useEventActions } from '../../app/EventActionsContext';
import { useEventSearch } from '../../hooks/useEventSearch';
import { PageHeader, SearchBox, NewButton, Segments, Empty, Alert, Button, IconButton } from '../../components/UI';
import { byMoment } from '../../features/events/presentation';
import { dateLabel, durationLabel } from '../../features/calendar/dates';
export function MeetingsPage() {
  const { today } = useData(); const actions = useEventActions(); const [query, setQuery] = useState(''); const [period, setPeriod] = useState<'upcoming' | 'today'>('upcoming'); const [selected, setSelected] = useState<number | null>(null); const [limit, setLimit] = useState(30);
  const result = useEventSearch(query); const items = result.events.filter(event => event.kind === 'meeting' && event.status === 'open' && event.date && event.date >= today && (period === 'upcoming' || event.date === today)).sort(byMoment);
  const meeting = items.find(item => item.id === selected) || items[0];
  const days = [...new Set(items.slice(0, limit).map(item => item.date!))];
  return <div className="page"><PageHeader title="Встречи" subtitle="Управляйте встречами и созвонами"/><div className="toolbar"><SearchBox value={query} onChange={setQuery} label="Поиск встреч"/><Segments value={period} onChange={setPeriod} label="Период встреч" options={[{ value: 'upcoming', label: 'Ближайшие' }, { value: 'today', label: 'Сегодня' }]}/><NewButton onClick={() => actions.create('meeting')}>Запланировать встречу</NewButton></div>{result.error && <Alert>{result.error}</Alert>}
    {meeting ? <div className="meetings-layout"><div className="meeting-timeline">{days.map(day => <section key={day}><h2>{dateLabel(day, today)}<span className="count">{items.filter(item => item.date === day).length}</span></h2>{items.slice(0, limit).filter(item => item.date === day).map(item => <button type="button" key={item.id} className={`meeting-tile ${item.id === meeting.id ? 'selected' : ''}`} onClick={() => setSelected(item.id)} aria-pressed={item.id === meeting.id}><span className="timeline-time">{item.time}</span><span><strong>{item.title}</strong><small>{item.location || (item.meeting_url ? 'Онлайн' : 'Место не указано')}</small></span><small>{durationLabel(item.duration_minutes)}</small></button>)}</section>)}{items.length > limit && <Button variant="ghost" onClick={() => setLimit(value => value + 30)}>Показать ещё</Button>}</div>
      <section className="meeting-details"><header><span>{dateLabel(meeting.date, today)}, {meeting.time}</span><IconButton icon={Pencil} label="Редактировать выбранную встречу" onClick={() => actions.edit(meeting)}/></header><h2>{meeting.title}</h2><div className="detail-chips"><span><Clock3 size={17}/>{durationLabel(meeting.duration_minutes)}</span>{meeting.location && <span><MapPin size={17}/>{meeting.location}</span>}</div>{meeting.description && <p className="meeting-description">{meeting.description}</p>}{meeting.meeting_url && <a className="button soft meeting-url" href={meeting.meeting_url} target="_blank" rel="noopener noreferrer"><LinkIcon size={18}/>Открыть ссылку</a>}
        <footer><Button variant="soft" onClick={() => actions.edit(meeting, true)}><CalendarDays size={18}/>Перенести</Button><Button variant="ghost" onClick={() => actions.remove(meeting)}><Trash2 size={18}/>Удалить встречу</Button></footer>
      </section></div> : <Empty action={<Button variant="soft" onClick={() => actions.create('meeting')}>Добавить встречу</Button>}>Встреч на этот период нет</Empty>}
  </div>;
}
