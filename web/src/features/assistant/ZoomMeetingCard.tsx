import { useEffect, useRef, useState } from 'react';
import { Video, Copy, ExternalLink } from 'lucide-react';
import type { ZoomMeeting } from '../../types/api';

export function validZoomJoinURL(raw?: string): boolean {
  if (!raw) return false;
  try {
    const u = new URL(raw); const h = u.hostname;
    return u.protocol === 'https:' && !u.username && !u.password && !u.port && !u.hash && !u.searchParams.has('zak') &&
      (h === 'zoom.us' || h.endsWith('.zoom.us') || h === 'zoomgov.com' || h.endsWith('.zoomgov.com')) && /^\/(j|w)\/.+/.test(u.pathname);
  } catch { return false; }
}

export function ZoomMeetingCard({ meeting }: { meeting: ZoomMeeting }) {
  const [notice, setNotice] = useState(''); const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(timer.current), []);
  const ended = meeting.status === 'ended'; const started = meeting.status === 'started';
  const available = validZoomJoinURL(meeting.join_url);
  const when = meeting.scheduled_at ? new Intl.DateTimeFormat('ru-RU', { timeZone: meeting.timezone, day: 'numeric', month: 'long', hour: '2-digit', minute: '2-digit', year: 'numeric' }).format(new Date(meeting.scheduled_at)) : '';
  async function copy() {
    try { await navigator.clipboard.writeText(meeting.join_url!); setNotice('Ссылка скопирована'); }
    catch { setNotice('Не удалось скопировать ссылку. Повторите попытку.'); }
    clearTimeout(timer.current); timer.current = setTimeout(() => setNotice(''), 3000);
  }
  return <section className={`zoom-meeting-card is-${meeting.status}`} aria-label={`Zoom: ${meeting.title}`}>
    <header><span className="zoom-card-icon"><Video size={22}/></span><div><strong>{meeting.title}</strong><span className="zoom-card-status">{ended ? 'Завершилась' : started ? 'Идёт' : 'Запланирована'}</span></div></header>
    {when && <time dateTime={meeting.scheduled_at}>{when} · {meeting.timezone}</time>}
    <p>{ended ? 'Встреча завершилась.' : started ? 'Встреча уже началась. Можете подключиться.' : 'Вы можете попробовать подключиться заранее, если это разрешено настройками конференции. Фактическое начало пока не подтверждено.'}</p>
    <div className="zoom-card-actions">{available && !ended ? <a className={`button ${started ? 'primary' : 'soft'}`} href={meeting.join_url} target="_blank" rel="noopener noreferrer"><ExternalLink size={16}/>Подключиться</a> : <button className="button soft" disabled><ExternalLink size={16}/>Подключиться</button>}
      <button type="button" className="button soft" disabled={!available} onClick={() => void copy()}><Copy size={16}/>Скопировать ссылку</button></div>
    {!available && !ended && <small>Ссылка для подключения пока недоступна.</small>}
    <span className="zoom-copy-notice" role="status">{notice}</span>
  </section>;
}
