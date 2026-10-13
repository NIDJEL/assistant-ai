import { useEffect, useId, useRef, type ReactNode } from 'react';
import { X } from 'lucide-react';
import { IconButton } from './UI';
import { usePresent } from './motion/Presence';
export function Dialog({ title, children, onClose, drawer = false, side = 'right' }: { title: string; children: ReactNode; onClose: () => void; drawer?: boolean; side?: 'left' | 'right' }) {
  const ref = useRef<HTMLDialogElement>(null); const id = useId(); const close = useRef(onClose); close.current = onClose;
  const present = usePresent();
  useEffect(() => {
    const dialog = ref.current!; const previous = document.activeElement as HTMLElement | null;
    dialog.showModal();
    dialog.querySelector<HTMLElement>('[data-autofocus]')?.focus();
    return () => { dialog.close(); if (previous?.isConnected) previous.focus(); };
  }, []);
  return <dialog ref={ref} className={`dialog ${drawer ? `drawer drawer-${side}` : ''} ${present ? '' : 'closing'}`} aria-labelledby={id} onCancel={e => { e.preventDefault(); if (present) close.current(); }} onClickCapture={e => { if (!present) { e.preventDefault(); e.stopPropagation(); } }} onSubmitCapture={e => { if (!present) { e.preventDefault(); e.stopPropagation(); } }} onKeyDown={e => {
    if (!present) { e.preventDefault(); return; }
    if (e.key !== 'Tab') return;
    const controls = [...e.currentTarget.querySelectorAll<HTMLElement>('button:not([disabled]), a[href], input:not([disabled]), textarea:not([disabled]), select:not([disabled]), [tabindex="0"]')].filter(element => element.getClientRects().length);
    const first = controls[0]; const last = controls.at(-1);
    if (!first || !last) { e.preventDefault(); return; }
    if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
    else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
  }} onClick={e => { if (e.target === e.currentTarget) close.current(); }}>
    <div className="dialog-heading"><h2 id={id}>{title}</h2><IconButton icon={X} label="Закрыть" onClick={onClose}/></div>{children}
  </dialog>;
}
