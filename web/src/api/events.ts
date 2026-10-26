import { request } from './client';
import type { EventInput, EventList, EventPatch, EventQuery, RhythmEvent } from '../types/api';

export async function listEvents(query: EventQuery = {}, signal?: AbortSignal, onSearch?: (status: EventList['search']) => void): Promise<RhythmEvent[]> {
  const result: RhythmEvent[] = [];
  let offset = 0;
  do {
    const params = new URLSearchParams({ ...query, limit: '200', offset: String(offset) });
    const page = await request<EventList>(`/api/events?${params}`, { signal });
    onSearch?.(page.search);
    result.push(...page.events);
    offset += page.events.length;
    if (offset >= page.total) break;
    if (!page.events.length) throw new Error('Список изменился. Обновите данные.');
  } while (true);
  return result;
}
export const createEvent = (input: EventInput) => request<RhythmEvent>('/api/events', { method: 'POST', body: JSON.stringify(input) });
export const updateEvent = (id: number, input: EventPatch) => request<RhythmEvent>(`/api/events/${id}`, { method: 'PATCH', body: JSON.stringify(input) });
export const deleteEvent = (id: number) => request<void>(`/api/events/${id}`, { method: 'DELETE' });
export const restoreEvent = (id: number, force = false) => request<RhythmEvent>(`/api/events/${id}/restore`, { method: 'POST', body: JSON.stringify({ force }) });
export interface ImportResult { imported: number; skipped: number; total_rows: number }
export const importDemo = () => request<ImportResult>('/api/import-demo', { method: 'POST', body: '{}' });
export const importCSV = (file: File) => {
  const body = new FormData();
  body.append('file', file);
  return request<ImportResult>('/api/import-csv', { method: 'POST', body });
};
