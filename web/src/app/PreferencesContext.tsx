import { createContext, useContext, useState, type ReactNode } from 'react';
export interface Preferences { name: string; tts: boolean; widgets: boolean }
const defaults: Preferences = { name: 'Алексей', tts: false, widgets: true };
const key = 'rhythm.preferences.v1';
function read(): Preferences {
  try { const data = JSON.parse(localStorage.getItem(key) || '{}'); return { name: typeof data.name === 'string' && data.name.trim() ? data.name.slice(0, 50) : defaults.name, tts: typeof data.tts === 'boolean' ? data.tts : localStorage.getItem('rhythm-speak') === 'true', widgets: typeof data.widgets === 'boolean' ? data.widgets : true }; } catch { return defaults; }
}
const Context = createContext<{ preferences: Preferences; update: (patch: Partial<Preferences>) => void } | null>(null);
export function PreferencesProvider({ children }: { children: ReactNode }) {
  const [preferences, setPreferences] = useState(read);
  const update = (patch: Partial<Preferences>) => setPreferences(current => { const next = { ...current, ...patch }; try { localStorage.setItem(key, JSON.stringify(next)); } catch { /* Preferences still work for this tab. */ } return next; });
  return <Context value={{ preferences, update }}>{children}</Context>;
}
export function usePreferences() { const data = useContext(Context); if (!data) throw new Error('PreferencesProvider missing'); return data; }
