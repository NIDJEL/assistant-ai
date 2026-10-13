import type { RhythmEvent } from '../types/api';

export class ApiError extends Error {
  constructor(message: string, public code: string, public status: number, public conflicts: RhythmEvent[] = []) { super(message); this.name = 'ApiError'; }
}
export async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  try {
    const signal = init.signal ? AbortSignal.any([init.signal, AbortSignal.timeout(30000)]) : AbortSignal.timeout(30000);
    const response = await fetch(path, { ...init, signal, headers: { Accept: 'application/json', ...(init.body ? { 'Content-Type': 'application/json' } : {}), ...init.headers } });
    if (response.status === 204) return undefined as T;
    const body = await response.json();
    if (!response.ok) throw new ApiError(body.error?.message || 'Не удалось выполнить действие', body.error?.code || 'server_error', response.status, body.conflicts || []);
    return body as T;
  } catch (error) {
    if (error instanceof ApiError || init.signal?.aborted) throw error;
    throw new ApiError('Нет связи с сервером. Проверьте соединение и повторите действие.', 'network_error', 0);
  }
}
export const errorMessage = (error: unknown): string => error instanceof Error ? error.message : 'Не удалось выполнить действие';
