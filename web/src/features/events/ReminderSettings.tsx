import { Select } from '../../components/Select';
import { useId } from 'react';
import { Bell } from 'lucide-react';

export function reminderLabel(minutes: number) {
  if (!minutes) return 'В момент события';
  if (minutes % 1440 === 0) return `За ${minutes / 1440} дн.`;
  if (minutes % 60 === 0) return `За ${minutes / 60} ч.`;
  return `За ${minutes} мин.`;
}
export function ReminderSettings({ enabled, minutes, disabled, onChange }: { enabled: boolean; minutes: number; disabled?: boolean; onChange: (enabled: boolean, minutes: number) => void }) {
  const id = useId(); const options = [...new Set([0, 5, 15, 30, 60, 120, 1440, minutes])].sort((a, b) => a - b);
  return <fieldset className="reminder-settings"><legend><Bell size={17}/>Уведомление о событии</legend>
    <label className="switch-row"><span>Напомнить о событии</span><input type="checkbox" role="switch" checked={enabled} disabled={disabled} onChange={e => onChange(e.target.checked, minutes)}/></label>
    {enabled && <div className="form-grid"><label htmlFor={`${id}-preset`}>Когда напомнить<Select id={`${id}-preset`} aria-label="Когда напомнить" value={minutes} disabled={disabled} onChange={value => onChange(true, Number(value))} options={options.map(value => ({ value, label: reminderLabel(value) }))}/></label><label htmlFor={`${id}-minutes`}>За сколько минут<input id={`${id}-minutes`} type="number" min={0} max={10080} step={1} required value={minutes} disabled={disabled} onChange={e => onChange(true, Number(e.target.value))}/></label></div>}
    <p className="muted">Время события сохраняется. Уведомление придёт заранее. При подключённом Яндекс Календаре этот интервал передаётся вместе с событием.</p>
  </fieldset>;
}
