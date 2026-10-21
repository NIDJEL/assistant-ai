import { useLayoutEffect, useRef, type KeyboardEvent } from 'react';
import { motionUpdate, reducedMotion } from './motion/motion';

export interface SegmentsProps<T extends string> {
  value: T;
  onChange: (value: T) => void;
  options: { value: T; label: string; count?: number }[];
  label: string;
}

export function SlidingSegments<T extends string>({ value, onChange, options, label }: SegmentsProps<T>) {
  const group = useRef<HTMLDivElement>(null);
  const indicator = useRef<HTMLSpanElement>(null);

  useLayoutEffect(() => {
    const root = group.current;
    const pill = indicator.current;
    if (!root || !pill) return;
    const update = () => {
      const selected = root.querySelector<HTMLButtonElement>('button[aria-pressed="true"]');
      if (!selected) { delete root.dataset.ready; return; }
      const bounds = selected.getBoundingClientRect();
      const track = root.getBoundingClientRect();
      const left = bounds.left - track.left + root.scrollLeft - root.clientLeft;
      const top = bounds.top - track.top + root.scrollTop - root.clientTop;
      pill.style.width = `${bounds.width}px`;
      pill.style.height = `${bounds.height}px`;
      pill.style.transform = `translate3d(${left}px, ${top}px, 0)`;
      root.dataset.ready = 'true';
      if (root.scrollWidth > root.clientWidth && typeof root.scrollTo === 'function') {
        const right = left + bounds.width;
        const scroll = left < root.scrollLeft ? left - 3 : right > root.scrollLeft + root.clientWidth ? right - root.clientWidth + 3 : root.scrollLeft;
        if (scroll !== root.scrollLeft) root.scrollTo({ left: scroll, behavior: reducedMotion() ? 'instant' : 'smooth' });
      }
    };
    update();
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(update);
    observer?.observe(root);
    root.querySelectorAll('button').forEach(button => observer?.observe(button));
    window.addEventListener('resize', update);
    return () => { observer?.disconnect(); window.removeEventListener('resize', update); };
  }, [value, options]);

  const select = (next: T) => { if (next !== value) motionUpdate(() => onChange(next)); };
  function navigate(event: KeyboardEvent<HTMLDivElement>) {
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
    const buttons = Array.from(group.current?.querySelectorAll('button') || []);
    const current = buttons.indexOf(event.target as HTMLButtonElement);
    if (current < 0) return;
    event.preventDefault();
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? options.length - 1 : (current + (event.key === 'ArrowRight' ? 1 : -1) + options.length) % options.length;
    buttons[next]?.focus({ preventScroll: true });
    select(options[next].value);
  }

  return <div ref={group} className="segments segments--sliding" role="group" aria-label={label} onKeyDown={navigate}>
    <span ref={indicator} className="segment-indicator" aria-hidden="true"/>
    {options.map(option => <button type="button" key={option.value} aria-pressed={value === option.value} onClick={() => select(option.value)}>
      <span className="segment-label">{option.label}</span>{option.count !== undefined && <span className="count">{option.count}</span>}
    </button>)}
  </div>;
}
