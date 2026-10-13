import { flushSync } from 'react-dom';

export const exitDuration = 220;
export const motionEase = 'cubic-bezier(.22, 1, .36, 1)';
export function reducedMotion() { return window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false; }

let active: ViewTransition | undefined;
// Keep live React state and focus in place while the browser animates snapshots.
export function motionUpdate(update: () => void) {
  if (!document.startViewTransition || reducedMotion() || document.querySelector('dialog[open], .widget-drag-overlay')) {
    update(); return;
  }
  active?.skipTransition();
  let applied = false;
  const apply = () => { if (!applied) { applied = true; flushSync(update); } };
  try {
    // Rendering pauses during this callback, so it must not wait on animation frames.
    const transition = document.startViewTransition(apply);
    active = transition;
    void transition.ready.catch(() => {});
    void transition.finished.catch(() => {}).finally(() => { if (active === transition) active = undefined; });
  } catch { apply(); }
}
