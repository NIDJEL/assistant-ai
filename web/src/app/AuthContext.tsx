import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react';
import type { LegalAcceptance } from '../pages/Legal/documents';
import { ApiError, request } from '../api/client';
import { releasePushOnLogout, setPushUser } from '../features/notifications/push';
export interface User { id: string; email: string; name: string; avatar_url: string; onboarding_completed: boolean; providers: string[] }
interface Auth { user: User | null; loading: boolean; error: string; refresh: () => Promise<void>; login: (email: string, password: string, acceptance: LegalAcceptance) => Promise<User>; register: (name: string, email: string, password: string, confirmation: string, acceptance: LegalAcceptance) => Promise<User>; enableEmailLogin: (password: string, confirmation: string) => Promise<User>; logout: () => Promise<void>; updateProfile: (profile: { name?: string; avatar_url?: string }) => Promise<void>; completeOnboarding: () => Promise<void> }
const Context = createContext<Auth | null>(null);
export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null); const [loading, setLoading] = useState(true); const [error, setError] = useState('');
  const generation = useRef(0);
  const channel = useRef<BroadcastChannel | null>(null);
  useEffect(() => { if (!loading) void setPushUser(user?.id || null).catch(() => {}); }, [user?.id, loading]);
  async function refresh() { const version = ++generation.current; setError(''); try { const data = await request<{ user: User }>('/api/auth/me'); if (version === generation.current) setUser(data.user); } catch (e) { if (version !== generation.current) return; if (e instanceof ApiError && e.status === 401) setUser(null); else setError(e instanceof Error ? e.message : 'Не удалось проверить сессию'); } finally { if (version === generation.current) setLoading(false); } }
  useEffect(() => { void refresh(); const expired = () => { ++generation.current; setUser(null); setLoading(false); }; const changed = () => { setLoading(true); void refresh(); };
    if (typeof BroadcastChannel !== 'undefined') { channel.current = new BroadcastChannel('rhythm.auth'); channel.current.onmessage = changed; }
    window.addEventListener('rhythm:unauthorized', expired);
    return () => { channel.current?.close(); channel.current = null; window.removeEventListener('rhythm:unauthorized', expired); };
  }, []);
  async function authenticate(path: string, body: object) { ++generation.current; const data = await request<{ user: User }>(path, { method: 'POST', body: JSON.stringify(body) }); setUser(data.user); channel.current?.postMessage('changed'); return data.user; }
  const value: Auth = { user, loading, error, refresh,
    login: (email, password, acceptance) => authenticate('/api/auth/login', { email, password, ...acceptance }),
    register: (name, email, password, password_confirmation, acceptance) => authenticate('/api/auth/register', { name, email, password, password_confirmation, ...acceptance }),
    enableEmailLogin: (password, password_confirmation) => authenticate('/api/auth/email/enable', { password, password_confirmation }),
    logout: async () => {
      ++generation.current;
      // Optional browser notification cleanup must never prevent server logout.
      if (user) await releasePushOnLogout(user.id).catch(() => {});
      await request('/api/auth/logout', { method: 'POST', body: '{}' });
      setUser(null); channel.current?.postMessage('changed');
    },
    updateProfile: async profile => { const data = await request<{ user: User }>('/api/auth/me', { method: 'PATCH', body: JSON.stringify(profile) }); setUser(data.user); channel.current?.postMessage('changed'); },
    completeOnboarding: async () => { const data = await request<{ user: User }>('/api/auth/onboarding', { method: 'POST', body: '{}' }); setUser(data.user); },
  };
  return <Context value={value}>{children}</Context>;
}
export function useAuth() { const auth = useContext(Context); if (!auth) throw new Error('AuthProvider missing'); return auth; }
