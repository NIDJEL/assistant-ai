import { useEffect, useRef } from 'react';
import { reducedMotion } from '../../components/motion/motion';

/** A damped pointer spring. It only runs while moving and never renders React. */
export function usePointerSpring<T extends HTMLElement>() {
  const ref = useRef<T>(null);
  useEffect(() => {
    const node = ref.current;
    if (!node || typeof window.matchMedia !== 'function') return;
    const pointer = window.matchMedia('(hover: hover) and (pointer: fine)');
    const preference = window.matchMedia('(prefers-reduced-motion: reduce)');
    let frame = 0;
    let previous = 0;
    let x = 0; let y = 0; let vx = 0; let vy = 0; let tx = 0; let ty = 0;
    const write = () => {
      node.style.setProperty('--pointer-x', x.toFixed(4));
      node.style.setProperty('--pointer-y', y.toFixed(4));
    };
    const tick = (time: number) => {
      const dt = Math.min((time - previous) / 1000 || 1 / 60, 1 / 30);
      previous = time;
      vx += ((tx - x) * 110 - vx * 21) * dt;
      vy += ((ty - y) * 110 - vy * 21) * dt;
      x += vx * dt; y += vy * dt;
      if (Math.abs(tx - x) + Math.abs(ty - y) + Math.abs(vx) + Math.abs(vy) < .002) {
        x = tx; y = ty; write(); frame = 0; previous = 0; return;
      }
      write(); frame = requestAnimationFrame(tick);
    };
    const start = () => { if (!frame) frame = requestAnimationFrame(tick); };
    const reset = () => { tx = 0; ty = 0; start(); };
    const stop = () => {
      cancelAnimationFrame(frame); frame = 0; previous = 0;
      x = y = vx = vy = tx = ty = 0; write();
    };
    const move = (event: PointerEvent) => {
      if (event.pointerType === 'touch' || !pointer.matches || reducedMotion() || document.hidden) return;
      const bounds = node.getBoundingClientRect();
      tx = Math.max(-1, Math.min(1, ((event.clientX - bounds.left) / bounds.width - .5) * 2));
      ty = Math.max(-1, Math.min(1, ((event.clientY - bounds.top) / bounds.height - .5) * 2));
      start();
    };
    const visibility = () => {
      node.dataset.paused = String(document.hidden);
      if (document.hidden) stop();
    };
    visibility();
    node.addEventListener('pointermove', move, { passive: true });
    node.addEventListener('pointerleave', reset);
    document.addEventListener('visibilitychange', visibility);
    pointer.addEventListener('change', stop);
    preference.addEventListener('change', stop);
    const observer = new MutationObserver(stop);
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['data-motion'] });
    return () => {
      stop(); observer.disconnect();
      node.removeEventListener('pointermove', move);
      node.removeEventListener('pointerleave', reset);
      document.removeEventListener('visibilitychange', visibility);
      pointer.removeEventListener('change', stop);
      preference.removeEventListener('change', stop);
    };
  }, []);
  return ref;
}
