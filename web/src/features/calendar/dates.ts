export function dayKey(instant: string, timezone: string): string {
  const parts = new Intl.DateTimeFormat('en-CA', { timeZone: timezone, year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(new Date(instant));
  const get = (key: string) => parts.find(p => p.type === key)!.value;
  return `${get('year')}-${get('month')}-${get('day')}`;
}
export const civilDate = (day: string) => new Date(`${day}T12:00:00Z`);
export const isoDay = (date: Date) => date.toISOString().slice(0, 10);
export function addDays(day: string, amount: number): string {
  const date = civilDate(day); date.setUTCDate(date.getUTCDate() + amount);
  if (date.getUTCFullYear() < 1) return '0001-01-01'; if (date.getUTCFullYear() > 9999) return '9999-12-31'; return isoDay(date);
}
export const monthStart = (day: string) => `${day.slice(0, 7)}-01`;
export function addMonths(day: string, amount: number): string {
  const date = civilDate(monthStart(day)); date.setUTCMonth(date.getUTCMonth() + amount);
  return date.getUTCFullYear() < 1 || date.getUTCFullYear() > 9999 ? monthStart(day) : isoDay(date);
}
export function weekStart(day: string): string { return addDays(day, -((civilDate(day).getUTCDay() + 6) % 7)); }
export function monthDays(day: string): string[] {
  const first = weekStart(monthStart(day)); const last = civilDate(monthStart(day)); last.setUTCMonth(last.getUTCMonth() + 1); last.setUTCDate(0);
  const length = Math.ceil(((last.getTime() - civilDate(first).getTime()) / 86400000 + 1) / 7) * 7;
  return [...new Set(Array.from({ length }, (_, i) => addDays(first, i)))];
}
export function isCivilDay(value: string | null): value is string { if (!value || !/^\d{4}-\d{2}-\d{2}$/.test(value) || Number(value.slice(0, 4)) < 1) return false; const date = civilDate(value); return !Number.isNaN(date.getTime()) && isoDay(date) === value; }
export const formatDay = (day: string, options: Intl.DateTimeFormatOptions = { day: 'numeric', month: 'long' }) => new Intl.DateTimeFormat('ru-RU', { ...options, timeZone: 'UTC' }).format(civilDate(day));
export const formatMonth = (day: string) => formatDay(day, { month: 'long', year: 'numeric' }).replace(/ г\.$/, '');
export const durationLabel = (minutes: number) => minutes % 60 === 0 ? `${minutes / 60} ч` : `${minutes} мин`;
export function dateLabel(day: string | null, today: string): string {
  if (!day) return 'Без срока'; if (day === today) return 'Сегодня'; if (day === addDays(today, 1)) return 'Завтра';
  return formatDay(day, { day: 'numeric', month: 'short', ...(day.slice(0, 4) !== today.slice(0, 4) ? { year: 'numeric' } : {}) });
}
