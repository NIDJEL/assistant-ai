import { useEffect, useId, useLayoutEffect, useRef, useState, type InputHTMLAttributes, type KeyboardEvent } from 'react';
import { createPortal } from 'react-dom';
import { CalendarDays, ChevronLeft, ChevronRight, X } from 'lucide-react';
import { addDays, addMonths, civilDate, dayKey, formatDay, isCivilDay, monthDays, monthStart, weekStart } from '../features/calendar/dates';

type Props = Omit<InputHTMLAttributes<HTMLInputElement>, 'type' | 'value' | 'onChange' | 'min' | 'max'> & {
  value: string; onChange: (date: string) => void; min?: string; max?: string; timezone?: string;
};
const months = Array.from({ length: 12 }, (_, month) => new Intl.DateTimeFormat('ru-RU', { month: 'long', timeZone: 'UTC' }).format(new Date(Date.UTC(2024, month, 1))));
export const displayDate = (value: string) => isCivilDay(value) ? value.split('-').reverse().join('.') : value;
export function parseDate(value: string) {
  const text = value.trim();
  const match = /^(\d{2})\.(\d{2})\.(\d{4})$/.exec(text) || /^(\d{2})(\d{2})(\d{4})$/.exec(text);
  const iso = match ? `${match[3]}-${match[2]}-${match[1]}` : text;
  return isCivilDay(iso) ? iso : '';
}
export function DatePicker({ value, onChange, min, max, timezone = Intl.DateTimeFormat().resolvedOptions().timeZone, disabled, required, name, id, onBlur, onFocus, onKeyDown, ...props }: Props) {
  const generated = useId(); const inputId = id || `date-${generated}`; const popupId = `${inputId}-calendar`;
  const input = useRef<HTMLInputElement>(null); const anchor = useRef<HTMLSpanElement>(null); const panel = useRef<HTMLDialogElement>(null);
  const [draft, setDraft] = useState(displayDate(value)); const [touched, setTouched] = useState(false);
  const [phase, setPhase] = useState<'closed' | 'open' | 'closing'>('closed'); const visible = phase !== 'closed';
  const [view, setView] = useState<'days' | 'months' | 'years'>('days');
  const today = dayKey(new Date().toISOString(), timezone);
  const minimum = isCivilDay(min || null) ? min! : '0001-01-01'; const maximum = isCivilDay(max || null) ? max! : '9999-12-31';
  const bounded = (day: string) => day < minimum ? minimum : day > maximum ? maximum : day;
  const allowed = (day: string) => day >= minimum && day <= maximum;
  const [cursor, setCursor] = useState(bounded(isCivilDay(value) ? value : today)); const [month, setMonth] = useState(monthStart(cursor));
  const focusDay = useRef(false); const closeTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const year = Number(month.slice(0, 4)); const monthIndex = Number(month.slice(5, 7)) - 1;
  const yearBase = Math.max(1, Math.min(9988, Math.floor(year / 12) * 12));
  const parsed = parseDate(draft);
  const invalid = !!draft && (!parsed || !allowed(parsed));
  useEffect(() => { setDraft(displayDate(value)); setTouched(false); }, [value]);
  useLayoutEffect(() => { input.current?.setCustomValidity(invalid ? 'Введите допустимую дату в формате ДД.ММ.ГГГГ.' : ''); }, [invalid]);
  useEffect(() => () => clearTimeout(closeTimer.current), []);
  function open() {
    if (disabled || phase !== 'closed') return;
    const next = bounded(isCivilDay(value) ? value : today);
    setCursor(next); setMonth(monthStart(next)); setView('days'); focusDay.current = true; setPhase('open');
  }
  function close() {
    if (phase !== 'open') return;
    setPhase('closing');
    const reduced = document.documentElement.dataset.motion === 'reduced' || document.documentElement.dataset.motion !== 'full' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
    closeTimer.current = setTimeout(() => setPhase('closed'), reduced ? 0 : 180);
  }
  function choose(day: string) { if (!allowed(day) || phase !== 'open') return; onChange(day); setDraft(displayDate(day)); setTouched(false); close(); }
  useLayoutEffect(() => {
    if (!visible) return;
    const popup = panel.current!;
    popup.showModal();
    function position() {
      const viewport = window.visualViewport; const leftEdge = viewport?.offsetLeft || 0; const topEdge = viewport?.offsetTop || 0;
      const width = viewport?.width || innerWidth; const height = viewport?.height || innerHeight;
      const box = anchor.current!.getBoundingClientRect();
      popup.style.maxHeight = `${Math.max(180, height - 24)}px`;
      const popupWidth = popup.offsetWidth; const popupHeight = popup.offsetHeight;
      popup.style.left = `${Math.max(leftEdge + 12, Math.min(box.left, leftEdge + width - popupWidth - 12))}px`;
      const below = box.bottom + 8;
      popup.style.top = `${Math.max(topEdge + 12, Math.min(below + popupHeight <= topEdge + height - 12 ? below : box.top - popupHeight - 8, topEdge + height - popupHeight - 12))}px`;
    }
    position();
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(position); observer?.observe(popup);
    window.addEventListener('resize', position); window.addEventListener('scroll', position, true); window.visualViewport?.addEventListener('resize', position);
    return () => { observer?.disconnect(); window.removeEventListener('resize', position); window.removeEventListener('scroll', position, true); window.visualViewport?.removeEventListener('resize', position); popup.close(); input.current?.focus({ preventScroll: true }); };
  }, [visible]);
  useLayoutEffect(() => {
    if (visible && view === 'days' && focusDay.current) { panel.current?.querySelector<HTMLButtonElement>(`[data-date="${cursor}"]`)?.focus({ preventScroll: true }); focusDay.current = false; }
  }, [visible, view, cursor, month]);
  function canNavigate(amount: number) {
    const next = addMonths(month, amount * (view === 'days' ? 1 : view === 'months' ? 12 : 144));
    if (next === month) return false;
    if (view === 'days') return `${next.slice(0, 7)}-31` >= minimum && next <= maximum;
    const nextYear = Number(next.slice(0, 4));
    const firstYear = view === 'years' ? Math.max(1, Math.min(9988, Math.floor(nextYear / 12) * 12)) : nextYear;
    const lastYear = view === 'years' ? firstYear + 11 : firstYear;
    return `${String(lastYear).padStart(4, '0')}-12-31` >= minimum && `${String(firstYear).padStart(4, '0')}-01-01` <= maximum;
  }
  function navigate(amount: number) {
    if (!canNavigate(amount)) return;
    const next = addMonths(month, amount * (view === 'days' ? 1 : view === 'months' ? 12 : 144));
    if (next !== month) {
      if (view === 'days') {
        const last = civilDate(next); last.setUTCMonth(last.getUTCMonth() + 1); last.setUTCDate(0);
        setCursor(bounded(`${next.slice(0, 8)}${String(Math.min(Number(cursor.slice(-2)), last.getUTCDate())).padStart(2, '0')}`));
      }
      setMonth(next);
    }
  }
  function selectMonth(index: number, selectedYear = year) {
    const first = `${String(selectedYear).padStart(4, '0')}-${String(index + 1).padStart(2, '0')}-01`;
    const last = new Date(`${first}T12:00:00Z`); last.setUTCMonth(last.getUTCMonth() + 1); last.setUTCDate(0);
    const day = String(Math.min(Number(cursor.slice(-2)), last.getUTCDate())).padStart(2, '0');
    const next = bounded(`${first.slice(0, 8)}${day}`);
    setMonth(monthStart(next)); setCursor(next); setView('days'); focusDay.current = true;
  }
  function keyboard(event: KeyboardEvent<HTMLDialogElement>) {
    event.stopPropagation();
    if (event.key === 'Escape') { event.preventDefault(); close(); return; }
    if (!(event.target instanceof HTMLElement) || !event.target.hasAttribute('data-date')) return;
    let next = cursor;
    if (event.key === 'ArrowLeft') next = addDays(cursor, -1);
    else if (event.key === 'ArrowRight') next = addDays(cursor, 1);
    else if (event.key === 'ArrowUp') next = addDays(cursor, -7);
    else if (event.key === 'ArrowDown') next = addDays(cursor, 7);
    else if (event.key === 'Home') next = weekStart(cursor);
    else if (event.key === 'End') next = addDays(weekStart(cursor), 6);
    else if (event.key === 'PageUp' || event.key === 'PageDown') {
      const nextMonth = addMonths(cursor, (event.key === 'PageUp' ? -1 : 1) * (event.shiftKey ? 12 : 1));
      const last = civilDate(nextMonth); last.setUTCMonth(last.getUTCMonth() + 1); last.setUTCDate(0);
      next = `${nextMonth.slice(0, 8)}${String(Math.min(Number(cursor.slice(-2)), last.getUTCDate())).padStart(2, '0')}`;
    } else return;
    event.preventDefault(); next = bounded(next); focusDay.current = true; setCursor(next); setMonth(monthStart(next));
  }
  const days = monthDays(month); const rows = Array.from({ length: Math.ceil(days.length / 7) }, (_, index) => days.slice(index * 7, index * 7 + 7));
  return <span className={`date-field ${touched && invalid ? 'invalid' : ''}`} ref={anchor} data-disabled={disabled || undefined}>
    <input {...props} id={inputId} ref={input} type="text" inputMode="numeric" value={draft} placeholder="ДД.ММ.ГГГГ" autoComplete="off" disabled={disabled} required={required} data-iso-date={value} aria-invalid={props['aria-invalid'] || touched && invalid || undefined} aria-haspopup="dialog" aria-expanded={visible} aria-controls={popupId} onFocus={onFocus} onBlur={event => { setTouched(true); if (parsed && allowed(parsed)) setDraft(displayDate(parsed)); onBlur?.(event); }} onChange={event => {
      const text = event.target.value; setDraft(text); const iso = parseDate(text);
      if (!text) onChange(''); else if (iso && allowed(iso)) onChange(iso);
    }} onKeyDown={event => { if (event.key === 'F4' || event.altKey && event.key === 'ArrowDown') { event.preventDefault(); open(); } onKeyDown?.(event); }}/>
    {name && <input type="hidden" name={name} value={value} disabled={disabled}/>}
    <button type="button" className="date-field-trigger" aria-label="Выбрать дату" aria-haspopup="dialog" aria-expanded={visible} aria-controls={popupId} disabled={disabled} onClick={event => { event.preventDefault(); open(); }}><CalendarDays size={19} aria-hidden="true"/></button>
    {visible && createPortal(<dialog ref={panel} id={popupId} className="date-picker-popover" aria-label="Выбор даты" data-closing={phase === 'closing' || undefined} onKeyDown={keyboard} onCancel={event => { event.preventDefault(); event.stopPropagation(); close(); }} onClick={event => {
      event.stopPropagation(); const box = event.currentTarget.getBoundingClientRect();
      if (event.target === event.currentTarget && (event.clientX < box.left || event.clientX > box.right || event.clientY < box.top || event.clientY > box.bottom)) close();
    }}>
      <header className="date-picker-header"><button type="button" aria-label="Предыдущий период выбора даты" onClick={() => navigate(-1)} disabled={phase !== 'open' || !canNavigate(-1)}><ChevronLeft size={18}/></button><span><button type="button" onClick={() => setView(view === 'months' ? 'days' : 'months')} aria-label="Выбрать месяц">{months[monthIndex]}</button><button type="button" onClick={() => setView(view === 'years' ? 'days' : 'years')} aria-label="Выбрать год">{year}</button></span><button type="button" aria-label="Следующий период выбора даты" onClick={() => navigate(1)} disabled={phase !== 'open' || !canNavigate(1)}><ChevronRight size={18}/></button><button type="button" aria-label="Закрыть выбор даты" onClick={close}><X size={17}/></button></header>
      <div className="date-picker-view" key={`${view}-${month}`}>
        {view === 'days' ? <div role="grid" aria-label={formatDay(month, { month: 'long', year: 'numeric' })}><div role="row" className="date-picker-weekdays">{['Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб', 'Вс'].map(day => <span role="columnheader" key={day}>{day}</span>)}</div>{rows.map((row, index) => <div role="row" className="date-picker-week" key={index}>{row.map(day => <button type="button" role="gridcell" data-date={day} tabIndex={day === cursor ? 0 : -1} aria-label={formatDay(day, { day: 'numeric', month: 'long', year: 'numeric' })} aria-selected={day === value} aria-current={day === today ? 'date' : undefined} className={day.slice(0, 7) !== month.slice(0, 7) ? 'outside' : ''} disabled={phase !== 'open' || !allowed(day)} key={day} onClick={() => choose(day)}>{Number(day.slice(-2))}</button>)}</div>)}</div>
          : view === 'months' ? <div className="date-picker-options" aria-label="Месяцы">{months.map((label, index) => <button type="button" key={label} aria-pressed={index === monthIndex} disabled={`${String(year).padStart(4, '0')}-${String(index + 1).padStart(2, '0')}-31` < minimum || `${String(year).padStart(4, '0')}-${String(index + 1).padStart(2, '0')}-01` > maximum} onClick={() => selectMonth(index)}>{label}</button>)}</div>
          : <div className="date-picker-options" aria-label="Годы">{Array.from({ length: 12 }, (_, index) => yearBase + index).map(nextYear => <button type="button" key={nextYear} aria-pressed={nextYear === year} disabled={`${String(nextYear).padStart(4, '0')}-12-31` < minimum || `${String(nextYear).padStart(4, '0')}-01-01` > maximum} onClick={() => selectMonth(monthIndex, nextYear)}>{nextYear}</button>)}</div>}
      </div><footer className="date-picker-footer"><button type="button" disabled={!allowed(today) || phase !== 'open'} onClick={() => choose(today)}>Сегодня</button>{!required && value && <button type="button" onClick={() => { onChange(''); setDraft(''); close(); }}>Очистить</button>}</footer>
    </dialog>, document.body)}
  </span>;
}
