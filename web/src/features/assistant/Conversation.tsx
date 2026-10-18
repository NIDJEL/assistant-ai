import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { CalendarDays } from 'lucide-react';
import { BrandMark } from '../../components/Brand';
import { Alert, Button } from '../../components/UI';
import { useData } from '../../app/DataContext';
import { useAuth } from '../../app/AuthContext';
import { Avatar } from '../../components/Avatar';
import { ChatEventCard } from './ChatEventCard';
import { ZoomMeetingCard } from './ZoomMeetingCard';
import { useChatNotifications } from './ChatNotifications';
import { formatDay } from '../calendar/dates';
import { SpeakButton } from '../voice/SpeakButton';
import { usePreferences } from '../../app/PreferencesContext';
import type { ChatItem } from './useAssistant';
function Response({ item, onDraft, active }: { item: ChatItem; onDraft: (text: string) => void; active: boolean }) {
  const [expanded, setExpanded] = useState(false); const response = item.response!;
  const { events } = useData(); const current = new Map(events.map(event => [event.id, event])); const firstDate = current.get(response.events[0]?.id)?.date;
  return <><p className="reply-text">{response.reply}</p>{response.events.length > 0 && <div className="response-events">{response.events.slice(0, expanded ? 100 : 4).map(event => { const zoom = response.zoom_meetings?.find(card => card.event_id === event.id); if (zoom) return <ZoomMeetingCard key={event.id} meeting={zoom}/>; const live = current.get(event.id); return live ? <ChatEventCard key={event.id} event={live}/> : <div className="event-row compact" key={event.id}><span className="event-title">{event.title}</span><span className="muted">Запись удалена</span></div>; })}{response.events.length > 4 && <Button variant="ghost" onClick={() => setExpanded(current => !current)}>{expanded ? 'Свернуть' : `Ещё ${response.events.length - 4}`}</Button>}{firstDate && <Link className="calendar-link" to={`/calendar?date=${firstDate}`}><CalendarDays size={18}/>Открыть в календаре</Link>}</div>}
    {active && !!response.candidates?.length && <div className="reply-choices" aria-label="Выбор записи">{response.candidates.map(event => <Button key={event.id} variant="soft" onClick={() => onDraft(`#${event.id}`)}>{event.title} · {event.date ? formatDay(event.date, { day: 'numeric', month: 'short', year: 'numeric' }) : 'Без срока'}{event.time ? `, ${event.time}` : ''} · {{ task: 'Задача', meeting: 'Встреча', reminder: 'Напоминание' }[event.kind]}</Button>)}</div>}
    {active && response.pending && <div className="reply-choices">{response.suggestions?.map(text => <Button key={text} variant="soft" onClick={() => onDraft(text)}>{text}</Button>)}{response.reply.includes('«да»') && <Button variant="soft" onClick={() => onDraft('да')}>Да</Button>}<Button variant="ghost" onClick={() => onDraft('отмена')}>Отмена</Button></div>}
    </>;
}
export function Conversation({ items, busy, loading, error, onDraft }: { items: ChatItem[]; busy: boolean; loading: boolean; error: string; onDraft: (text: string) => void }) {
  const { markRead } = useChatNotifications(); const [readError, setReadError] = useState('');
  const { preferences } = usePreferences();
  const bottom = useRef<HTMLDivElement>(null); const { health } = useData(); const { user } = useAuth();
  useEffect(() => { bottom.current?.scrollIntoView({ block: 'end', behavior: 'instant' }); }, [items, busy]);
  const time = (instant: string) => new Intl.DateTimeFormat('ru-RU', { timeZone: health!.timezone, hour: '2-digit', minute: '2-digit' }).format(new Date(instant));
  return <div className="conversation" aria-label="Диалог с ассистентом">{error && <Alert>{error}</Alert>}{loading && <p role="status" className="muted">Загружаю историю…</p>}
    {!items.length && !loading && <div className="conversation-empty"><BrandMark/><p>Напишите или продиктуйте сообщение</p><Button variant="ghost" onClick={() => onDraft('Что у меня на эту неделю?')}>Что у меня на неделю?</Button></div>}
    {readError && <Alert>{readError}</Alert>}{items.map(item => <div className="exchange" key={item.id}>{item.notification && !item.read && <div className="chat-unread-marker"><span>Новое сообщение</span><Button variant="ghost" onClick={() => { void markRead(item.id).catch(() => setReadError('Не удалось отметить сообщение прочитанным. Повторите попытку.')); }}>Прочитано</Button></div>}{!item.notification && <div className="user-message"><Avatar name={user!.name} url={user!.avatar_url}/><div className="message-bubble"><p>{item.message}</p><small>{time(item.created)}</small></div></div>}
      <div className="assistant-message"><span className="assistant-avatar"><BrandMark small/></span><div className="message-bubble">{item.response ? <Response item={item} onDraft={onDraft} active={!item.notification && item.id === items.filter(x => !x.notification).at(-1)?.id}/> : item.error ? <Alert>{item.error}</Alert> : <span role="status">Думаю…</span>}{item.response && <div className="reply-footer"><SpeakButton id={item.id} text={item.response.reply} voice={preferences.ttsVoice}/><small className="reply-time">{time(item.created)}</small></div>}</div></div></div>)}<div ref={bottom}/>
  </div>;
}
