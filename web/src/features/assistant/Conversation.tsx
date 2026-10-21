import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { CalendarDays } from 'lucide-react';
import { BrandMark } from '../../components/Brand';
import { Alert, Button } from '../../components/UI';
import { useData } from '../../app/DataContext';
import { useAuth } from '../../app/AuthContext';
import { Avatar } from '../../components/Avatar';
import { ChatEventCard } from './ChatEventCard';
import { ZoomMeetingCard } from './ZoomMeetingCard';
import { SocialChatLink } from '../social/SharedMeetings';
import { useChatNotifications } from './ChatNotifications';
import { formatDay } from '../calendar/dates';
import { SpeakButton } from '../voice/SpeakButton';
import { usePreferences } from '../../app/PreferencesContext';
import { SchedulingCard } from './SchedulingCard';
import type { ChatChoice } from '../../types/api';
import type { ChatItem } from './useAssistant';
function Response({ item, onDraft, active, busy, onChoose }: { item: ChatItem; onDraft: (text: string) => void; active: boolean; busy: boolean; onChoose: (text: string, choice: ChatChoice) => void }) {
  const [expanded, setExpanded] = useState(false); const response = item.response!;
  const responseEvents = response.events || [];
  const { events } = useData(); const current = new Map(events.map(event => [event.id, event])); const firstDate = current.get(responseEvents[0]?.id)?.date;
  const otherZoom = response.zoom_meetings?.filter(card => !responseEvents.some(event => event.id === card.event_id)) || [];
  return <><p className="reply-text">{response.reply}</p>{(responseEvents.length > 0 || otherZoom.length > 0) && <div className="response-events">{responseEvents.slice(0, expanded ? 100 : 4).map(event => { const zoom = response.zoom_meetings?.find(card => card.event_id === event.id); if (zoom) return <ZoomMeetingCard key={event.id} meeting={zoom}/>; const live = current.get(event.id); return live || event.collaborative || item.notification ? <ChatEventCard key={event.id} event={live || event}/> : <div className="event-row compact" key={event.id}><span className="event-title">{event.display_title || event.title}</span><span className="muted">Запись удалена</span></div>; })}{otherZoom.map(card => <ZoomMeetingCard key={card.event_id} meeting={card}/>)}{responseEvents.length > 4 && <Button variant="ghost" onClick={() => setExpanded(current => !current)}>{expanded ? 'Свернуть' : `Ещё ${responseEvents.length - 4}`}</Button>}{firstDate && <Link className="calendar-link" to={`/calendar?date=${firstDate}`}><CalendarDays size={18}/>Открыть в календаре</Link>}</div>}
    {response.scheduling && <SchedulingCard card={response.scheduling} active={active} busy={busy} onChoose={onChoose}/>}
    {response.social && <SocialChatLink kind={response.social.kind} eventId={response.social.event_id}/>}{active && !!response.candidates?.length && <div className="reply-choices" aria-label="Выбор записи">{response.candidates.map(event => <Button key={event.id} variant="soft" onClick={() => onDraft(`#${event.id}`)}>{event.display_title || event.title} · {event.date ? formatDay(event.date, { day: 'numeric', month: 'short', year: 'numeric' }) : 'Без срока'}{event.time ? `, ${event.time}` : ''} · {{ task: 'Задача', meeting: 'Встреча', reminder: 'Напоминание' }[event.kind]}</Button>)}</div>}
    {active && response.pending && !response.scheduling && <div className="reply-choices">{response.suggestions?.map(text => <Button key={text} variant="soft" onClick={() => onDraft(text)}>{text}</Button>)}{response.reply.includes('«да»') && <Button variant="soft" onClick={() => onDraft('да')}>Да</Button>}<Button variant="ghost" onClick={() => onDraft('отмена')}>Отмена</Button></div>}
    </>;
}
export function Conversation({ items, busy, loading, error, onDraft, onChoose }: { items: ChatItem[]; busy: boolean; loading: boolean; error: string; onDraft: (text: string) => void; onChoose: (text: string, choice: ChatChoice) => void }) {
  const { markRead } = useChatNotifications(); const [readError, setReadError] = useState('');
  const { preferences } = usePreferences();
  const log = useRef<HTMLDivElement>(null); const followBottom = useRef(true); const initialized = useRef(false); const previousOwn = useRef(''); const opened = useRef('');
  const { health } = useData(); const { user } = useAuth(); const location = useLocation();
  const contentKey = items.map(item => `${item.id}:${!!item.response}:${item.error || ''}`).join('|');
  const own = items.filter(item => !item.notification).at(-1); const ownKey = own ? `${own.id}:${!!own.response}:${own.error || ''}` : '';
  const notifications = useMemo(() => items.filter(item => item.notification), [items]);
  const unreadIds = notifications.filter(item => !item.read).map(item => item.id); const unreadKey = unreadIds.join('|');
  const requestedIds = useMemo<string[]>(() => Array.isArray(location.state?.chatNotifications) ? location.state.chatNotifications.filter((id: unknown) => typeof id === 'string') : [], [location.state]);
  const acknowledge = useCallback((ids: string[]) => {
    if (!ids.length || document.visibilityState === 'hidden') return;
    void markRead(ids).then(() => setReadError('')).catch(() => setReadError('Не удалось отметить сообщения прочитанными.'));
  }, [markRead]);
  useLayoutEffect(() => {
    const root = log.current; if (!root || loading && !contentKey) return;
    const target = location.hash.startsWith('#chat-message-') ? document.getElementById(location.hash.slice(1)) : null;
    const key = `${location.key}:${location.hash}`;
    if (target && root.contains(target) && opened.current !== key) {
      target.scrollIntoView({ block: 'start', behavior: 'instant' }); opened.current = key;
    } else if (!initialized.current || ownKey !== previousOwn.current || followBottom.current) root.scrollTop = root.scrollHeight;
    initialized.current = true; previousOwn.current = ownKey;
    followBottom.current = root.scrollHeight - root.scrollTop - root.clientHeight < 80;
  }, [contentKey, ownKey, loading, location.key, location.hash]);
  useEffect(() => {
    if (loading || !requestedIds.length) return;
    const showOpened = () => acknowledge(notifications.filter(item => !item.read && requestedIds.includes(item.id)).map(item => item.id));
    showOpened(); window.addEventListener('focus', showOpened); document.addEventListener('visibilitychange', showOpened);
    return () => { window.removeEventListener('focus', showOpened); document.removeEventListener('visibilitychange', showOpened); };
  }, [loading, requestedIds, notifications, acknowledge]);
  useEffect(() => {
    const root = log.current; if (!root || loading || !unreadKey) return;
    const ids = unreadKey.split('|');
    if (typeof IntersectionObserver === 'undefined') { acknowledge(ids); return; }
    const visible = new Set<string>(); let frame = 0;
    const readVisible = () => { cancelAnimationFrame(frame); frame = requestAnimationFrame(() => acknowledge([...visible])); };
    const observer = new IntersectionObserver(entries => {
      for (const entry of entries) { const id = (entry.target as HTMLElement).dataset.notificationId!; if (entry.isIntersecting) visible.add(id); else visible.delete(id); }
      readVisible();
    }, { root, threshold: 0 });
    root.querySelectorAll<HTMLElement>('[data-notification-id]').forEach(element => { if (ids.includes(element.dataset.notificationId!)) observer.observe(element); });
    window.addEventListener('focus', readVisible); window.addEventListener('online', readVisible); document.addEventListener('visibilitychange', readVisible);
    return () => { observer.disconnect(); cancelAnimationFrame(frame); window.removeEventListener('focus', readVisible); window.removeEventListener('online', readVisible); document.removeEventListener('visibilitychange', readVisible); };
  }, [unreadKey, loading, acknowledge]);
  const time = (instant: string) => new Intl.DateTimeFormat('ru-RU', { timeZone: health!.timezone, hour: '2-digit', minute: '2-digit' }).format(new Date(instant));
  return <div ref={log} className="conversation" role="log" tabIndex={0} aria-label="Диалог с ассистентом" aria-live="polite" onScroll={event => { const root = event.currentTarget; followBottom.current = root.scrollHeight - root.scrollTop - root.clientHeight < 80; }}>{error && <Alert>{error}</Alert>}{loading && <p role="status" className="muted">Загружаю историю…</p>}
    {!items.length && !loading && <div className="conversation-empty"><BrandMark/><p>Напишите или продиктуйте сообщение</p><Button variant="ghost" onClick={() => onDraft('Что у меня на эту неделю?')}>Что у меня на неделю?</Button></div>}
    {readError && <Alert action={<Button variant="ghost" onClick={() => acknowledge(unreadIds)}>Повторить</Button>}>{readError}</Alert>}{items.map(item => <div className="exchange" id={`chat-message-${item.id}`} data-notification-id={item.notification ? item.id : undefined} key={item.id}>{!item.notification && <div className="user-message"><Avatar name={user!.name} url={user!.avatar_url}/><div className="message-bubble"><p>{item.message}</p><small>{time(item.created)}</small></div></div>}
      <div className="assistant-message"><span className="assistant-avatar"><BrandMark small/></span><div className="message-bubble">{item.response ? <Response item={item} onDraft={onDraft} busy={busy} onChoose={onChoose} active={!item.notification && item.id === own?.id}/> : item.error ? <Alert>{item.error}</Alert> : <span role="status">Думаю…</span>}{item.response && <div className="reply-footer"><SpeakButton id={item.id} text={item.response.reply} voice={preferences.ttsVoice}/><small className="reply-time">{time(item.created)}</small></div>}</div></div></div>)}
  </div>;
}
