import { useEffect, useLayoutEffect, useRef, type RefObject } from 'react';
import { motionEase, reducedMotion, viewTransitionActive } from '../../components/motion/motion';

type Position = { top: number; left: number; surface: HTMLElement; animation?: Animation };
// Animate the card inside its sortable wrapper, leaving drag transforms to dnd-kit.
export function useWidgetMotion(list: RefObject<HTMLDivElement | null>, dragging: boolean) {
  const previous = useRef(new Map<string, Position>()); const initialized = useRef(false); const wasDragging = useRef(false);
  const measure = useRef(() => {});
  measure.current = () => {
    const nodes = [...(list.current?.querySelectorAll<HTMLElement>(':scope > [data-widget]') || [])];
    const next = new Map<string, Position>();
    const skip = reducedMotion() || dragging || wasDragging.current || viewTransitionActive();
    for (const node of nodes) {
      const key = node.dataset.widget!; const surface = node.querySelector<HTMLElement>('.widget')!;
      const before = previous.current.get(key);
      const position: Position = { top: node.offsetTop, left: node.offsetLeft, surface, animation: before?.animation };
      if (skip) { before?.animation?.cancel(); position.animation = undefined; }
      else if (initialized.current && surface.animate && (!before || Math.abs(before.top - position.top) > .5 || Math.abs(before.left - position.left) > .5)) {
        // A new layout during an unfinished animation starts at the visible position.
        const matrix = new DOMMatrixReadOnly(getComputedStyle(surface).transform);
        const x = before ? before.left + matrix.m41 - position.left : 0;
        const y = before ? before.top + matrix.m42 - position.top : 16;
        const opacity = before ? getComputedStyle(surface).opacity : '0';
        before?.animation?.cancel();
        const animation = surface.animate([{ transform: `translate(${x}px, ${y}px)`, opacity }, { transform: 'translate(0, 0)', opacity: 1 }], { duration: 440, easing: motionEase, fill: 'both' });
        position.animation = animation;
        void animation.finished.then(() => {
          animation.cancel();
          if (previous.current.get(key)?.animation === animation) previous.current.get(key)!.animation = undefined;
        }).catch(() => {});
      }
      next.set(key, position);
    }
    for (const [key, position] of previous.current) if (!next.has(key)) position.animation?.cancel();
    previous.current = next; initialized.current = true; wasDragging.current = dragging;
  };
  useLayoutEffect(() => measure.current());
  useEffect(() => {
    if (!list.current || typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver(() => measure.current()); observer.observe(list.current);
    return () => observer.disconnect();
  }, [list]);
  useEffect(() => () => { for (const position of previous.current.values()) position.animation?.cancel(); previous.current.clear(); initialized.current = false; }, []);
}
