import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { useAuth } from '../../app/AuthContext';
import { request } from '../../api/client';
import type { Exchange } from '../../types/api';

const ChatNotifications = createContext<{ exchanges: Exchange[]; markRead: (id: string) => Promise<void> }>({ exchanges: [], markRead: async () => {} });
export const useChatNotifications = () => useContext(ChatNotifications);

export function ChatNotificationsProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth(); const { pathname } = useLocation(); const [exchanges, setExchanges] = useState<Exchange[]>([]);
  useEffect(() => {
    const controller = new AbortController(); let polling = false;
    async function poll() {
      if (polling || controller.signal.aborted || document.visibilityState === 'hidden') return;
      polling = true;
      try {
        const data = await request<{ exchanges: Exchange[] }>('/api/chat/notifications', { signal: controller.signal });
        if (!controller.signal.aborted && Array.isArray(data.exchanges)) setExchanges(current => JSON.stringify(current) === JSON.stringify(data.exchanges) ? current : data.exchanges);
      } catch { /* Persisted messages are retried on reconnect and focus. */ }
      finally { polling = false; }
    }
    setExchanges([]); void poll(); const timer = setInterval(() => void poll(), 5000);
    window.addEventListener('focus', poll); document.addEventListener('visibilitychange', poll); window.addEventListener('online', poll);
    return () => { controller.abort(); clearInterval(timer); window.removeEventListener('focus', poll); document.removeEventListener('visibilitychange', poll); window.removeEventListener('online', poll); };
  }, [user?.id]);
  const markRead = useCallback(async (id: string) => {
    await request('/api/chat/notifications/read', { method: 'POST', body: JSON.stringify({ ids: [id] }) });
    setExchanges(current => current.map(x => x.response.request_id === id ? { ...x, read: true } : x));
  }, []);
  const unread = exchanges.filter(x => !x.read).length;
  return <ChatNotifications.Provider value={{ exchanges, markRead }}>{children}{pathname !== '/' && unread > 0 && <Link className="zoom-chat-notice" to="/" aria-live="polite">Новые сообщения Гены: {unread}</Link>}</ChatNotifications.Provider>;
}
