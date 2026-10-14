import { useEffect, useRef, useState } from 'react';
import * as api from '../../api/assistant';
import { errorMessage } from '../../api/client';
import { useData } from '../../app/DataContext';
import { usePreferences } from '../../app/PreferencesContext';
import { speak, stopSpeaking } from '../voice/tts';
import type { ChatInput, ChatResponse } from '../../types/api';
export interface ChatItem { id: string; message: string; response?: ChatResponse; created: string; error?: string }
function sessionId(): string {
  try {
    for (const key of ['rhythm.session.v1', 'rhythm-session']) {
      const stored = localStorage.getItem(key);
      if (stored && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(stored)) {
        try { localStorage.setItem('rhythm.session.v1', stored); } catch { /* Keep the valid existing session. */ }
        return stored;
      }
    }
  } catch { /* In-memory session works when storage is disabled. */ }
  const id = crypto.randomUUID(); try { localStorage.setItem('rhythm.session.v1', id); } catch { /* optional persistence */ } return id;
}
export function useAssistant() {
  const [session] = useState(sessionId); const [items, setItems] = useState<ChatItem[]>([]); const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true); const [historyError, setHistoryError] = useState('');
  const locked = useRef(false); const interacted = useRef(false); const pending = useRef<ChatInput | null>(null);
  const { refresh, accept } = useData(); const { preferences } = usePreferences();
  useEffect(() => {
    const controller = new AbortController();
    api.history(session, controller.signal).then(data => {
      if (!interacted.current) setItems(data.exchanges.map(e => ({ id: e.response.request_id, message: e.message, response: e.response, created: e.created_at })));
    }).catch(e => { if (!controller.signal.aborted) setHistoryError(errorMessage(e)); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => { controller.abort(); stopSpeaking(); };
  }, [session]);
  async function send(text: string): Promise<boolean> {
    const message = text.trim(); if (!message || locked.current || [...message].length > 2000) return false;
    locked.current = true; interacted.current = true; setBusy(true); stopSpeaking();
    const input = pending.current?.message === message ? pending.current : { message, session_id: session, request_id: crypto.randomUUID() };
    pending.current = input;
    setItems(current => [...current.filter(item => item.id !== input.request_id), { id: input.request_id, message, created: new Date().toISOString() }]);
    try {
      const response = await api.sendChat(input);
      accept(response.events);
      setItems(current => current.map(item => item.id === input.request_id ? { ...item, response } : item));
      pending.current = null; if (preferences.tts) speak(response.reply); await refresh(); return true;
    } catch (e) {
      setItems(current => current.map(item => item.id === input.request_id ? { ...item, error: errorMessage(e) } : item)); return false;
    } finally { locked.current = false; setBusy(false); }
  }
  return { items, busy, loading, historyError, send };
}
