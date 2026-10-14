import { createContext, useContext, useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { request, errorMessage } from '../api/client';
export type WidgetKey = 'calendar' | 'today' | 'tasks' | 'meetings' | 'progress' | 'quick' | 'tags';
export interface WidgetLayout { order: WidgetKey[]; hidden: WidgetKey[]; settings: Partial<Record<WidgetKey, { limit: number }>> }
export type MotionPreference = 'system' | 'full' | 'reduced';
export interface Preferences { name: string; tts: boolean; widgets: boolean; navCollapsed: boolean; motion: MotionPreference; layouts: Record<string, WidgetLayout>; revision: number }
const defaults: Preferences = { name: 'Алексей', tts: false, widgets: true, navCollapsed: false, motion: 'system', layouts: {}, revision: 0 };
const key = 'rhythm.preferences.v1';
function normalize(value: Partial<Preferences> & { panel_visible?: boolean }): Preferences {
  return { name: typeof value.name === 'string' && value.name.trim() ? value.name.slice(0, 50) : defaults.name, tts: typeof value.tts === 'boolean' ? value.tts : localStorage.getItem('rhythm-speak') === 'true', widgets: typeof value.panel_visible === 'boolean' ? value.panel_visible : typeof value.widgets === 'boolean' ? value.widgets : true, navCollapsed: value.navCollapsed === true, motion: value.motion === 'full' || value.motion === 'reduced' ? value.motion : 'system', layouts: value.layouts && typeof value.layouts === 'object' ? value.layouts : {}, revision: Number.isSafeInteger(value.revision) && value.revision! >= 0 ? value.revision! : 0 };
}
function read(): Preferences { try { return normalize(JSON.parse(localStorage.getItem(key) || '{}')); } catch { return defaults; } }
function profile() {
  let id = localStorage.getItem('rhythm.profile.v1');
  if (!id || !/^[\da-f]{8}-([\da-f]{4}-){3}[\da-f]{12}$/i.test(id)) { id = crypto.randomUUID(); localStorage.setItem('rhythm.profile.v1', id); }
  return id;
}
const Context = createContext<{ preferences: Preferences; update: (patch: Partial<Preferences>) => void; ready: boolean; saving: boolean; error: string; retry: () => void } | null>(null);
export function PreferencesProvider({ children }: { children: ReactNode }) {
  const [preferences, setPreferences] = useState(read); const current = useRef(preferences);
  const [ready, setReady] = useState(false); const [saving, setSaving] = useState(false); const [error, setError] = useState('');
  const endpoint = useRef(`/api/preferences/${profile()}`); const queue = useRef(Promise.resolve());
  const pendingWrites = useRef(0);
  useLayoutEffect(() => {
    document.documentElement.dataset.motion = preferences.motion;
    return () => { delete document.documentElement.dataset.motion; };
  }, [preferences.motion]);
  function persist(next: Preferences) {
    pendingWrites.current++; setSaving(true);
    queue.current = queue.current.catch(() => {}).then(async () => {
      try { await request(endpoint.current, { method: 'PUT', body: JSON.stringify({ ...next, panel_visible: next.widgets }), keepalive: true }); setError(''); }
      catch (e) { setError(errorMessage(e)); }
    }).finally(() => { pendingWrites.current--; setSaving(pendingWrites.current > 0); });
  }
  useEffect(() => {
    let canceled = false;
    void request<Partial<Preferences>>(endpoint.current).then(value => {
      if (canceled) return;
      if (Object.keys(value).length) { const next = normalize(value); if (current.current.revision > next.revision) persist(current.current); else { current.current = next; setPreferences(next); localStorage.setItem(key, JSON.stringify(next)); } }
      else persist(current.current);
    }).catch(e => { if (!canceled) setError(errorMessage(e)); }).finally(() => { if (!canceled) setReady(true); });
    return () => { canceled = true; };
  }, []);
  const update = (patch: Partial<Preferences>) => {
    const next = { ...current.current, ...patch, revision: Math.max(Date.now(), current.current.revision + 1) }; current.current = next; setPreferences(next);
    localStorage.setItem(key, JSON.stringify(next)); persist(next);
  };
  return <Context value={{ preferences, update, ready, saving, error, retry: () => persist(current.current) }}>{children}</Context>;
}
export function usePreferences() { const data = useContext(Context); if (!data) throw new Error('PreferencesProvider missing'); return data; }
