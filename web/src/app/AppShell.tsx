import { useState } from 'react';
import { Outlet, useLocation, Link } from 'react-router-dom';
import { Menu, PanelRight, ChevronRight, Settings } from 'lucide-react';
import { Sidebar } from '../components/Sidebar';
import { Dialog } from '../components/Dialog';
import { Alert, Button, IconButton } from '../components/UI';
import { WidgetList, type WidgetKey } from '../features/widgets/Widgets';
import { usePreferences } from './PreferencesContext';
import { useData } from './DataContext';
const layouts: Record<string, WidgetKey[]> = { '/tasks': ['progress', 'tasks', 'meetings'], '/meetings': ['calendar', 'today', 'tasks'], '/settings': ['progress', 'meetings'], '/archive': ['progress', 'calendar'] };
export function AppShell() {
  const { preferences, update } = usePreferences(); const { health, error, refresh } = useData(); const location = useLocation();
  const [menu, setMenu] = useState(false); const [widgetsDrawer, setWidgetsDrawer] = useState(false);
  const keys = layouts[location.pathname] || ['calendar', 'today', 'tasks'];
  const widgetBody = <><div className="widgets-heading"><div><h2>Виджеты</h2><p>Настройте панель под себя</p></div><Link to="/settings" className="icon-button" aria-label="Настройки виджетов" onClick={() => setWidgetsDrawer(false)}><Settings size={21}/></Link></div>{health && <WidgetList keys={keys}/>}</>;
  return <div className={`app-shell ${preferences.widgets ? '' : 'widgets-hidden'}`}>
    <a className="skip-link" href="#main-content">К содержимому</a><aside className="sidebar"><Sidebar/></aside>
    <main className="main-panel" id="main-content"><div className="shell-controls"><IconButton icon={Menu} label="Открыть навигацию" className="mobile-menu" onClick={() => setMenu(true)}/><button type="button" className="drawer-widget-button" aria-label="Открыть виджеты" onClick={() => setWidgetsDrawer(true)}><PanelRight size={19}/><span>Виджеты</span></button><IconButton icon={preferences.widgets ? ChevronRight : PanelRight} label={preferences.widgets ? 'Скрыть виджеты' : 'Показать виджеты'} className="desktop-widget-button" onClick={() => update({ widgets: !preferences.widgets })}/></div>
      {error && <Alert action={<Button variant="ghost" onClick={() => void refresh()}>Повторить</Button>}>{error}</Alert>}
      {health ? <Outlet/> : <div className="page-loading" role="status">{error ? 'Данные недоступны' : 'Загружаю ваш день…'}</div>}
    </main>
    {preferences.widgets && <aside className="widgets-panel" aria-label="Виджеты">{widgetBody}</aside>}
    {menu && <Dialog title="Навигация" drawer onClose={() => setMenu(false)}><Sidebar onNavigate={() => setMenu(false)}/></Dialog>}
    {widgetsDrawer && <Dialog title="Виджеты" drawer onClose={() => setWidgetsDrawer(false)}>{health && <WidgetList keys={keys}/>}</Dialog>}
  </div>;
}
