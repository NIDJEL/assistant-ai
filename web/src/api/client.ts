import type { RhythmEvent } from '../types/api';

export class ApiError extends Error {
  constructor(message: string, public code: string, public status: number, public conflicts: RhythmEvent[] = []) { super(message); this.name = 'ApiError'; }
}
export async function request<T>(path: string, init: RequestInit = {}, timeoutMs = 30000): Promise<T> {
  try {
    const signal = init.signal ? AbortSignal.any([init.signal, AbortSignal.timeout(timeoutMs)]) : AbortSignal.timeout(timeoutMs);
    const response = await fetch(path, { credentials: 'same-origin', ...init, signal, headers: { Accept: 'application/json', ...(init.body && !(init.body instanceof FormData) ? { 'Content-Type': 'application/json' } : {}), ...init.headers } });
    if (response.status === 204) return undefined as T;
    const body = await response.json().catch(() => null);
    if (!response.ok) { if (response.status === 401 && (!path.startsWith('/api/auth/') || path.endsWith('/onboarding') || (path.endsWith('/me') && init.method === 'PATCH'))) window.dispatchEvent(new Event('rhythm:unauthorized')); throw new ApiError(body?.error?.message || 'Не удалось выполнить действие', body?.error?.code || 'server_error', response.status, body?.conflicts || []); }
    if (body === null) throw new ApiError('Сервер прислал некорректный ответ. Попробуйте ещё раз.', 'invalid_response', 502);
    return body as T;
  } catch (error) {
    if (error instanceof ApiError || init.signal?.aborted) throw error;
    throw new ApiError('Нет связи с сервером. Проверьте соединение и повторите действие.', 'network_error', 0);
  }
}
export const errorMessage = (error: unknown): string => error instanceof Error ? error.message : 'Не удалось выполнить действие';
