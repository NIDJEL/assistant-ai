import { listEvents } from './events';
import { request } from './client';
import type { Health } from '../types/api';
// Calendar views compose the P0 event API; there is no second conflict implementation.
export const calendarEvents = (start: string, end: string, q = '', signal?: AbortSignal) => listEvents({ start, end, ...(q ? { q } : {}) }, signal);
export const health = (signal?: AbortSignal) => request<Health>('/health', { signal });
