import { createContext, useContext, useRef, useState, type ReactNode } from 'react';
import { EventEditor, type EditorSpec } from '../features/events/EventEditor';
import { Dialog } from '../components/Dialog';
import { Alert, Button } from '../components/UI';
import { useData } from './DataContext';
import { errorMessage } from '../api/client';
import type { EventKind, RhythmEvent } from '../types/api';
interface Actions {
  create: (kind: EventKind, date?: string, time?: string, preset?: { title: string; duration: number }) => void; edit: (event: RhythmEvent, focusDate?: boolean) => void;
  complete: (event: RhythmEvent) => Promise<void>; remove: (event: RhythmEvent) => void;
}
const Context = createContext<Actions | null>(null);
export function EventActionsProvider({ children }: { children: ReactNode }) {
  const data = useData(); const [editor, setEditor] = useState<EditorSpec | null>(null); const [deleting, setDeleting] = useState<RhythmEvent | null>(null);
  const [error, setError] = useState(''); const [busy, setBusy] = useState(false); const locked = useRef(false); const pendingIds = useRef(new Set<number>());
  const actions: Actions = {
    create: (kind, date, time, preset) => setEditor({ kind, date, time, ...preset, nonce: Date.now() }),
    edit: (event, focusDate) => setEditor({ kind: event.kind, event, focusDate, nonce: Date.now() }),
    remove: event => { setError(''); setDeleting(event); },
    complete: async event => {
      if (pendingIds.current.has(event.id)) return; pendingIds.current.add(event.id);
      try { await data.update(event.id, { status: event.status === 'done' ? 'open' : 'done' }); }
      catch (e) { setError(errorMessage(e)); } finally { pendingIds.current.delete(event.id); }
    },
  };
  async function confirmDelete() {
    if (!deleting || locked.current) return; locked.current = true; setBusy(true);
    try { await data.remove(deleting.id); setDeleting(null); setError(''); }
    catch (e) { setError(errorMessage(e)); } finally { setBusy(false); locked.current = false; }
  }
  return <Context value={actions}>{children}
    {error && !deleting && <div className="toast"><Alert action={<Button variant="ghost" onClick={() => setError('')}>Закрыть</Button>}>{error}</Alert></div>}
    {editor && <EventEditor key={editor.nonce} spec={editor} onClose={() => setEditor(null)}/>}
    {deleting && <Dialog title="Удалить запись?" onClose={() => setDeleting(null)}><p className="delete-title">{deleting.title}</p>{error && <Alert>{error}</Alert>}<footer className="dialog-actions"><Button variant="ghost" onClick={() => setDeleting(null)}>Отмена</Button><Button variant="danger" onClick={() => void confirmDelete()} disabled={busy}>{busy ? 'Удаляю…' : 'Удалить'}</Button></footer></Dialog>}
  </Context>;
}
export function useEventActions() { const actions = useContext(Context); if (!actions) throw new Error('EventActionsProvider missing'); return actions; }
