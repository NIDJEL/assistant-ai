import { useEffect, useState } from 'react';
import { AccountCard } from './AccountCard';
import { Volume2, PanelRight, Server, RefreshCw, Link2 } from 'lucide-react';
import { MotionLink as Link } from '../../components/motion/MotionNavLink';
import { useData } from '../../app/DataContext';
import { usePreferences, type MotionPreference } from '../../app/PreferencesContext';
import { PageHeader, Button } from '../../components/UI';
import { browserRecognition } from '../../features/voice/controller';
import { SpeechLearningSettings } from '../../features/voice/SpeechLearningSettings';
import { NotificationSettings } from '../../features/notifications/NotificationSettings';
import { speak, stopSpeaking, ttsAvailable } from '../../features/voice/tts';
export function SettingsPage() {
  const { preferences, update } = usePreferences(); const { health, refresh } = useData();
  const [refreshing, setRefreshing] = useState(false);
  const [microphone, setMicrophone] = useState('Проверяется при начале диктовки');
  useEffect(() => {
    let disposed = false; let status: PermissionStatus | undefined;
    const changed = () => { if (!disposed && status) setMicrophone(status.state === 'granted' ? 'Разрешён' : status.state === 'denied' ? 'Доступ запрещён' : 'Требуется разрешение'); };
    if (navigator.permissions) void navigator.permissions.query({ name: 'microphone' as PermissionName }).then(value => { status = value; changed(); status.addEventListener('change', changed); }).catch(() => {});
    return () => { disposed = true; status?.removeEventListener('change', changed); };
  }, []);
  async function reload() { setRefreshing(true); await refresh(); setRefreshing(false); }
  return <div className="page"><PageHeader title="Настройки" subtitle="Настройте Гену под себя"/><div className="settings-grid"><AccountCard/>
    <section className="settings-card"><h2><Volume2 size={22}/>Голос</h2><label className="switch-row"><span>Озвучивать ответы</span><input type="checkbox" role="switch" checked={preferences.tts} onChange={e => { update({ tts: e.target.checked }); if (!e.target.checked) stopSpeaking(); }}/></label><p className="muted">Диктовка остаётся черновиком. Отправка — по Send или Enter.</p><dl className="settings-info"><div><dt>Язык</dt><dd>Русский · ru-RU</dd></div><div><dt>Микрофон</dt><dd>{microphone}</dd></div><div><dt>Распознавание речи</dt><dd>{browserRecognition() ? 'Поддерживается' : 'Недоступно в этом браузере'}</dd></div><div><dt>Озвучивание</dt><dd>{ttsAvailable() ? 'Поддерживается' : 'Недоступно в этом браузере'}</dd></div></dl>{ttsAvailable() && <Button variant="soft" onClick={() => speak('Готово. Гена поможет запланировать ваши дела.')}><Volume2 size={18}/>Проверить голос</Button>}</section>
    <SpeechLearningSettings/>
    <section className="settings-card"><h2><PanelRight size={22}/>Интерфейс</h2><label className="switch-row"><span>Показывать панель на компьютере</span><input type="checkbox" role="switch" checked={preferences.widgets} onChange={e => update({ widgets: e.target.checked })}/></label><p className="muted">Добавляйте виджеты кнопкой + и меняйте их порядок за ручку. На телефоне панель открывается кнопкой «Виджеты».</p><label className="motion-setting"><span>Анимации интерфейса</span><select aria-label="Анимации интерфейса" value={preferences.motion} onChange={e => update({ motion: e.target.value as MotionPreference })}><option value="system">Как в системе</option><option value="full">Плавные</option><option value="reduced">Без анимаций</option></select></label><p className="muted">«Плавные» включает движение в приложении даже при отключённых анимациях системы.</p></section>
    <NotificationSettings/>
    <section className="settings-card"><h2><Link2 size={22}/>Интеграции</h2><p className="muted">Добавляйте ваши дела в Яндекс Календарь автоматически.</p><Link className="button soft" to="/integrations?service=yandex">Настроить интеграции</Link></section>
    <section className="settings-card"><h2><Server size={22}/>Данные</h2><dl className="settings-info"><div><dt>Часовой пояс</dt><dd>{health!.timezone}</dd></div><div><dt>Событий</dt><dd>{health!.events}</dd></div><div><dt>Сервер</dt><dd>{health!.backend === 'go' ? 'Go · доступен' : health!.backend}</dd></div></dl><Button variant="soft" onClick={() => void reload()} disabled={refreshing}><RefreshCw size={18}/>{refreshing ? 'Обновляю…' : 'Обновить данные'}</Button></section>
  </div></div>;
}
