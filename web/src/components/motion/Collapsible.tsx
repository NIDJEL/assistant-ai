import { useEffect, useLayoutEffect, useRef, type ReactNode } from 'react';
import { Presence, usePresent } from './Presence';
import { exitDuration, motionEase, reducedMotion } from './motion';

function CollapseSurface({ children, animateInitial }: { children: ReactNode; animateInitial: boolean }) {
  const present = usePresent(); const ref = useRef<HTMLDivElement>(null); const animation = useRef<Animation | null>(null);
  const first = useRef(true);
  useLayoutEffect(() => {
    const node = ref.current!;
    if (first.current && present && !animateInitial) { first.current = false; node.style.overflow = 'visible'; return; }
    const from = first.current ? 0 : node.getBoundingClientRect().height;
    first.current = false; animation.current?.cancel();
    if (reducedMotion() || !node.animate) { node.style.height = present ? '' : '0px'; node.style.overflow = present ? 'visible' : 'hidden'; return; }
    node.style.overflow = 'hidden';
    const to = present ? node.scrollHeight : 0;
    const current = node.animate([{ height: `${from}px`, opacity: present ? .35 : 1 }, { height: `${to}px`, opacity: present ? 1 : 0 }], { duration: present ? 320 : exitDuration, easing: motionEase, fill: 'both' });
    animation.current = current;
    void current.finished.then(() => {
      if (animation.current !== current) return;
      node.style.height = present ? '' : '0px'; node.style.overflow = present ? 'visible' : 'hidden'; current.cancel(); animation.current = null;
    }).catch(() => {});
  }, [present]);
  useEffect(() => () => { animation.current?.cancel(); animation.current = null; first.current = true; }, []);
  return <div ref={ref} className="motion-collapse" data-open={present} aria-hidden={!present} inert={!present}><div>{children}</div></div>;
}
export function Collapsible({ open, children, animateInitial = true }: { open: boolean; children: ReactNode; animateInitial?: boolean }) {
  return <Presence present={open}><CollapseSurface animateInitial={animateInitial}>{children}</CollapseSurface></Presence>;
}
