import { useState } from 'react';
import { motionUpdate } from '../../components/motion/motion';
import { AnimatedList } from '../../components/motion/AnimatedList';
import { useSearchParams } from 'react-router-dom';
import { CalendarDays, Video, Settings, ArrowLeft, RefreshCw, CheckSquare, Users, Bell, Trash2, Plus, MoreHorizontal } from 'lucide-react';
import { PageHeader, SearchBox, Button, Empty, Segments, IconButton } from '../../components/UI';
import { disconnectYandex, syncYandex, type CalendarConnection } from '../../api/integrations';
import { errorMessage } from '../../api/client';
import { refreshIntegrations, useYandexIntegration } from './useYandexIntegration';
export const services = [
  { id: 'yandex', title: 'Яндекс Календарь', description: 'Ваши задачи, встречи и напоминания из приложения в Яндекс Календаре.' },
  { id: 'google', title: 'Google Calendar', description: 'Синхронизация событий с Google Календарём.' },
  { id: 'zoom', title: 'Zoom', description: 'Создавайте встречи в Zoom из Гена.' },
];
export function ServiceLogo({ id }: { id: string }) {
  return <span className={`service-logo service-${id}`} aria-hidden="true">{id === 'zoom' ? <Video size={32} fill="white"/> : id === 'google' ? <span className="google-calendar-logo"><b>31</b></span> : <span className="yandex-logo">Я</span>}</span>;
}
export function IntegrationDetail({ id, onBack }: { id: string; onBack: () => void }) {
  if (id === 'yandex') return <YandexDetail onBack={onBack}/>;
  const item = services.find(s => s.id === id) || services[0];
  return <div className="integration-detail"><div className="integration-detail-toolbar"><IconButton icon={ArrowLeft} label="Назад к виджетам" onClick={onBack}/><IconButton icon={MoreHorizontal} label="Дополнительные настройки интеграции — скоро" disabled/></div><header><ServiceLogo id={item.id}/><div><h2>{item.title}</h2><span className="integration-state"><i className="dot"/>Не подключено</span><p>{item.description}</p></div></header><section className="sync-card"><h3>Статус синхронизации</h3><div className="sync-state"><span className="sync-icon"><RefreshCw size={29}/></span><div><strong>Требуется настройка</strong><p>Подключение будет доступно на этапе P2.</p></div></div></section><section className="sync-card"><h3>Настройки синхронизации</h3>{[{ icon: CalendarDays, title: 'События и встречи', subtitle: 'Импортировать события' }, { icon: CheckSquare, title: 'Задачи из календаря', subtitle: 'Создавать задачи из событий' }, { icon: Users, title: 'Участники', subtitle: 'Импортировать участников' }, { icon: Bell, title: 'Напоминания', subtitle: 'Синхронизировать напоминания' }].map(({ icon: Icon, title, subtitle }) => <label className="sync-setting" key={title}><Icon size={22}/><span><strong>{title}</strong><small>{subtitle}</small></span><span className="switch-row"><input aria-label={title} type="checkbox" role="switch" disabled checked={false} readOnly/></span></label>)}</section><Button variant="soft" disabled><RefreshCw size={19}/>Синхронизировать сейчас</Button><Button variant="danger" disabled><Trash2 size={19}/>Отключить интеграцию</Button><p className="integration-footnote">Настройки станут доступны после подключения сервиса.</p></div>;
}
function connectionLabel(connection?: CalendarConnection, loaded = true) {
  if (!loaded) return 'Проверяем подключение…';
  switch (connection?.status) {
    case 'connected': return 'Подключено';
    case 'pending': return 'Подключаем…';
    case 'error': return 'Ошибка синхронизации';
    case 'reauthorization_required': return 'Требуется подключение';
    default: return 'Не подключено';
  }
}
function YandexConnectionStatus({ connection, loaded }: { connection?: CalendarConnection; loaded: boolean }) {
  const connected = loaded && connection?.status === 'connected';
  return <span className={`integration-state${connected ? ' is-connected' : ''}`}><i className="dot" aria-hidden="true"/>{connectionLabel(connection, loaded)}</span>;
}
function YandexDetail({ onBack }: { onBack: () => void }) {
  const { connection, configured, loaded, error: loadError } = useYandexIntegration();
  const [busy, setBusy] = useState(false); const [error, setError] = useState(''); const [params] = useSearchParams();
  const linked = !!connection && connection.status !== 'disconnected';
  const authorized = linked && connection.status !== 'reauthorization_required';
  const callbackErrors: Record<string, string> = {
    oauth_cancelled: 'Подключение отменено. Вы можете попробовать снова.',
    oauth_invalid: 'Не удалось завершить подключение. Попробуйте снова.',
    oauth_account_exists: 'Этот аккаунт Яндекса уже связан с другим аккаунтом Гена.',
    calendar_permission: 'Разрешите доступ к календарю на странице Яндекса.',
    calendar_unavailable: 'Подключение Яндекс Календаря не настроено на сервере.',
  };
  const callbackError = callbackErrors[params.get('error') || ''];
  async function run(disconnect = false) {
    if (busy) return; setBusy(true); setError('');
    try { if (disconnect) await disconnectYandex(); else await syncYandex(); }
    catch (e) { setError(errorMessage(e)); }
    finally { refreshIntegrations(); setBusy(false); }
  }
  return <div className="integration-detail"><div className="integration-detail-toolbar"><IconButton icon={ArrowLeft} label="Назад к виджетам" onClick={onBack}/></div>
    <header><ServiceLogo id="yandex"/><div><h2>Яндекс Календарь</h2><YandexConnectionStatus connection={connection} loaded={loaded}/><p>{services[0].description}</p></div></header>
    {(error || loadError || callbackError) && <p className="integration-error" role="alert">{error || loadError || callbackError}</p>}
    <section className="sync-card"><h3>Статус синхронизации</h3><div className="sync-state"><span className="sync-icon"><RefreshCw size={29}/></span><div><strong>{connection?.status === 'connected' ? 'Автоматическая синхронизация' : linked ? 'Подключение календаря' : 'Требуется подключение'}</strong><p>{connection?.last_error || (authorized ? 'Изменения отправляются автоматически.' : 'Разрешите Гене добавлять ваши дела в календарь.')}</p></div></div>
      {connection?.last_sync && <p className="integration-footnote">Последняя синхронизация: {new Date(connection.last_sync).toLocaleString('ru-RU')}</p>}
      {!!connection?.pending_events && <p className="integration-footnote">Ожидают отправки: {connection.pending_events}</p>}
    </section>
    {loaded && configured && (!authorized || connection?.status === 'error') && <a className="button soft" href="/api/auth/yandex/start?calendar=1"><CalendarDays size={19}/>{linked ? 'Подключить заново' : 'Подключить Яндекс Календарь'}</a>}
    {loaded && !configured && <p className="integration-footnote">Для подключения настройте приложение Яндекс ID на сервере с доступом к календарю.</p>}
    <section className="sync-card"><h3>Что появится в Яндексе</h3>{[{ icon: CalendarDays, title: 'Календарь «Гена»', subtitle: 'Отдельный календарь для ваших дел' }, { icon: CheckSquare, title: 'Задачи и встречи', subtitle: 'Все дела с датой, включая уже созданные' }, { icon: Bell, title: 'Напоминания', subtitle: 'С датой, временем и уведомлением' }, { icon: RefreshCw, title: 'Изменения и удаления', subtitle: 'Переносы и завершения обновляют запись' }].map(({ icon: Icon, title, subtitle }) => <div className="sync-setting" key={title}><Icon size={22}/><span><strong>{title}</strong><small>{subtitle}</small></span></div>)}</section>
    <Button variant="soft" disabled={!authorized || busy} onClick={() => void run()}><RefreshCw size={19}/>{busy ? 'Подождите…' : 'Синхронизировать сейчас'}</Button>
    <Button variant="danger" disabled={!linked || busy} onClick={() => void run(true)}><Trash2 size={19}/>Отключить интеграцию</Button>
    <p className="integration-footnote">Дела без даты останутся в приложении. После отключения созданные события сохранятся в Яндексе.</p>
  </div>;
}
export function IntegrationsPage() {
  const [query, setQuery] = useState(''); const [filter, setFilter] = useState<'all' | 'connected' | 'available'>('all'); const [params, setParams] = useSearchParams(); const selected = params.get('service');
  const { connection, loaded, error } = useYandexIntegration();
  const linked = !!connection && connection.status !== 'disconnected';
  const items = services.filter(item => item.title.toLowerCase().includes(query.toLowerCase().trim()) && (filter === 'all' || (filter === 'connected' ? item.id === 'yandex' && linked : item.id !== 'yandex' || !linked)));
  return <div className="page integrations-page"><PageHeader title="Интеграции" subtitle="Подключите нужные сервисы"/>{error && <p className="integration-error" role="alert">{error}</p>}<div className="toolbar"><SearchBox value={query} onChange={setQuery} label="Найти интеграцию" placeholder="Найти интеграцию..."/></div><div className="integration-filters"><Segments value={filter} onChange={setFilter} label="Фильтр интеграций" options={[{ value: 'all', label: 'Все' }, { value: 'connected', label: 'Подключено' }, { value: 'available', label: 'Доступно' }]}/></div><AnimatedList className="integration-grid motion-content" items={items} itemKey={item => item.id} renderItem={item => <section className={`integration-card ${selected === item.id ? 'prepared' : ''}`} key={item.id}><header><ServiceLogo id={item.id}/><div><h2>{item.title}</h2>{item.id === 'yandex' ? <YandexConnectionStatus connection={connection} loaded={loaded}/> : <span className="integration-state"><i className="dot" aria-hidden="true"/>Скоро</span>}</div></header><p>{item.description}</p><footer><Button variant={item.id === 'yandex' ? 'soft' : 'primary'} disabled={item.id !== 'yandex'} onClick={() => motionUpdate(() => setParams({ service: item.id }))}>{item.id === 'yandex' ? <Settings size={19}/> : <Plus size={19}/ >}{item.id === 'yandex' ? 'Настроить' : 'Подключить'}</Button></footer></section>}/>{!items.length && <Empty>{filter === 'connected' ? 'Подключённых интеграций пока нет' : 'Интеграция не найдена'}</Empty>}</div>;
}
