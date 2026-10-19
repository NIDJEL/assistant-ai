import type { RhythmEvent } from '../../types/api';
export interface Person { id: string; name: string; avatar_url: string; email?: string; tags: string[] }
export interface FriendRequest { id: string; person: Person; status: string; created_at: string }
export interface People { profile: Person & { code: string; share_email: boolean; timezone: string; start_hour: number; end_hour: number }; friends: Person[]; incoming: FriendRequest[]; outgoing: FriendRequest[] }
export interface Slot { start: string; end: string }
export interface Availability { users: { user_id: string; status: 'free' | 'busy' | 'unknown'; complete: boolean }[]; slots: Slot[]; complete: boolean }
export interface Member extends Person { status: 'pending' | 'accepted' | 'declined' | 'cancelled'; version: number }
export interface Proposal { id: string; proposer_id: string; name: string; version: number; slots: Slot[]; status: string }
export interface SharedMeeting { event: RhythmEvent; organizer: Person; members: Member[]; proposals: Proposal[]; version: number; cancelled: boolean; timezone: string }
export interface SocialOperation { action: string; code?: string; id?: string; user_id?: string; tags?: string[]; event_id?: number; event?: unknown; friend_ids?: string[]; status?: string; version?: number; slots?: Slot[]; slot_index?: number; share_email?: boolean; timezone?: string; start_hour?: number; end_hour?: number }
export interface SocialResult { message: string; event_id?: number; id?: string }
export function localSlot(date: string, time: string, minutes: number, zone: string): Slot {
  // Resolve a civil time using the server's IANA zone, independent of the device.
  const parts = `${date}T${time}:00`; const guess = Date.parse(`${parts}Z`);
  const fmt = new Intl.DateTimeFormat('sv-SE', { timeZone: zone, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23' });
  let epoch = guess;
  for (let i = 0; i < 3; i++) { const shown = Date.parse(`${fmt.format(new Date(epoch)).replace(' ', 'T')}Z`); epoch += guess - shown; }
  if (fmt.format(new Date(epoch)).replace(' ', 'T') !== parts) throw new Error('В этом часовом поясе выбранное время не существует. Выберите другое.');
  return { start: new Date(epoch).toISOString(), end: new Date(epoch + minutes * 60000).toISOString() };
}
export function slotLabel(slot: Slot, zone: string) { const date = new Intl.DateTimeFormat('ru-RU', { timeZone: zone, day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' }); return `${date.format(new Date(slot.start))} – ${new Intl.DateTimeFormat('ru-RU', { timeZone: zone, hour: '2-digit', minute: '2-digit' }).format(new Date(slot.end))}`; }
