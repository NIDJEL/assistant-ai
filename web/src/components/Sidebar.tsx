import { MotionNavLink as NavLink } from './motion/MotionNavLink';
import { useEffect, useRef, useState } from 'react';
import { Settings, LogOut, PanelLeftClose, PanelLeftOpen } from 'lucide-react';
import { useLocation } from 'react-router-dom';
import { navigation } from '../app/navigation';
import { useAuth } from '../app/AuthContext';
import { usePreferences } from '../app/PreferencesContext';
import { BrandMark } from './Brand';
import { IconButton } from './UI';
import { motionUpdate } from './motion/motion';
import { Avatar } from './Avatar';
import { errorMessage } from '../api/client';
export function Sidebar({ onNavigate, collapsible = false }: { onNavigate?: () => void; collapsible?: boolean }) {
  const { user, logout } = useAuth();
  const { preferences, update } = usePreferences();
  const location = useLocation();
  const [profileOpen, setProfileOpen] = useState(false); const [busy, setBusy] = useState(false); const [error, setError] = useState('');
  const profile = useRef<HTMLDivElement>(null); const trigger = useRef<HTMLButtonElement>(null);
  useEffect(() => { setProfileOpen(false); }, [location.pathname]);
  useEffect(() => {
    if (!profileOpen) return;
    profile.current?.querySelector<HTMLElement>('[role="menuitem"]')?.focus();
    const outside = (event: PointerEvent) => { if (!profile.current?.contains(event.target as Node)) setProfileOpen(false); };
    const escape = (event: KeyboardEvent) => { if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); setProfileOpen(false); trigger.current?.focus(); } };
    document.addEventListener('pointerdown', outside); document.addEventListener('keydown', escape, true);
    return () => { document.removeEventListener('pointerdown', outside); document.removeEventListener('keydown', escape, true); };
  }, [profileOpen]);
  async function leave() { if (busy) return; setBusy(true); setError(''); try { await logout(); } catch (e) { setError(errorMessage(e)); setBusy(false); } }
  return <div className="sidebar-inner"><div className="sidebar-brand-row"><NavLink className="brand" to="/" aria-label="Гена — Сегодня" onClick={onNavigate}><BrandMark/><span>Гена</span></NavLink>{collapsible && <IconButton className="sidebar-collapse" icon={preferences.navCollapsed ? PanelLeftOpen : PanelLeftClose} label={preferences.navCollapsed ? 'Развернуть навигацию' : 'Свернуть навигацию'} onClick={() => motionUpdate(() => update({ navCollapsed: !preferences.navCollapsed }), 'layout')}/>}</div>
    <nav aria-label="Основная навигация">{navigation.filter(item => item.path !== '/settings').map(({ path, label, icon: Icon }) => <NavLink key={path} to={path} end={path === '/'} className="nav-link" aria-label={label} title={label} onClick={onNavigate}><Icon size={24} aria-hidden="true"/><span>{label}</span></NavLink>)}</nav>
    <div className="sidebar-footer"><div className="profile-menu-anchor" ref={profile}><button type="button" ref={trigger} className="profile-trigger" aria-label="Меню аккаунта" aria-haspopup="menu" aria-expanded={profileOpen} onClick={() => setProfileOpen(!profileOpen)}><Avatar name={user!.name} url={user!.avatar_url}/></button>{profileOpen && <div className="profile-menu" role="menu" aria-label="Аккаунт" onBlur={event => { if (!event.currentTarget.contains(event.relatedTarget) && event.relatedTarget !== trigger.current) setProfileOpen(false); }} onKeyDown={event => {
      if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
      event.preventDefault(); const items = [...event.currentTarget.querySelectorAll<HTMLElement>('[role="menuitem"]:not(:disabled)')]; const index = items.indexOf(document.activeElement as HTMLElement);
      items[event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 : (index + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length]?.focus();
    }}><NavLink role="menuitem" to="/settings" onClick={() => { setProfileOpen(false); onNavigate?.(); }}><Settings size={18}/>Настройки</NavLink><button type="button" role="menuitem" className="profile-logout" disabled={busy} onClick={() => void leave()}><LogOut size={18}/>{busy ? 'Выходим…' : 'Выйти'}</button>{error && <p role="alert">{error}</p>}</div>}</div><span className="profile-name">{user!.name}</span><NavLink to="/settings" className="settings-link" aria-label="Настройки" onClick={onNavigate}><Settings size={24}/></NavLink></div>
  </div>;
}
