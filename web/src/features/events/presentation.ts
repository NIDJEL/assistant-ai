import type { EventKind, Priority, RhythmEvent } from '../../types/api';
export const kindLabels: Record<EventKind, string> = { task: 'Задача', meeting: 'Встреча', reminder: 'Напоминание' };
export const priorityLabels: Record<Priority, string> = { normal: 'Обычный', low: 'Низкий', high: 'Высокий' };
export const byMoment = (a: RhythmEvent, b: RhythmEvent) => (a.date || '9999').localeCompare(b.date || '9999') || (a.time || '').localeCompare(b.time || '') || a.id - b.id;
export const isArchived = (event: RhythmEvent, today: string) => event.status === 'done' || !!event.date && event.date < today;
