import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { Volume2, CalendarDays } from 'lucide-react';
import { BrandMark } from '../../components/Brand';
import { Alert, Button, IconButton } from '../../components/UI';
import { useData } from '../../app/DataContext';
import { usePreferences } from '../../app/PreferencesContext';
import { ChatEventCard } from './ChatEventCard';
import { speak, ttsAvailable } from '../voice/tts';
import type { ChatItem } from './useAssistant';
function Response({ item, onDraft }: { item: ChatItem; onDraft: (text: string) => void }) {
  const [expanded, setExpanded] = useState(false); const response = item.response!;
  const { events } = useData(); const current = new Map(events.map(event => [event.id, event])); const firstDate = current.get(response.events[0]?.id)?.date;
  return <><p className="reply-text">{response.action === 'create' && response.events.length === 1 && response.events[0].kind === 'reminder' ? 'Готово! Добавил напоминание.' : response.reply}</p>{response.events.length > 0 && <div className="response-events">{response.events.slice(0, expanded ? 100 : 4).map(event => { const live = current.get(event.id); return live ? <ChatEventCard key={event.id} event={live}/> : <div className="event-row compact" key={event.id}><span className="event-title">{event.title}</span><span className="muted">Запись удалена</span></div>; })}{response.events.length > 4 && <Button variant="ghost" onClick={() => setExpanded(current => !current)}>{expanded ? 'Свернуть' : `Ещё ${response.events.length - 4}`}</Button>}{firstDate && <Link className="calendar-link" to={`/calendar?date=${firstDate}`}><CalendarDays size={18}/>Открыть в календаре</Link>}</div>}
    {!!response.candidates?.length && <div className="reply-choices">{response.candidates.map(event => <Button key={event.id} variant="soft" onClick={() => onDraft(`#${event.id}`)}>#{event.id} · {event.title}</Button>)}</div>}
    {response.pending && <div className="reply-choices">{response.reply.includes('«да»') && <Button variant="soft" onClick={() => onDraft('да')}>Да</Button>}<Button variant="ghost" onClick={() => onDraft('отмена')}>Отмена</Button></div>}
    {ttsAvailable() && <IconButton className="tts-button" icon={Volume2} label="Озвучить ответ" onClick={() => speak(response.reply)}/>}</>;
}
export function Conversation({ items, busy, loading, error, onDraft }: { items: ChatItem[]; busy: boolean; loading: boolean; error: string; onDraft: (text: string) => void }) {
  const bottom = useRef<HTMLDivElement>(null); const { health } = useData(); const { preferences } = usePreferences();
  useEffect(() => { bottom.current?.scrollIntoView({ block: 'end', behavior: 'instant' }); }, [items, busy]);
  const time = (instant: string) => new Intl.DateTimeFormat('ru-RU', { timeZone: health!.timezone, hour: '2-digit', minute: '2-digit' }).format(new Date(instant));
  return <div className="conversation" aria-label="Диалог с ассистентом">{error && <Alert>{error}</Alert>}{loading && <p role="status" className="muted">Загружаю историю…</p>}
    {!items.length && !loading && <div className="conversation-empty"><BrandMark/><p>Напишите или продиктуйте сообщение</p><Button variant="ghost" onClick={() => onDraft('Что у меня на эту неделю?')}>Что у меня на неделю?</Button></div>}
    {items.map(item => <div className="exchange" key={item.id}><div className="user-message"><span className="avatar" aria-hidden="true">{preferences.name.slice(0, 1).toUpperCase()}</span><div className="message-bubble"><p>{item.message}</p><small>{time(item.created)}</small></div></div>
      <div className="assistant-message"><span className="assistant-avatar"><BrandMark small/></span><div className="message-bubble">{item.response ? <Response item={item} onDraft={onDraft}/> : item.error ? <Alert>{item.error}</Alert> : <span role="status">Думаю…</span>}{item.response && <small className="reply-time">{time(item.created)}</small>}</div></div></div>)}<div ref={bottom}/>
  </div>;
}
