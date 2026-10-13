import { useState } from 'react';
import { Outlet, useLocation, useSearchParams } from 'react-router-dom';
import { Menu, PanelRight, ChevronRight } from 'lucide-react';
import { Sidebar } from '../components/Sidebar';
import { Dialog } from '../components/Dialog';
import { Alert, Button, IconButton } from '../components/UI';
import { WidgetPanel } from '../features/widgets/WidgetPanel';
import { IntegrationDetail, services } from '../pages/Integrations/IntegrationsPage';
import { usePreferences } from './PreferencesContext';
import { useData } from './DataContext';
export function AppShell() {
  const prefs = usePreferences(); const { preferences, update } = prefs; const { health, error, refresh } = useData(); const location = useLocation(); const [params, setParams] = useSearchParams();
  const [menu, setMenu] = useState(false); const [widgetsDrawer, setWidgetsDrawer] = useState(false);
  const service = location.pathname === '/integrations' && services.some(s => s.id === params.get('service')) ? params.get('service') : null;
  const panelVisible = preferences.widgets || !!service;
  const panel = service ? <IntegrationDetail id={service} onBack={() => setParams({})}/> : <WidgetPanel key={location.pathname}/>;
  return <div className={`app-shell ${panelVisible ? '' : 'widgets-hidden'}`}>
    <a className="skip-link" href="#main-content">К содержимому</a><aside className="sidebar"><Sidebar/></aside>
    <main className="main-panel" id="main-content"><div className="shell-controls"><IconButton icon={Menu} label="Открыть навигацию" className="mobile-menu" onClick={() => setMenu(true)}/><button type="button" className="drawer-widget-button" aria-label="Открыть виджеты" onClick={() => setWidgetsDrawer(true)}><PanelRight size={19}/><span>{service ? 'Интеграция' : 'Виджеты'}</span></button>{panelVisible && !service && <IconButton icon={ChevronRight} label="Скрыть виджеты" className="desktop-widget-button" onClick={() => update({ widgets: false })}/ >}{!panelVisible && <IconButton icon={PanelRight} label="Показать виджеты" className="desktop-widget-button" onClick={() => update({ widgets: true })}/>}</div>
      {error && <Alert action={<Button variant="ghost" onClick={() => void refresh()}>Повторить</Button>}>{error}</Alert>}{prefs.error && <Alert action={<Button variant="ghost" onClick={prefs.retry}>Повторить сохранение</Button>}>Настройки не сохранены: {prefs.error}</Alert>}
      {health && prefs.ready ? <Outlet/> : <div className="page-loading" role="status">{error ? 'Данные недоступны' : 'Загружаю ваш день…'}</div>}
    </main>
    {panelVisible && <aside className={`widgets-panel ${service ? 'integration-panel' : ''}`} aria-label={service ? 'Интеграция' : 'Виджеты'}>{health && prefs.ready && panel}</aside>}
    {menu && <Dialog title="Навигация" drawer onClose={() => setMenu(false)}><Sidebar onNavigate={() => setMenu(false)}/></Dialog>}
    {widgetsDrawer && <Dialog title={service ? 'Интеграция' : 'Виджеты'} drawer onClose={() => setWidgetsDrawer(false)}>{service ? panel : <WidgetPanel key={location.pathname} drawer/>}</Dialog>}
  </div>;
}
