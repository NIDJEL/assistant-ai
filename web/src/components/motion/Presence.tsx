import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react';
import { exitDuration, reducedMotion } from './motion';

const PresentContext = createContext(true);
export function usePresent() { return useContext(PresentContext); }

export function Presence({ present, children }: { present: boolean; children: ReactNode }) {
  const [mounted, setMounted] = useState(present);
  const content = useRef(children);
  if (present) content.current = children;
  if (present && !mounted) setMounted(true);
  useEffect(() => {
    if (present) return;
    if (reducedMotion()) { setMounted(false); return; }
    const timer = window.setTimeout(() => setMounted(false), exitDuration);
    return () => window.clearTimeout(timer);
  }, [present]);
  return mounted || present ? <PresentContext value={present}>{present ? children : content.current}</PresentContext> : null;
}
