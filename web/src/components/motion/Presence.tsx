import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react';
import { exitDuration, reducedMotion } from './motion';

const PresentContext = createContext(true);
export function usePresent() { return useContext(PresentContext); }

export function Presence({ present, children, duration = exitDuration }: { present: boolean; children: ReactNode; duration?: number }) {
  const [mounted, setMounted] = useState(present);
  const content = useRef(children);
  if (present) content.current = children;
  if (present && !mounted) setMounted(true);
  useEffect(() => {
    if (present) return;
    if (reducedMotion()) { setMounted(false); return; }
    const timer = window.setTimeout(() => setMounted(false), duration);
    return () => window.clearTimeout(timer);
  }, [present, duration]);
  return mounted || present ? <PresentContext value={present}>{present ? children : content.current}</PresentContext> : null;
}
