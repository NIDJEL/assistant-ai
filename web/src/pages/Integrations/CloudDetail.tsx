import { Select } from '../../components/Select';
import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { ArrowLeft, RefreshCw, Trash2, CalendarDays, Video } from 'lucide-react';
import { Button, IconButton } from '../../components/UI';
import { errorMessage } from '../../api/client';
import { disconnectCloud, getGoogleCalendars, selectGoogleCalendar, syncCloud, type CloudProvider, type GoogleCalendar } from '../../api/integrations';
import { refreshIntegrations, useYandexIntegration } from './useYandexIntegration';

export function CloudDetail({ provider, onBack }: { provider: CloudProvider; onBack: () => void }) {
  const { data, loaded, error: loadError } = useYandexIntegration();
  const connection = data?.connections.find(c => c.provider === provider);
  const configured = provider === 'google' ? data?.google_configured : data?.zoom_configured;
  const linked = !!connection && connection.status !== 'disconnected';
  const authorized = linked && connection.status !== 'reauthorization_required';
  const [busy, setBusy] = useState(false); const [error, setError] = useState(''); const [message, setMessage] = useState('');
  const [calendars, setCalendars] = useState<GoogleCalendar[]>([]); const [selected, setSelected] = useState(''); const [saved, setSaved] = useState('');
  const [params] = useSearchParams();
  const title = provider === 'google' ? 'Google Calendar' : 'Zoom';
  useEffect(() => {
    let active = true;
    if (provider === 'google' && authorized) getGoogleCalendars().then(v => { if (active) { setCalendars(v.calendars); setSelected(v.selected); setSaved(v.selected); } }).catch(e => { if (active) setError(errorMessage(e)); });
    return () => { active = false; };
  }, [provider, authorized]);
  async function run(disconnect = false) {
    setBusy(true); setError(''); setMessage('');
    try {
      if (disconnect) { await disconnectCloud(provider); setCalendars([]); setSelected(''); setSaved(''); setMessage('Подключение отключено. Дела сохранены в Гене.'); }
      else { if (provider === 'google' && selected !== saved) { await selectGoogleCalendar(selected); setSaved(selected); } await syncCloud(provider); setMessage('Синхронизация завершена.'); }
    } catch (e) { setError(errorMessage(e)); } finally { setBusy(false); refreshIntegrations(); }
  }
  const callbackError = params.has('error') ? (params.get('error') === 'oauth_cancelled' ? 'Вы отменили подключение. Можно попробовать снова.' : 'Не удалось завершить подключение. Повторите вход и разрешите необходимые права.') : '';
  const status = !loaded ? 'Проверяем подключение…' : !linked ? 'Не подключено' : connection?.status === 'reauthorization_required' ? 'Требуется повторный вход' : connection?.status === 'error' ? 'Ошибка синхронизации' : 'Подключено';
  return <div className="integration-detail">
    <div className="integration-detail-toolbar"><IconButton icon={ArrowLeft} label="Назад к виджетам" onClick={onBack}/></div>
    <header>{provider === 'google' ? <CalendarDays size={32}/> : <Video size={32}/>}<div><h2>{title}</h2><span className={`integration-state${authorized ? ' is-connected' : ''}`}><i className="dot" aria-hidden="true"/>{status}</span></div></header>
    {(error || loadError || callbackError) && <p className="integration-error" role="alert">{error || loadError || callbackError}</p>}
    {message && <p role="status">{message}</p>}
    <section className="sync-card"><h3>Подключение аккаунта</h3>
      <p>{provider === 'google' ? 'Дела Гены с датой отправляются в выбранный календарь. Изменения и новые события из Google появляются в Гене автоматически.' : 'Попросите Гену создать Zoom-встречу. Ссылка появится в карточке встречи; переносы обновляют конференцию.'}</p>
      {loaded && configured && <a className="button soft" href={`/api/auth/${provider}/start`}>{linked ? 'Подключить заново' : `Подключить ${title}`}</a>}
      {loaded && !configured && <p>Подключение пока не настроено на сервере.</p>}
      {connection?.last_error && <p role="alert">{connection.last_error}</p>}
      {connection?.last_sync && <p>Последняя синхронизация: {new Date(connection.last_sync).toLocaleString('ru-RU')}</p>}
      {!!connection?.pending_events && <p>Ожидают отправки: {connection.pending_events}</p>}
    </section>
    {provider === 'google' && authorized && <section className="sync-card"><h3>Календарь для синхронизации</h3>
      <label>Календарь <Select aria-label="Календарь Google" value={selected} onChange={setSelected} disabled={busy} options={[{ value: '', label: 'Выберите календарь' }, ...calendars.map(c => ({ value: c.id, label: `${c.summary}${c.primary ? ' (основной)' : ''}` }))]}/></label>
      <p>После выбора все дела Гены с датой будут добавлены в этот календарь. При первом импорте загружаются события за прошлый год и на два года вперёд.</p>
    </section>}
    <Button variant="soft" disabled={!authorized || busy || (provider === 'google' && !selected)} onClick={() => void run()}><RefreshCw size={19}/>{busy ? 'Подождите…' : selected !== saved ? 'Сохранить и синхронизировать' : 'Синхронизировать сейчас'}</Button>
    <Button variant="danger" disabled={!linked || busy} onClick={() => void run(true)}><Trash2 size={19}/>Отключить интеграцию</Button>
    <p className="integration-footnote">После отключения Гена прекращает обращения к аккаунту и удаляет сохранённые токены. События и конференции у провайдера сохраняются.</p>
  </div>;
}
