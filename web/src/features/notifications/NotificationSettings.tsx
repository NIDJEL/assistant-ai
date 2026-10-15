import { useEffect, useState } from 'react';
import { Bell } from 'lucide-react';
import { useAuth } from '../../app/AuthContext';
import { errorMessage } from '../../api/client';
import { Button } from '../../components/UI';
import { registerNotifications } from './notifications';
import { deviceID, disablePush, enablePush, getDevices, getPushConfig, pushCapability, testPush, type Device } from './push';

export function NotificationSettings() {
  const { user } = useAuth(); const capability = pushCapability();
  const [permission, setPermission] = useState(() => 'Notification' in window ? Notification.permission : 'default');
  const [devices, setDevices] = useState<Device[]>([]); const [enabled, setEnabled] = useState(false);
  const [configured, setConfigured] = useState<boolean>(); const [busy, setBusy] = useState(false); const [message, setMessage] = useState('');
  async function refresh() {
    if (!user) return;
    const [config, result] = await Promise.all([getPushConfig(), getDevices()]);
    setConfigured(config.enabled); setDevices(result.subscriptions);
    setPermission('Notification' in window ? Notification.permission : 'default');
    const registration = capability === 'supported' ? await registerNotifications() : undefined;
    const subscription = await registration?.pushManager.getSubscription();
    setEnabled(!!subscription && (!subscription.expirationTime || subscription.expirationTime > Date.now()) && Notification.permission === 'granted' && result.subscriptions.some(device => device.id === deviceID(user.id) && !device.disabled_at));
  }
  useEffect(() => { let active = true; const sync = () => { if (active) void refresh().catch(e => { if (active) setMessage(errorMessage(e)); }); }; const invalid = (event: MessageEvent) => { if (event.data?.type === 'GENA_PUSH_INVALID') sync(); }; sync(); window.addEventListener('focus', sync); navigator.serviceWorker?.addEventListener('message',invalid); return () => { active = false; window.removeEventListener('focus', sync); navigator.serviceWorker?.removeEventListener('message',invalid); }; }, [user?.id]);
  async function change(enable: boolean) {
    if (!user || busy) return; setBusy(true); setMessage('');
    try { if (enable) await enablePush(user.id); else await disablePush(user.id); await refresh(); }
    catch (e) { setMessage(errorMessage(e)); }
    finally { setBusy(false); }
  }
  async function test() {
    const id = user && deviceID(user.id); if (!id || busy) return; setBusy(true);
    try { await testPush(id); setMessage('Проверка отправлена. Ожидайте уведомление на этом устройстве.'); }
    catch (e) { setMessage(errorMessage(e)); } finally { setBusy(false); }
  }
  return <section className="settings-card"><h2><Bell size={22}/>Системные уведомления</h2>
    <p className="muted">Гена может присылать напоминания, даже когда приложение закрыто.</p>
    {capability === 'ios-install' ? <><p>Чтобы Гена мог присылать напоминания:</p><ol><li>Нажмите «Поделиться» в Safari.</li><li>Выберите «На экран Домой».</li><li>Откройте Гену с домашнего экрана.</li><li>Нажмите «Включить уведомления».</li></ol></> : capability === 'unsupported' ? <p>Этот браузер не поддерживает фоновые уведомления. Напоминания внутри приложения работают, пока оно открыто.</p> : <>
      <p>{permission === 'denied' ? 'Браузер запретил уведомления' : enabled ? 'Уведомления включены' : 'Уведомления выключены'}</p>
      {permission === 'denied' && <p className="muted">Разрешите уведомления в настройках сайта, затем вернитесь сюда.</p>}
      <Button variant="soft" disabled={busy || configured !== true || (!enabled && permission === 'denied')} onClick={() => void change(!enabled)}>{enabled ? 'Выключить уведомления' : 'Включить уведомления'}</Button>
      {enabled && <Button variant="ghost" disabled={busy || !configured} onClick={() => void test()}>Отправить тестовое уведомление</Button>}
    </>}
    {configured === false && <p className="muted">Фоновые уведомления пока не настроены на сервере. Напоминания внутри приложения работают, пока оно открыто.</p>}
    {devices.some(device => !device.disabled_at) && <ul aria-label="Устройства с уведомлениями">{devices.filter(device => !device.disabled_at).map(device => <li key={device.id}>{device.platform}{user && device.id === deviceID(user.id) ? ' — этот браузер' : ''} · Активно</li>)}</ul>}
    {message && <p role="status">{message}</p>}
  </section>;
}
