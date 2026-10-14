import { useState } from 'react';
import { UserRound, Volume2, PanelRight, Server, RefreshCw } from 'lucide-react';
import { useData } from '../../app/DataContext';
import { usePreferences } from '../../app/PreferencesContext';
import { PageHeader, Button } from '../../components/UI';
import { browserRecognition } from '../../features/voice/controller';
import { speak, stopSpeaking, ttsAvailable } from '../../features/voice/tts';
export function SettingsPage() {
  const { preferences, update } = usePreferences(); const { health, refresh } = useData();
  const [name, setName] = useState(preferences.name); const [saved, setSaved] = useState(false); const [refreshing, setRefreshing] = useState(false);
  async function reload() { setRefreshing(true); await refresh(); setRefreshing(false); }
  return <div className="page"><PageHeader title="Настройки" subtitle="Настройте Ритм под себя"/><div className="settings-grid">
    <section className="settings-card"><h2><UserRound size={22}/>Профиль</h2><form onSubmit={e => { e.preventDefault(); update({ name: name.trim() }); setSaved(true); }}><label>Ваше имя<input value={name} onChange={e => { setName(e.target.value); setSaved(false); }} required maxLength={50}/></label><Button type="submit" disabled={!name.trim()}>Сохранить имя</Button>{saved && <span className="saved" role="status">Сохранено</span>}</form></section>
    <section className="settings-card"><h2><Volume2 size={22}/>Голос</h2><label className="switch-row"><span>Озвучивать ответы</span><input type="checkbox" role="switch" checked={preferences.tts} onChange={e => { update({ tts: e.target.checked }); if (!e.target.checked) stopSpeaking(); }}/></label><p className="muted">Диктовка остаётся черновиком. Отправка — по Send или Enter.</p><dl className="settings-info"><div><dt>Распознавание речи</dt><dd>{browserRecognition() ? 'Поддерживается' : 'Недоступно в этом браузере'}</dd></div><div><dt>Озвучивание</dt><dd>{ttsAvailable() ? 'Поддерживается' : 'Недоступно в этом браузере'}</dd></div></dl>{ttsAvailable() && <Button variant="soft" onClick={() => speak('Готово. Ритм поможет запланировать ваши дела.')}><Volume2 size={18}/>Проверить голос</Button>}</section>
    <section className="settings-card"><h2><PanelRight size={22}/>Виджеты</h2><label className="switch-row"><span>Показывать панель на компьютере</span><input type="checkbox" role="switch" checked={preferences.widgets} onChange={e => update({ widgets: e.target.checked })}/></label><p className="muted">На телефоне и планшете панель открывается кнопкой «Виджеты».</p></section>
    <section className="settings-card"><h2><Server size={22}/>Данные</h2><dl className="settings-info"><div><dt>Часовой пояс</dt><dd>{health!.timezone}</dd></div><div><dt>Событий</dt><dd>{health!.events}</dd></div><div><dt>Сервер</dt><dd>{health!.backend === 'go' ? 'Go · доступен' : health!.backend}</dd></div></dl><Button variant="soft" onClick={() => void reload()} disabled={refreshing}><RefreshCw size={18}/>{refreshing ? 'Обновляю…' : 'Обновить данные'}</Button></section>
  </div></div>;
}
