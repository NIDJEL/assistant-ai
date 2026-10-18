import { request } from './client';
export interface CalendarConnection {
  provider: string;
  status: 'pending' | 'connected' | 'disconnected' | 'error' | 'reauthorization_required';
  last_sync: string | null;
  last_error: string;
  pending_events: number;
}
export interface IntegrationState { connections: CalendarConnection[]; yandex_configured: boolean; google_configured?: boolean; zoom_configured?: boolean }
export const getIntegrations = (signal?: AbortSignal) => request<IntegrationState>('/api/integrations', { signal });
export const syncYandex = () => request<IntegrationState>('/api/integrations/yandex/sync', { method: 'POST', body: '{}' }, 45000);
export const disconnectYandex = () => request<void>('/api/integrations/yandex', { method: 'DELETE' });
export const calendarOAuthURL = (source: 'onboarding' | 'integrations') => `/api/auth/yandex/start?calendar=1&source=${source}`;
export const startCalendarConnection = (source: 'onboarding' | 'integrations') => window.location.assign(calendarOAuthURL(source));
export type CloudProvider = 'google' | 'zoom';
export interface GoogleCalendar { id: string; summary: string; accessRole: string; primary: boolean }
export const syncCloud = (provider: CloudProvider) => request<IntegrationState>(`/api/integrations/${provider}/sync`, { method: 'POST', body: '{}' }, 45000);
export const disconnectCloud = (provider: CloudProvider) => request<void>(`/api/integrations/${provider}`, { method: 'DELETE' });
export const getGoogleCalendars = () => request<{ calendars: GoogleCalendar[]; selected: string }>('/api/integrations/google/calendars');
export const selectGoogleCalendar = (calendar_id: string) => request<{ selected: string }>('/api/integrations/google/calendar', { method: 'PUT', body: JSON.stringify({ calendar_id }) });
