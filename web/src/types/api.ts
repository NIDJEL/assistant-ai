export type EventKind = 'task' | 'meeting' | 'reminder';
export type EventStatus = 'open' | 'done';
export type Priority = 'normal' | 'low' | 'high';
export type Action = 'create' | 'list' | 'search' | 'update' | 'complete' | 'delete' | 'help' | 'clarify' | 'cancel';
export interface Subtask { id: string; title: string; done: boolean }
export interface Participant { name: string; email?: string }
export interface EventDetails { tags?: string[]; subtasks?: Subtask[]; participants?: Participant[]; reminder_enabled?: boolean }
export interface RhythmEvent extends EventDetails {
  id: number; kind: EventKind; title: string; description: string;
  date: string | null; time: string | null; duration_minutes: number;
  status: EventStatus; priority: Priority; location: string; meeting_url: string;
  created_at: string; updated_at: string; source_key: string | null;
  external_calendar_id: string | null; score?: number;
}
export interface EventInput extends EventDetails {
  kind: EventKind; title: string; description?: string; date?: string | null;
  time?: string | null; duration_minutes?: number; status?: EventStatus;
  priority?: Priority; location?: string; meeting_url?: string; force?: boolean;
}
export type EventPatch = Partial<Omit<EventInput, 'kind'>>;
export interface EventList { events: RhythmEvent[]; total: number; offset: number; limit: number }
export interface EventQuery { start?: string; end?: string; kind?: EventKind; status?: EventStatus; q?: string }
export interface Health { status: string; backend: string; events: number; timezone: string; now: string; nlp_provider: string; schema_version: number }
export interface ChatInput { message: string; session_id: string; request_id: string }
export interface ChatResponse {
  reply: string; action: Action; events: RhythmEvent[]; candidates: RhythmEvent[] | null;
  conflicts: RhythmEvent[] | null; total?: number; session_id: string;
  request_id: string; provider: string; timezone: string; pending: boolean;
}
export interface Exchange { message: string; response: ChatResponse; created_at: string }
export interface History { exchanges: Exchange[]; state: { pending?: unknown; last_ids?: number[] } }
