import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react';
import * as eventsAPI from '../api/events';
import { health } from '../api/calendar';
import { errorMessage } from '../api/client';
import { dayKey } from '../features/calendar/dates';
import type { EventInput, EventPatch, Health, RhythmEvent } from '../types/api';

interface Data {
  events: RhythmEvent[]; health: Health | null; today: string; loading: boolean; error: string; revision: number;
  refresh: () => Promise<void>; create: (input: EventInput) => Promise<RhythmEvent>;
  accept: (items: RhythmEvent[]) => void;
  update: (id: number, input: EventPatch) => Promise<RhythmEvent>; remove: (id: number) => Promise<void>;
  restore: (id: number, force?: boolean) => Promise<RhythmEvent>;
}
const Context = createContext<Data | null>(null);
export function DataProvider({ children }: { children: ReactNode }) {
  const [events, setEvents] = useState<RhythmEvent[]>([]); const [info, setInfo] = useState<Health | null>(null);
  const [error, setError] = useState(''); const [loading, setLoading] = useState(true); const [revision, setRevision] = useState(0);
  const current = useRef<AbortController | null>(null);
  const refresh = useCallback(async () => {
    current.current?.abort(); const controller = new AbortController(); current.current = controller;
    setError('');
    try {
      const [items, status] = await Promise.all([eventsAPI.listEvents({}, controller.signal), health(controller.signal)]);
      if (!controller.signal.aborted) { setEvents(items); setInfo(status); setRevision(r => r + 1); }
    } catch (e) { if (!controller.signal.aborted) setError(errorMessage(e)); }
    finally { if (!controller.signal.aborted) setLoading(false); }
  }, []);
  useEffect(() => {
    const onFocus = () => { if (document.visibilityState !== 'hidden') void refresh(); };
    void refresh(); const timer = setInterval(() => void refresh(), 60000);
    window.addEventListener('focus', onFocus); document.addEventListener('visibilitychange', onFocus);
    return () => { clearInterval(timer); current.current?.abort(); window.removeEventListener('focus', onFocus); document.removeEventListener('visibilitychange', onFocus); };
  }, [refresh]);
  const create = async (input: EventInput) => { const item = await eventsAPI.createEvent(input); setEvents(current => [...current, item]); await refresh(); return item; };
  const update = async (id: number, input: EventPatch) => { const item = await eventsAPI.updateEvent(id, input); setEvents(current => current.map(event => event.id === id ? item : event)); await refresh(); return item; };
  const remove = async (id: number) => { await eventsAPI.deleteEvent(id); setEvents(current => current.filter(event => event.id !== id)); await refresh(); };
  const restore = async (id: number, force = false) => { const item = await eventsAPI.restoreEvent(id, force); setEvents(current => [...current.filter(event => event.id !== id), item]); await refresh(); return item; };
  const accept = (items: RhythmEvent[]) => setEvents(current => [...new Map([...current, ...items].map(event => [event.id, event])).values()]);
  return <Context value={{ events, health: info, today: info ? dayKey(info.now, info.timezone) : '', loading, error, revision, refresh, accept, create, update, remove, restore }}>{children}</Context>;
}
export function useData(): Data { const data = useContext(Context); if (!data) throw new Error('DataProvider missing'); return data; }
