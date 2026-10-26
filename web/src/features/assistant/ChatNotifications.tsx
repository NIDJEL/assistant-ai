import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { useAuth } from '../../app/AuthContext';
import { request } from '../../api/client';
import type { Exchange } from '../../types/api';

const ChatNotifications = createContext<{ exchanges: Exchange[]; loading: boolean; markRead: (ids: string | string[]) => Promise<void> }>({ exchanges: [], loading: false, markRead: async () => {} });
export const useChatNotifications = () => useContext(ChatNotifications);

export function ChatNotificationsProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth(); const { pathname } = useLocation(); const [exchanges, setExchanges] = useState<Exchange[]>([]);
  const [loading, setLoading] = useState(true);
  const account = useRef(user?.id); account.current = user?.id;
  const confirmed = useRef(new Set<string>()); const pending = useRef(new Map<string, Promise<void>>());
  useEffect(() => {
    const controller = new AbortController(); let polling = false; const scope = user?.id;
    confirmed.current = new Set(); pending.current = new Map(); setExchanges([]); setLoading(!!scope);
    if (!scope) return () => controller.abort();
    async function poll() {
      if (polling || controller.signal.aborted || document.visibilityState === 'hidden') return;
      polling = true;
      try {
        const data = await request<{ exchanges: Exchange[] }>('/api/chat/notifications', { signal: controller.signal });
        if (!controller.signal.aborted && account.current === scope && Array.isArray(data.exchanges)) {
          const messages = [...new Map(data.exchanges.map(exchange => {
            const id = exchange.response.request_id;
            if (exchange.read) confirmed.current.add(id);
            return [id, { ...exchange, read: !!exchange.read || confirmed.current.has(id) }] as const;
          })).values()];
          setExchanges(current => JSON.stringify(current) === JSON.stringify(messages) ? current : messages);
        }
      } catch { /* Persisted messages are retried on reconnect and focus. */ }
      finally { polling = false; if (!controller.signal.aborted && account.current === scope) setLoading(false); }
    }
    void poll(); const timer = setInterval(() => void poll(), 5000);
    window.addEventListener('focus', poll); document.addEventListener('visibilitychange', poll); window.addEventListener('online', poll);
    return () => { controller.abort(); clearInterval(timer); window.removeEventListener('focus', poll); document.removeEventListener('visibilitychange', poll); window.removeEventListener('online', poll); };
  }, [user?.id]);
  const markRead = useCallback(async (value: string | string[]) => {
    const scope = user?.id; if (!scope || account.current !== scope) return;
    const read = confirmed.current; const queue = pending.current;
    const ids = [...new Set(typeof value === 'string' ? [value] : value)].filter(id => id && !read.has(id));
    const waiting = ids.flatMap(id => queue.has(id) ? [queue.get(id)!] : []); const fresh = ids.filter(id => !queue.has(id));
    if (fresh.length) {
      const work = (async () => {
        try {
          for (let offset = 0; offset < fresh.length; offset += 200) {
            if (account.current !== scope) return;
            const batch = fresh.slice(offset, offset + 200);
            await request('/api/chat/notifications/read', { method: 'POST', body: JSON.stringify({ ids: batch }) });
            if (account.current !== scope) return;
            batch.forEach(id => read.add(id));
            setExchanges(current => current.map(exchange => read.has(exchange.response.request_id) ? { ...exchange, read: true } : exchange));
          }
        } finally { fresh.forEach(id => queue.delete(id)); }
      })();
      fresh.forEach(id => queue.set(id, work)); waiting.push(work);
    }
    await Promise.all(waiting);
  }, [user?.id]);
  const unread = exchanges.filter(x => !x.read); const ids = unread.map(x => x.response.request_id);
  return <ChatNotifications.Provider value={{ exchanges, loading, markRead }}>{children}{pathname !== '/' && unread.length > 0 && <Link className="chat-notice" to={`/#chat-message-${ids[0]}`} state={{ chatNotifications: ids }} aria-live="polite">Новые сообщения Гены: {unread.length}</Link>}</ChatNotifications.Provider>;
}
