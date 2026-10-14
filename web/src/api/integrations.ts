import { request } from './client';
export interface CalendarConnection {
  provider: string;
  status: 'pending' | 'connected' | 'disconnected' | 'error' | 'reauthorization_required';
  last_sync: string | null;
  last_error: string;
  pending_events: number;
}
export interface IntegrationState { connections: CalendarConnection[]; yandex_configured: boolean }
export const getIntegrations = (signal?: AbortSignal) => request<IntegrationState>('/api/integrations', { signal });
export const syncYandex = () => request<IntegrationState>('/api/integrations/yandex/sync', { method: 'POST', body: '{}' }, 45000);
export const disconnectYandex = () => request<void>('/api/integrations/yandex', { method: 'DELETE' });
