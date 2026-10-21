import { useEffect, useId, useLayoutEffect, useRef, useState, type ButtonHTMLAttributes, type KeyboardEvent } from 'react';
import { createPortal } from 'react-dom';
import { Check, ChevronDown } from 'lucide-react';
import { reducedMotion } from './motion/motion';

export interface SelectOption { value: string | number; label: string; disabled?: boolean }
type Props = Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'value' | 'onChange' | 'children' | 'name'> & {
  value: string | number; onChange: (value: string) => void; options: SelectOption[];
  name?: string; required?: boolean; placeholder?: string;
};

export function Select({ value, onChange, options, name, required, placeholder = 'Выберите', disabled, id, className = '', onKeyDown, ...props }: Props) {
  const generated = useId(); const controlId = id || `select-${generated}`; const listId = `${controlId}-list`;
  const trigger = useRef<HTMLButtonElement>(null); const panel = useRef<HTMLDivElement>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined); const search = useRef({ text: '', time: 0 });
  const [phase, setPhase] = useState<'closed' | 'open' | 'closing'>('closed'); const visible = phase !== 'closed';
  const selected = options.findIndex(option => String(option.value) === String(value));
  const [active, setActive] = useState(selected); const [host, setHost] = useState<HTMLElement | null>(null);
  const optionId = (index: number) => `${listId}-${index}`;
  function close() {
    clearTimeout(timer.current); setPhase('closing');
    timer.current = setTimeout(() => setPhase('closed'), reducedMotion() ? 0 : 160);
  }
  function open(index = selected) {
    if (disabled || !options.some(option => !option.disabled)) return;
    clearTimeout(timer.current); search.current = { text: '', time: 0 };
    setHost(trigger.current?.closest('dialog') || document.body);
    setActive(index >= 0 && !options[index]?.disabled ? index : options.findIndex(option => !option.disabled)); setPhase('open');
  }
  function choose(index: number) {
    const option = options[index]; if (!option || option.disabled || phase !== 'open') return;
    if (String(option.value) !== String(value)) onChange(String(option.value));
    close(); trigger.current?.focus({ preventScroll: true });
  }
  useEffect(() => () => clearTimeout(timer.current), []);
  useEffect(() => { if (disabled && visible) setPhase('closed'); }, [disabled, visible]);
  useLayoutEffect(() => {
    if (!visible || !host) return;
    const popup = panel.current!; const button = trigger.current!;
    if (typeof popup.showPopover === 'function') popup.showPopover();
    else { popup.dataset.fallback = 'true'; popup.style.display = 'block'; }
    function position() {
      const viewport = window.visualViewport; const left = viewport?.offsetLeft || 0; const top = viewport?.offsetTop || 0;
      const width = viewport?.width || innerWidth; const height = viewport?.height || innerHeight;
      const box = button.getBoundingClientRect();
      popup.style.width = `${Math.min(Math.max(box.width, 220), 360, width - 24)}px`;
      const below = top + height - box.bottom - 16; const above = box.top - top - 16;
      const upwards = below < Math.min(popup.scrollHeight, 280) && above > below;
      popup.style.maxHeight = `${Math.max(64, Math.min(320, upwards ? above : below, height - 24))}px`;
      popup.dataset.placement = upwards ? 'above' : 'below';
      popup.style.left = `${Math.max(left + 12, Math.min(box.left, left + width - popup.offsetWidth - 12))}px`;
      popup.style.top = `${Math.max(top + 12, Math.min(upwards ? box.top - popup.offsetHeight - 6 : box.bottom + 6, top + height - popup.offsetHeight - 12))}px`;
    }
    const outside = (event: PointerEvent) => { if (!popup.contains(event.target as Node) && !button.contains(event.target as Node)) close(); };
    position(); const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(position);
    observer?.observe(popup); observer?.observe(button);
    document.addEventListener('pointerdown', outside, true);
    window.addEventListener('resize', position); window.addEventListener('scroll', position, true);
    window.visualViewport?.addEventListener('resize', position); window.visualViewport?.addEventListener('scroll', position);
    return () => {
      observer?.disconnect(); document.removeEventListener('pointerdown', outside, true);
      window.removeEventListener('resize', position); window.removeEventListener('scroll', position, true);
      window.visualViewport?.removeEventListener('resize', position); window.visualViewport?.removeEventListener('scroll', position);
      if (typeof popup.hidePopover === 'function' && popup.matches(':popover-open')) popup.hidePopover();
    };
  }, [visible, host]);
  useLayoutEffect(() => {
    if (phase === 'open') panel.current?.querySelector<HTMLElement>(`[id="${optionId(active)}"]`)?.scrollIntoView({ block: 'nearest' });
  }, [active, phase]);
  function keyboard(event: KeyboardEvent<HTMLButtonElement>) {
    onKeyDown?.(event); if (event.defaultPrevented) return;
    if (event.key === 'Tab') { if (phase === 'open') close(); return; }
    if (event.key === 'Escape' && phase === 'open') { event.preventDefault(); event.stopPropagation(); close(); return; }
    if (['Enter', ' '].includes(event.key)) { event.preventDefault(); phase === 'open' ? choose(active) : open(); return; }
    if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
      event.preventDefault();
      const enabled = options.map((option, index) => option.disabled ? -1 : index).filter(index => index >= 0); if (!enabled.length) return;
      const current = enabled.indexOf(active); const direction = event.key === 'ArrowUp' ? -1 : 1;
      const next = event.key === 'Home' ? enabled[0] : event.key === 'End' ? enabled.at(-1)! : enabled[(current + direction + enabled.length) % enabled.length];
      if (phase !== 'open') open(event.key === 'Home' || event.key === 'End' ? next : selected); else setActive(next);
      return;
    }
    if (event.key.length === 1 && !event.altKey && !event.ctrlKey && !event.metaKey) {
      event.preventDefault(); const letter = event.key.toLocaleLowerCase('ru'); const now = Date.now();
      const text = now - search.current.time < 700 ? search.current.text + letter : letter;
      const prefix = [...text].every(char => char === letter) ? letter : text; search.current = { text, time: now };
      const start = prefix.length === 1 ? active + 1 : active;
      const match = options.map((_, index) => (Math.max(0, start) + index) % options.length).find(index => !options[index].disabled && options[index].label.toLocaleLowerCase('ru').startsWith(prefix));
      if (match !== undefined) { if (phase !== 'open') open(match); else setActive(match); search.current = { text, time: now }; }
    }
  }
  return <span className={`gena-select ${className}`} data-disabled={disabled || undefined}>
    {name && <input type="hidden" name={name} value={value} disabled={disabled}/>}
    {required && <input className="select-validation" aria-hidden="true" tabIndex={-1} required disabled={disabled} value={value} onChange={() => {}} onInvalid={event => { event.preventDefault(); trigger.current?.focus(); }}/>}
    <button {...props} ref={trigger} id={controlId} type="button" className="select-trigger" role="combobox" aria-haspopup="listbox" aria-expanded={phase === 'open'} aria-controls={visible ? listId : undefined} aria-activedescendant={phase === 'open' && active >= 0 ? optionId(active) : undefined} aria-required={required || undefined} disabled={disabled} onKeyDown={keyboard} onClick={() => phase === 'open' ? close() : open()}>
      <span className="select-value">{options[selected]?.label || placeholder}</span><ChevronDown size={16} aria-hidden="true"/>
    </button>
    {visible && host && createPortal(<div ref={panel} popover="manual" id={listId} className="gena-select-menu" role="listbox" aria-labelledby={controlId} data-closing={phase === 'closing' || undefined} onPointerDown={event => event.preventDefault()} onClick={event => event.stopPropagation()}>
      {options.map((option, index) => <div id={optionId(index)} key={option.value} role="option" aria-selected={index === selected} aria-disabled={option.disabled || undefined} data-active={index === active || undefined} className="select-option" onPointerMove={() => { if (!option.disabled && phase === 'open') setActive(index); }} onClick={() => choose(index)}><span>{option.label}</span>{index === selected && <Check size={16} aria-hidden="true"/>}</div>)}
    </div>, host)}
  </span>;
}
