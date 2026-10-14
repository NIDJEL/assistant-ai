import type { Recurrence } from '../../types/api';
import { recurrenceLabel } from './recurrence';
export function RecurrenceEditor({ value, date, time, timezone, disabled, onChange }: { value?: Recurrence | null; date?: string | null; time?: string | null; timezone: string; disabled?: boolean; onChange: (value: Recurrence | null) => void }) {
  function select(choice: string) {
    if (choice === 'current') return;
    if (choice === 'once') { onChange(null); return; }
    const day = date ? new Date(`${date}T12:00:00Z`) : new Date();
    const rule: Recurrence = { frequency: 'day', interval: 1, timezone, local_time: time || '09:00' };
    if (choice === 'workdays' || choice === 'weekly') { rule.frequency = 'week'; rule.weekdays = choice === 'workdays' ? [1, 2, 3, 4, 5] : [day.getUTCDay()]; }
    if (choice === 'monthly' || choice === 'yearly') { rule.frequency = choice === 'monthly' ? 'month' : 'year'; rule.day_of_month = day.getUTCDate(); if (choice === 'yearly') rule.month = day.getUTCMonth() + 1; }
    if (choice === 'hourly') { rule.frequency = 'hour'; delete rule.local_time; }
    onChange(rule);
  }
  return <label>Повтор<select aria-label="Повтор напоминания" value={value ? 'current' : 'once'} disabled={disabled} onChange={e => select(e.target.value)}>
    {value && <option value="current">{recurrenceLabel(value)}</option>}<option value="once">Без повтора</option><option value="daily">Каждый день</option><option value="workdays">По будням</option><option value="weekly">Каждую неделю</option><option value="monthly">Каждый месяц</option><option value="yearly">Каждый год</option><option value="hourly">Каждый час</option>
  </select></label>;
}
