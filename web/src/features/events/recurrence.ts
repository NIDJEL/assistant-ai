import type { Recurrence } from '../../types/api';
const weekdays = ['воскресенье', 'понедельник', 'вторник', 'среду', 'четверг', 'пятницу', 'субботу'];
export function recurrenceLabel(rule?: Recurrence | null): string {
  if (!rule) return 'Без повтора';
  const { frequency: f, interval: n } = rule;
  let text = '';
  if (f === 'minute') text = n === 1 ? 'Каждую минуту' : `Каждые ${n} мин`;
  if (f === 'hour') text = n === 1 ? 'Каждый час' : `Каждые ${n} ч`;
  if (f === 'day') text = n === 1 ? 'Каждый день' : `Каждые ${n} дн`;
  if (f === 'week') {
    const days = rule.weekdays || [];
    if (n === 1 && [1, 2, 3, 4, 5].every(d => days.includes(d)) && days.length === 5) text = 'По будням';
    else if (n === 1 && days.length === 2 && days.includes(0) && days.includes(6)) text = 'По выходным';
    else text = `${n === 1 ? 'Каждую неделю' : `Каждые ${n} нед`}: ${days.map(d => weekdays[d]).join(', ')}`;
  }
  if (f === 'month') text = `${rule.day_of_month} числа ${n === 1 ? 'каждого месяца' : `каждые ${n} мес`}`;
  if (f === 'year') text = `${n === 1 ? 'Каждый год' : `Каждые ${n} лет`} ${rule.day_of_month}.${String(rule.month).padStart(2, '0')}`;
  return text + (rule.local_time ? `, ${rule.local_time}` : '')
    + (rule.count ? `, всего ${rule.count}` : '')
    + (rule.until ? `, до ${rule.until.split('-').reverse().join('.')} включительно` : '');
}
