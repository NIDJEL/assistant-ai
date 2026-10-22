import { useId } from 'react';
import type { Theme } from '../app/theme';
import '../styles/theme-picker.css';

export function ThemePicker({ theme, onChange, disabled = false }: { theme: Theme; onChange: (theme: Theme) => void; disabled?: boolean }) {
  const name = useId();
  return <fieldset className="theme-picker" disabled={disabled}>
    <legend>Тема оформления</legend>
    <div className="theme-options">{(['dark', 'light'] as const).map(value => <label className={`theme-option theme-option-${value}`} key={value}>
      <input type="radio" name={name} value={value} checked={theme === value} onChange={() => onChange(value)}/>
      <span className="theme-swatch" aria-hidden="true"><span className="theme-swatch-sidebar"/><span className="theme-swatch-content"><i/><i/><i/></span></span>
      <span className="theme-option-label">{value === 'dark' ? 'Тёмная' : 'Светлая'}</span><span className="theme-option-check" aria-hidden="true"><svg width="13" height="13" viewBox="0 0 16 16" fill="none"><path d="m3.5 8 3 3 6-6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/></svg></span>
    </label>)}</div>
  </fieldset>;
}
