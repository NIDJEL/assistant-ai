import type { ButtonHTMLAttributes, ReactNode } from 'react';
import { Search, X, Inbox, Plus, type LucideIcon } from 'lucide-react';
import { Mascot } from './Brand';
import { motionUpdate } from './motion/motion';
export function Button({ children, variant = 'primary', className = '', ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: 'primary' | 'soft' | 'ghost' | 'danger' }) {
  return <button type="button" className={`button ${variant} ${className}`} {...props}>{children}</button>;
}
export function IconButton({ icon: Icon, label, className = '', ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { icon: LucideIcon; label: string }) {
  return <button type="button" className={`icon-button ${className}`} aria-label={label} title={label} {...props}><Icon size={20}/></button>;
}
export function PageHeader({ title, subtitle, mascot = true }: { title: string; subtitle: string; mascot?: boolean }) {
  return <header className="page-header"><div><h1>{title}</h1><p>{subtitle}</p></div>{mascot && <Mascot/>}</header>;
}
export function SearchBox({ value, onChange, label, placeholder = label }: { value: string; onChange: (value: string) => void; label: string; placeholder?: string }) {
  return <div className="search-box"><Search size={20} aria-hidden="true"/><input aria-label={label} placeholder={placeholder} value={value} maxLength={200} onChange={e => onChange(e.target.value)}/>{value && <IconButton icon={X} label="Очистить поиск" onClick={() => onChange('')}/>}</div>;
}
export function Segments<T extends string>({ value, onChange, options, label }: { value: T; onChange: (value: T) => void; options: { value: T; label: string; count?: number }[]; label: string }) {
  return <div className="segments" role="group" aria-label={label}>{options.map(option => <button type="button" key={option.value} aria-pressed={value === option.value} onClick={() => { if (value !== option.value) motionUpdate(() => onChange(option.value)); }}>{option.label}{option.count !== undefined && <span className="count">{option.count}</span>}</button>)}</div>;
}
export function Empty({ children = 'Пока ничего нет', action }: { children?: ReactNode; action?: ReactNode }) {
  return <div className="empty-state"><Inbox size={30} aria-hidden="true"/><p>{children}</p>{action}</div>;
}
export function Alert({ children, action }: { children: ReactNode; action?: ReactNode }) { return <div className="alert" role="alert"><span>{children}</span>{action}</div>; }
export function NewButton({ children, onClick }: { children: ReactNode; onClick: () => void }) { return <Button onClick={onClick}><Plus size={19} aria-hidden="true"/>{children}</Button>; }
