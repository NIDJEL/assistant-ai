import { useEffect, useState } from 'react';
import { Presence } from '../components/motion/Presence';
import { motionUpdate } from '../components/motion/motion';
import { Outlet, useLocation, useSearchParams } from 'react-router-dom';
import { Menu, PanelRight } from 'lucide-react';
import { Sidebar } from '../components/Sidebar';
import { Dialog } from '../components/Dialog';
import { Alert, Button, IconButton } from '../components/UI';
import { WidgetPanel } from '../features/widgets/WidgetPanel';
import { WidgetRegion } from '../features/widgets/WidgetRegion';
import { IntegrationDetail, services } from '../pages/Integrations/IntegrationsPage';
import { usePreferences } from './PreferencesContext';
import { useData } from './DataContext';
import { ReminderNotifications } from '../features/notifications/ReminderNotifications';
import { ChatNotificationsProvider } from '../features/assistant/ChatNotifications';
export function AppShell() {
  return <ChatNotificationsProvider><AppShellContent/></ChatNotificationsProvider>;
}
function AppShellContent() {
  const prefs = usePreferences(); const { preferences, update } = prefs; const { health, error, refresh } = useData(); const location = useLocation(); const [params, setParams] = useSearchParams();
  const [menu, setMenu] = useState(false); const [widgetsDrawer, setWidgetsDrawer] = useState(false);
  useEffect(() => { setMenu(false); setWidgetsDrawer(false); }, [location.pathname]);
  const service = location.pathname === '/integrations' && services.some(s => s.id === params.get('service')) ? params.get('service') : null;
  const panelVisible = preferences.widgets || !!service;
  const panel = service ? <IntegrationDetail key={service} id={service} onBack={() => motionUpdate(() => setParams({}))}/> : <WidgetPanel onCollapse={() => update({ widgets: false })}/>;
  return <div className={`app-shell ${panelVisible ? '' : 'widgets-hidden'} ${preferences.navCollapsed ? 'nav-collapsed' : ''}`}>
    <ReminderNotifications/>
    <a className="skip-link" href="#main-content">К содержимому</a><aside className="sidebar"><Sidebar collapsible/></aside>
    <main className="main-panel" id="main-content"><div className="shell-controls"><IconButton icon={Menu} label="Открыть навигацию" className="mobile-menu" onClick={() => setMenu(true)}/><button type="button" className="drawer-widget-button" aria-label="Открыть виджеты" onClick={() => setWidgetsDrawer(true)}><PanelRight size={19}/><span>{service ? 'Интеграция' : 'Виджеты'}</span></button>{!panelVisible && <IconButton icon={PanelRight} label="Показать виджеты" className="desktop-widget-button" onClick={() => update({ widgets: true })}/>}</div>
      {error && <Alert action={<Button variant="ghost" onClick={() => void refresh()}>Повторить</Button>}>{error}</Alert>}{prefs.error && <Alert action={<Button variant="ghost" onClick={prefs.retry}>Повторить сохранение</Button>}>Настройки не сохранены: {prefs.error}</Alert>}
      {health && prefs.ready ? <Outlet/> : <div className="page-loading" role="status">{error ? 'Данные недоступны' : 'Загружаю ваш день…'}</div>}
    </main>
    <WidgetRegion visible={panelVisible} integration={!!service}>{health && prefs.ready && panel}</WidgetRegion>
    <Presence present={menu}>{menu && <Dialog title="Навигация" drawer side="left" onClose={() => setMenu(false)}><Sidebar onNavigate={() => setMenu(false)}/></Dialog>}</Presence>
    <Presence present={widgetsDrawer}>{widgetsDrawer && <Dialog title={service ? 'Интеграция' : 'Виджеты'} drawer onClose={() => setWidgetsDrawer(false)}>{service ? panel : <WidgetPanel key={location.pathname} drawer/>}</Dialog>}</Presence>
  </div>;
}
