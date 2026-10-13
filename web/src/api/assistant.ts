import { request } from './client';
import type { ChatInput, ChatResponse, History } from '../types/api';
export const sendChat = (input: ChatInput) => request<ChatResponse>('/api/chat', { method: 'POST', body: JSON.stringify(input) });
export const history = (sessionId: string, signal?: AbortSignal) => request<History>(`/api/history/${sessionId}`, { signal });
