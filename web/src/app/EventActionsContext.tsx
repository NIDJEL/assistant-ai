import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react';
import { X } from 'lucide-react';
import { EventEditor, type EditorSpec } from '../features/events/EventEditor';
import { Dialog } from '../components/Dialog';
import { Alert, Button, IconButton } from '../components/UI';
import { MotionLink } from '../components/motion/MotionNavLink';
import { useData } from './DataContext';
import { errorMessage } from '../api/client';
import { request } from '../api/client';
import { useSearchParams } from 'react-router-dom';
import type { EventKind, RhythmEvent } from '../types/api';
import { Presence } from '../components/motion/Presence';
interface Actions {
  create: (kind: EventKind, date?: string, time?: string, preset?: { title: string; duration: number }) => void; edit: (event: RhythmEvent, focusDate?: boolean) => void;
  complete: (event: RhythmEvent) => Promise<void>; remove: (event: RhythmEvent) => void;
}
const Context = createContext<Actions | null>(null);
export function EventActionsProvider({ children }: { children: ReactNode }) {
  const data = useData(); const [editor, setEditor] = useState<EditorSpec | null>(null); const [deleting, setDeleting] = useState<RhythmEvent | null>(null);
  const [params,setParams] = useSearchParams(); const deepLink = params.get('event');
  useEffect(() => {
    if (!data.health || !deepLink || !/^\d+$/.test(deepLink)) return;
    const controller = new AbortController();
    void request<RhythmEvent>(`/api/events/${deepLink}`, { signal: controller.signal }).then(event => {
      if (controller.signal.aborted) return;
      setEditor({ kind: event.kind, event, nonce: Date.now() });
      setParams(current => { const next = new URLSearchParams(current); next.delete('event'); return next; }, { replace: true });
    }).catch(e => { if (!controller.signal.aborted) setError(errorMessage(e)); });
    return () => controller.abort();
  }, [deepLink, data.health?.timezone]);
  const [error, setError] = useState(''); const [busy, setBusy] = useState(false); const locked = useRef(false); const pendingIds = useRef(new Set<number>());
  const [notice, setNotice] = useState<{ event: RhythmEvent; deleted: boolean } | null>(null); const undoLock = useRef(false); const [undoBusy, setUndoBusy] = useState(false);
  useEffect(() => { if (!notice) return; const timer = window.setTimeout(() => setNotice(null), 8000); return () => window.clearTimeout(timer); }, [notice]);
  async function undo() {
    if (!notice || undoLock.current) return; undoLock.current = true; setUndoBusy(true);
    try { if (notice.deleted) await data.restore(notice.event.id); else await data.update(notice.event.id, { status: notice.event.status }); setNotice(null); }
    catch (error) { setError(errorMessage(error)); } finally { undoLock.current = false; setUndoBusy(false); }
  }
  const actions: Actions = {
    create: (kind, date, time, preset) => setEditor({ kind, date, time, ...preset, nonce: Date.now() }),
    edit: (event, focusDate) => setEditor({ kind: event.kind, event, focusDate, nonce: Date.now() }),
    remove: event => { setError(''); setDeleting(event); },
    complete: async event => {
      if (pendingIds.current.has(event.id)) return; pendingIds.current.add(event.id);
      try { await data.update(event.id, { status: event.status === 'done' ? 'open' : 'done' }); if (event.status === 'open') setNotice({ event, deleted: false }); }
      catch (e) { setError(errorMessage(e)); } finally { pendingIds.current.delete(event.id); }
    },
  };
  async function confirmDelete() {
    if (!deleting || locked.current) return; locked.current = true; setBusy(true);
    try { await data.remove(deleting.id); setNotice({ event: deleting, deleted: true }); setDeleting(null); setError(''); }
    catch (e) { setError(errorMessage(e)); } finally { setBusy(false); locked.current = false; }
  }
  return <Context value={actions}>{children}
    {notice && <div className="toast"><div className="action-notice" role="status"><span>{notice.deleted ? 'Запись перенесена в удалённые' : 'Запись выполнена и перенесена в архив'}</span><Button variant="soft" disabled={undoBusy} onClick={() => void undo()}>Отменить</Button>{notice.deleted && <MotionLink to="/archive?view=deleted" className="text-action" onClick={() => setNotice(null)}>Открыть архив</MotionLink>}<IconButton icon={X} label="Закрыть сообщение" onClick={() => setNotice(null)}/></div></div>}
    {error && !deleting && <div className="toast"><Alert action={<Button variant="ghost" onClick={() => setError('')}>Закрыть</Button>}>{error}</Alert></div>}
    <Presence present={!!editor}>{editor && <EventEditor key={editor.nonce} spec={editor} onClose={() => setEditor(null)}/>}</Presence>
    <Presence present={!!deleting}>{deleting && <Dialog title="Удалить запись?" onClose={() => setDeleting(null)}><p className="delete-title">{deleting.title}</p><p className="muted">Запись попадёт в раздел «Удалённые». Её можно восстановить в архиве.</p>{error && <Alert>{error}</Alert>}<footer className="dialog-actions"><Button variant="ghost" onClick={() => setDeleting(null)}>Отмена</Button><Button variant="danger" onClick={() => void confirmDelete()} disabled={busy}>{busy ? 'Удаляю…' : 'Удалить'}</Button></footer></Dialog>}</Presence>
  </Context>;
}
export function useEventActions() { const actions = useContext(Context); if (!actions) throw new Error('EventActionsProvider missing'); return actions; }
