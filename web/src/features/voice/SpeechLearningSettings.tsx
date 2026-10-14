import { Mic } from 'lucide-react';
import { usePreferences } from '../../app/PreferencesContext';
import { Button } from '../../components/UI';

export function SpeechLearningSettings() {
  const { preferences, update } = usePreferences(); const rules = preferences.speechCorrections;
  return <section className="settings-card"><h2><Mic size={22}/>Распознавание под вас</h2>
    <p className="muted">Если диктовка ошиблась в имени или слове, исправьте текст перед отправкой. Гена запомнит похожее произношение и учтёт исправление в таком же контексте.</p>
    <p className="muted">Команды, даты, время и отрицания не переучиваются. Названия ваших дел помогают распознаванию, если браузер поддерживает подсказки.</p>
    <p role="status">Сохранено исправлений: {rules.length}</p>
    {rules.length > 0 && <><ul>{rules.slice(-3).map((rule, index) => <li key={index}>{rule.from} → {rule.to}</li>)}</ul><Button variant="ghost" onClick={() => update({ speechCorrections: [] })}>Сбросить речевые исправления</Button></>}
  </section>;
}
