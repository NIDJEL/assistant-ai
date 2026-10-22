import { PanelRight } from 'lucide-react';
import { Select } from '../../components/Select';
import { PageHeader } from '../../components/UI';
import { AccountCard } from './AccountCard';
import { DemoDataCard } from './DemoDataCard';
import { usePreferences, type MotionPreference } from '../../app/PreferencesContext';
import { NotificationSettings } from '../../features/notifications/NotificationSettings';
import { AssistantVoiceSettings } from '../../features/voice/AssistantVoiceSettings';
import { ThemePicker } from '../../components/ThemePicker';

export function SettingsPage() {
  const { preferences, update } = usePreferences();
  return <div className="page settings-page"><PageHeader title="Настройки"/><div className="settings-list">
    <AccountCard/>
    <AssistantVoiceSettings/>
    <section className="settings-card"><h2><PanelRight size={22}/>Интерфейс</h2>
      <div className="settings-row"><div className="settings-row-label"><strong>Тема оформления</strong><p>Светлый или тёмный интерфейс для всех разделов Гены.</p></div><div className="settings-row-control"><ThemePicker theme={preferences.theme} onChange={theme => update({ theme })}/></div></div>
      <label className="settings-row switch-row"><span className="settings-row-label"><strong>Показывать панель на компьютере</strong><small>Виджеты рядом с вашей страницей. На телефоне панель открывается кнопкой «Виджеты».</small></span><input type="checkbox" role="switch" checked={preferences.widgets} onChange={e => update({ widgets: e.target.checked })}/></label>
      <div className="settings-row"><div className="settings-row-label"><strong>Анимации интерфейса</strong><p>Плавное переключение разделов и элементов.</p></div><div className="settings-row-control"><Select aria-label="Анимации интерфейса" value={preferences.motion} onChange={value => update({ motion: value as MotionPreference })} options={[{ value: 'system', label: 'Как в системе' }, { value: 'full', label: 'Плавные' }, { value: 'reduced', label: 'Без анимаций' }]}/></div></div>
    </section>
    <NotificationSettings/>
    <DemoDataCard/>
  </div></div>;
}
