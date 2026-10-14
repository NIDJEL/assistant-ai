import { useEffect, useState } from 'react';
import { Bell } from 'lucide-react';
import { usePreferences } from '../../app/PreferencesContext';
import { Button } from '../../components/UI';
import { notificationSupport, registerNotifications } from './notifications';

export function NotificationSettings() {
  const { preferences, update } = usePreferences();
  const [permission, setPermission] = useState(() => notificationSupport() ? Notification.permission : 'unsupported');
  const [error, setError] = useState('');
  useEffect(() => { const sync = () => setPermission(notificationSupport() ? Notification.permission : 'unsupported'); window.addEventListener('focus', sync); return () => window.removeEventListener('focus', sync); }, []);
  async function enable() {
    try {
      const value = await Notification.requestPermission(); setPermission(value);
      if (value === 'granted') { update({ browserNotifications: true }); await registerNotifications(); }
    } catch { setError('Не удалось включить системные уведомления. Напоминания внутри приложения продолжают работать.'); }
  }
  return <section className="settings-card"><h2><Bell size={22}/>Уведомления</h2>
    <p className="muted">Гена показывает напоминания внутри приложения, пока оно открыто. Системные уведомления можно включить на этом устройстве.</p>
    {permission === 'granted' ? <label className="switch-row"><span>Показывать системные уведомления</span><input type="checkbox" role="switch" checked={preferences.browserNotifications} onChange={e => update({ browserNotifications: e.target.checked })}/></label> : <Button variant="soft" disabled={permission === 'unsupported' || permission === 'denied'} onClick={() => void enable()}>Разрешить уведомления</Button>}
    {permission === 'denied' && <p className="muted">Браузер запретил уведомления. Разрешите их в настройках сайта, затем вернитесь сюда.</p>}
    {permission === 'unsupported' && <p className="muted">Системные уведомления недоступны в этом браузере или по этому адресу. Уведомления внутри приложения работают.</p>}
    {error && <p role="status">{error}</p>}
    <p className="muted">Чтобы получать напоминания при закрытом приложении, используйте подключённый Яндекс Календарь. Интервал задаётся в событии; канал доставки включается в настройках Яндекса.</p>
    <a href="https://yandex.ru/support/yandex-360/customers/calendar/web/ru/plan-events/notifications" target="_blank" rel="noreferrer">Настроить доставку уведомлений Яндекса</a>
  </section>;
}
