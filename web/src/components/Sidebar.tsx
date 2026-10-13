import { MotionNavLink as NavLink } from './motion/MotionNavLink';
import { Settings } from 'lucide-react';
import { navigation } from '../app/navigation';
import { usePreferences } from '../app/PreferencesContext';
import { BrandMark } from './Brand';
export function Sidebar({ onNavigate }: { onNavigate?: () => void }) {
  const { preferences } = usePreferences();
  return <div className="sidebar-inner"><NavLink className="brand" to="/" aria-label="Ритм — Сегодня" onClick={onNavigate}><BrandMark/><span>Ритм</span></NavLink>
    <nav aria-label="Основная навигация">{navigation.filter(item => item.path !== '/settings').map(({ path, label, icon: Icon }) => <NavLink key={path} to={path} end={path === '/'} className="nav-link" aria-label={label} onClick={onNavigate}><Icon size={24} aria-hidden="true"/><span>{label}</span></NavLink>)}</nav>
    <div className="sidebar-footer"><span className="avatar" aria-hidden="true">{preferences.name.slice(0, 1).toUpperCase()}</span><span className="profile-name">{preferences.name}</span><NavLink to="/settings" className="settings-link" aria-label="Настройки" onClick={onNavigate}><Settings size={24}/></NavLink></div>
  </div>;
}
