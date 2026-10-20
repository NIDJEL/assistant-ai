import { useEffect, useMemo, useRef, useState } from 'react';
import { useChatNotifications } from './ChatNotifications';
import { useAuth } from '../../app/AuthContext';
import * as api from '../../api/assistant';
import { errorMessage } from '../../api/client';
import { useData } from '../../app/DataContext';
import { stopSpeaking } from '../voice/tts';
import type { ChatInput, ChatResponse, ChatChoice } from '../../types/api';
export interface ChatItem { id: string; message: string; response?: ChatResponse; created: string; error?: string; notification?: boolean; read?: boolean }
function sessionId(userId: string): string {
 const key = `rhythm.session.v2.${userId}`;
 try { const stored = localStorage.getItem(key); if (stored && /^[0-9a-f]{8}-([0-9a-f]{4}-){3}[0-9a-f]{12}$/i.test(stored)) return stored; } catch { /* storage optional */ }
 const id = crypto.randomUUID(); try { localStorage.setItem(key, id); } catch { /* storage optional */ } return id;
}
export function useAssistant() {
  const notifications = useChatNotifications();
  const { user } = useAuth();
  const [session] = useState(() => sessionId(user!.id)); const [items, setItems] = useState<ChatItem[]>([]); const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true); const [historyError, setHistoryError] = useState('');
  const locked = useRef(false); const interacted = useRef(false); const pending = useRef<ChatInput | null>(null);
  const { refresh, accept } = useData();
  useEffect(() => {
    const controller = new AbortController();
    let polling = false;
    async function poll() {
      if (polling || document.visibilityState === 'hidden' || controller.signal.aborted) return;
      polling = true;
      try {
        const data = await api.history(session, controller.signal);
        if (controller.signal.aborted) return;
        setItems(current => {
          const saved = data.exchanges.map(e => ({ id: e.response.request_id, message: e.message, response: e.response, created: e.created_at }));
          const merged = interacted.current ? [...new Map([...saved, ...current.map(item => saved.find(x => x.id === item.id) || item)].map(item => [item.id, item])).values()] : saved;
          return JSON.stringify(current) === JSON.stringify(merged) ? current : merged;
        }); setHistoryError('');
      } catch (e) { if (!controller.signal.aborted) setHistoryError(errorMessage(e)); }
      finally { polling = false; if (!controller.signal.aborted) setLoading(false); }
    }
    void poll(); const timer = setInterval(() => void poll(), 5000);
    window.addEventListener('focus', poll); document.addEventListener('visibilitychange', poll);
    return () => { controller.abort(); clearInterval(timer); window.removeEventListener('focus', poll); document.removeEventListener('visibilitychange', poll); stopSpeaking(); };
  }, [session]);
  async function send(text: string, source: 'text' | 'voice' = 'text', choice?: ChatChoice): Promise<boolean> {
    const message = text.trim(); if (!message || locked.current || [...message].length > 2000) return false;
    locked.current = true; interacted.current = true; setBusy(true); stopSpeaking();
    const input = pending.current?.message === message && JSON.stringify(pending.current.choice) === JSON.stringify(choice) ? pending.current : { message, source, choice, session_id: session, request_id: crypto.randomUUID() };
    pending.current = input;
    setItems(current => [...current.filter(item => item.id !== input.request_id), { id: input.request_id, message, created: new Date().toISOString() }]);
    try {
      const response = await api.sendChat(input);
      accept(response.events);
      setItems(current => current.map(item => item.id === input.request_id ? { ...item, response } : item));
      pending.current = null; await refresh(); return true;
    } catch (e) {
      setItems(current => current.map(item => item.id === input.request_id ? { ...item, error: errorMessage(e) } : item)); return false;
    } finally { locked.current = false; setBusy(false); }
  }
  const conversation = useMemo(() => [...items, ...notifications.exchanges.map(e => ({ id: e.response.request_id, message: '', response: e.response, created: e.created_at, notification: true, read: e.read }))].sort((a,b) => Date.parse(a.created) - Date.parse(b.created)), [items, notifications.exchanges]);
  return { items: conversation, busy, loading, historyError, send };
}
