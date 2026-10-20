import { useEffect, useLayoutEffect, useRef, type ReactNode } from 'react';
import { motionEase, reducedMotion } from './motion';

export function AutoHeight({ children, className = '' }: { children: ReactNode; className?: string }) {
  const outer = useRef<HTMLDivElement>(null); const inner = useRef<HTMLDivElement>(null); const height = useRef<number | null>(null); const animation = useRef<Animation | null>(null);
  useLayoutEffect(() => {
    if (!outer.current || !inner.current || typeof ResizeObserver === 'undefined') return;
    let frame = 0;
    const update = () => {
      const node = outer.current!; const next = inner.current!.getBoundingClientRect().height;
      if (height.current === null || reducedMotion() || !node.animate) { height.current = next; return; }
      if (Math.abs(height.current - next) < 1) return;
      const from = animation.current ? node.getBoundingClientRect().height : height.current; height.current = next;
      animation.current?.cancel();
      const current = node.animate([{ height: `${from}px` }, { height: `${next}px` }], { duration: 320, easing: motionEase, fill: 'both' }); animation.current = current;
      void current.finished.then(() => { if (animation.current === current) { current.cancel(); animation.current = null; } }).catch(() => {});
    };
    // Nested height transitions can resize an ancestor in this same delivery.
    // Coalesce measurements into the next frame instead of feeding the observer loop.
    const schedule = () => { cancelAnimationFrame(frame); frame = requestAnimationFrame(update); };
    update(); const observer = new ResizeObserver(schedule); observer.observe(inner.current);
    return () => { observer.disconnect(); cancelAnimationFrame(frame); animation.current?.cancel(); height.current = null; };
  }, []);
  useEffect(() => () => animation.current?.cancel(), []);
  return <div ref={outer} className={`auto-height ${className}`}><div ref={inner}>{children}</div></div>;
}
