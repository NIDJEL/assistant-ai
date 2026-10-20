export type EventKind = 'task' | 'meeting' | 'reminder';
export type EventStatus = 'open' | 'done';
export type Priority = 'normal' | 'low' | 'high';
export type Action = 'create' | 'list' | 'search' | 'update' | 'complete' | 'delete' | 'help' | 'clarify' | 'cancel' | 'restore';
export interface Subtask { id: string; title: string; done: boolean }
export interface Participant { name: string; email?: string }
export interface Recurrence { frequency: 'minute' | 'hour' | 'day' | 'week' | 'month' | 'year'; interval: number; weekdays?: number[]; day_of_month?: number; month?: number; local_time?: string; timezone: string; anchor?: string }
export interface EventDetails { recurrence?: Recurrence | null; tags?: string[]; subtasks?: Subtask[]; participants?: Participant[]; reminder_enabled?: boolean; reminder_minutes?: number; start_second?: number }
export interface RhythmEvent extends EventDetails {
	 display_title?: string; organizer_name?: string;
  organizer_id?: string; collaborative?: boolean; meeting_version?: number;
  occurrence_at?: string; last_notification_at?: string;
  id: number; kind: EventKind; title: string; description: string;
  date: string | null; time: string | null; duration_minutes: number;
  status: EventStatus; priority: Priority; location: string; meeting_url: string;
  created_at: string; updated_at: string; source_key: string | null;
  deleted_at?: string;
  external_calendar_id: string | null; score?: number;
}
export interface EventInput extends EventDetails {
  kind: EventKind; title: string; description?: string; date?: string | null;
  time?: string | null; duration_minutes?: number; status?: EventStatus;
  priority?: Priority; location?: string; meeting_url?: string; force?: boolean;
}
export type EventPatch = Partial<Omit<EventInput, 'kind'>>;
export interface EventList { events: RhythmEvent[]; total: number; offset: number; limit: number }
export interface EventQuery { start?: string; end?: string; kind?: EventKind; status?: EventStatus; q?: string; deleted?: string }
export interface Health { status: string; backend: string; events: number; timezone: string; now: string; nlp_provider: string; schema_version: number }
export interface PlanningSlot { start: string; end: string }
export interface ChatChoice { token: string; action: 'slot' | 'more' | 'cancel' | 'meeting' | 'confirm' | 'new'; index?: number; slot?: PlanningSlot }
export interface SchedulingCard { token: string; kind: 'slots' | 'meetings' | 'existing' | 'availability' | 'confirm'; title: string; requested?: PlanningSlot; slots?: PlanningSlot[]; meetings?: RhythmEvent[]; complete: boolean; timezone: string; today: string }
export interface ChatInput { message: string; session_id: string; request_id: string; source?: 'text' | 'voice'; choice?: ChatChoice }
export interface ChatResponse {
	 scheduling?: SchedulingCard;
  social?: { kind: string; event_id?: number };
  zoom_meetings?: ZoomMeeting[];
  reply: string; action: Action; events: RhythmEvent[]; candidates: RhythmEvent[] | null;
  conflicts: RhythmEvent[] | null; total?: number; session_id: string;
  request_id: string; provider: string; timezone: string; pending: boolean;
  suggestions?: string[];
}
export interface ZoomMeeting { event_id: number; title: string; scheduled_at?: string; timezone: string; status: 'scheduled' | 'started' | 'ended'; join_url?: string; run_id?: string }
export interface Exchange { message: string; response: ChatResponse; created_at: string; notification?: boolean; read?: boolean }
export interface History { exchanges: Exchange[]; state: { pending?: unknown; last_ids?: number[] } }
