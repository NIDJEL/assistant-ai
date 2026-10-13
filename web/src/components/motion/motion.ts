import { flushSync } from 'react-dom';

export const exitDuration = 220;
export const motionEase = 'cubic-bezier(.22, 1, .36, 1)';
export function reducedMotion() {
  const preference = document.documentElement.dataset.motion;
  return preference === 'full' ? false : preference === 'reduced' || (window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false);
}

let active: ViewTransition | undefined;
let fallback: Animation | undefined;
export function viewTransitionActive() { return !!active; }
function fadeFallback(scope: 'content' | 'navigation' | 'layout', target: string) {
  const page = document.querySelector<HTMLElement>('.main-panel .page');
  if (!page?.animate || reducedMotion()) return;
  if (scope === 'content') {
    for (const node of document.querySelectorAll<HTMLElement>(target)) {
      if (!node.parentElement?.closest('.motion-content')) node.animate([{ opacity: .75 }, { opacity: 1 }], { duration: 240, easing: motionEase });
    }
    return;
  }
  fallback?.cancel();
  const animation = page.animate([{ opacity: .15 }, { opacity: 1 }], { duration: 440, easing: motionEase });
  fallback = animation;
  void animation.finished.catch(() => {}).finally(() => { if (fallback === animation) fallback = undefined; });
}
// Keep live React state and focus in place while the browser animates snapshots.
export function motionUpdate(update: () => void, scope: 'content' | 'navigation' | 'layout' = 'content', target = '.main-panel .motion-content') {
  if (reducedMotion() || document.querySelector('.widget-drag-overlay')) {
    update(); return;
  }
  if (scope === 'content') { flushSync(update); fadeFallback(scope, target); return; }
  // Mobile navigation closes its drawer first; animate the live page beneath it.
  if (document.querySelector('dialog[open]')) { flushSync(update); fadeFallback(scope, target); return; }
  if (typeof document.startViewTransition !== 'function') { flushSync(update); fadeFallback(scope, target); return; }
  active?.skipTransition();
  let applied = false;
  const apply = () => { if (!applied) { applied = true; flushSync(update); } };
  try {
    // Rendering pauses during this callback, so it must not wait on animation frames.
    const transition = document.startViewTransition(apply);
    active = transition;
    void transition.ready.catch(() => { if (active === transition) fadeFallback(scope, target); });
    void transition.finished.catch(() => {}).finally(() => { if (active === transition) active = undefined; });
  } catch { apply(); fadeFallback(scope, target); }
}
