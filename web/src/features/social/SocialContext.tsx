import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react';
import { request, errorMessage } from '../../api/client';
import { useData } from '../../app/DataContext';
import type { Availability, People, SharedMeeting, Slot, SocialOperation, SocialResult } from './types';
interface SocialData { people: People | null; meetings: SharedMeeting[]; error: string; loading: boolean; refresh: () => Promise<void>; perform: (op: SocialOperation) => Promise<SocialResult>; check: (ids: string[], slot: Slot, eventId?: number) => Promise<Availability> }
const Context = createContext<SocialData | null>(null);
export function SocialProvider({ children }: { children: ReactNode }) {
  const [people, setPeople] = useState<People | null>(null); const [meetings, setMeetings] = useState<SharedMeeting[]>([]); const [error, setError] = useState(''); const [loading, setLoading] = useState(true);
  const data = useData(); const refreshData = data.refresh; const pending = useRef(new Map<string, string>()); const running = useRef(false); const mounted = useRef(true);
  const meetingStamp = useRef('');
  const refresh = useCallback(async () => {
    if (running.current) return; running.current = true;
    try { const [p, m] = await Promise.all([request<People>('/api/social/people'), request<{ meetings: SharedMeeting[] }>('/api/social/meetings')]); if (!p.profile || !Array.isArray(p.friends) || !Array.isArray(p.incoming) || !Array.isArray(p.outgoing) || !Array.isArray(m.meetings)) throw new Error('Не удалось загрузить друзей. Повторите попытку.'); if (mounted.current) { const stamp = JSON.stringify(m.meetings); if (meetingStamp.current && meetingStamp.current !== stamp) void refreshData(); meetingStamp.current = stamp; setPeople(current => JSON.stringify(current) === JSON.stringify(p) ? current : p); setMeetings(current => JSON.stringify(current) === stamp ? current : m.meetings); setError(''); } }
    catch (e) { if (mounted.current) setError(errorMessage(e)); }
    finally { running.current = false; if (mounted.current) setLoading(false); }
  }, []);
  useEffect(() => { mounted.current = true; void refresh(); const poll = () => { if (document.visibilityState !== 'hidden') void refresh(); }; const timer = setInterval(poll, 5000); window.addEventListener('focus', poll); window.addEventListener('online', poll); return () => { mounted.current = false; clearInterval(timer); window.removeEventListener('focus', poll); window.removeEventListener('online', poll); }; }, [refresh]);
  async function perform(op: SocialOperation) {
    const key = JSON.stringify(op); const requestId = pending.current.get(key) || crypto.randomUUID(); pending.current.set(key, requestId);
    const result = await request<SocialResult>('/api/social/operations', { method: 'POST', body: JSON.stringify({ ...op, request_id: requestId }) }, 45000);
    pending.current.delete(key); await Promise.all([refresh(), refreshData()]); return result;
  }
  const check = (ids: string[], slot: Slot, eventId = 0) => request<Availability>('/api/social/availability', { method: 'POST', body: JSON.stringify({ friend_ids: ids, slot, event_id: eventId }) }, 45000);
  return <Context value={{ people, meetings, error, loading, refresh, perform, check }}>{children}</Context>;
}
export function useSocial() { const value = useContext(Context); if (!value) throw new Error('SocialProvider missing'); return value; }
