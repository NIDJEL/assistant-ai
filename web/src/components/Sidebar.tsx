import { MotionNavLink as NavLink } from './motion/MotionNavLink';
import { Settings, PanelLeftClose, PanelLeftOpen } from 'lucide-react';
import { navigation } from '../app/navigation';
import { useAuth } from '../app/AuthContext';
import { usePreferences } from '../app/PreferencesContext';
import { BrandMark } from './Brand';
import { IconButton } from './UI';
import { motionUpdate } from './motion/motion';
export function Sidebar({ onNavigate, collapsible = false }: { onNavigate?: () => void; collapsible?: boolean }) {
  const { user } = useAuth();
  const { preferences, update } = usePreferences();
  return <div className="sidebar-inner"><div className="sidebar-brand-row"><NavLink className="brand" to="/" aria-label="Ритм — Сегодня" onClick={onNavigate}><BrandMark/><span>Ритм</span></NavLink>{collapsible && <IconButton className="sidebar-collapse" icon={preferences.navCollapsed ? PanelLeftOpen : PanelLeftClose} label={preferences.navCollapsed ? 'Развернуть навигацию' : 'Свернуть навигацию'} onClick={() => motionUpdate(() => update({ navCollapsed: !preferences.navCollapsed }), 'layout')}/>}</div>
    <nav aria-label="Основная навигация">{navigation.filter(item => item.path !== '/settings').map(({ path, label, icon: Icon }) => <NavLink key={path} to={path} end={path === '/'} className="nav-link" aria-label={label} title={label} onClick={onNavigate}><Icon size={24} aria-hidden="true"/><span>{label}</span></NavLink>)}</nav>
    <div className="sidebar-footer"><span className="avatar" aria-hidden="true">{user!.avatar_url ? <img src={user!.avatar_url} alt="" referrerPolicy="no-referrer"/> : user!.name.split(' ').map(part => part[0]).slice(0, 2).join('').toUpperCase()}</span><span className="profile-name">{user!.name}</span><NavLink to="/settings" className="settings-link" aria-label="Настройки" onClick={onNavigate}><Settings size={24}/></NavLink></div>
  </div>;
}
