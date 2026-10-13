import { BrowserRouter, Route, Routes } from 'react-router-dom';
import { DataProvider } from './DataContext';
import { PreferencesProvider } from './PreferencesContext';
import { EventActionsProvider } from './EventActionsContext';
import { AppShell } from './AppShell';
import { TodayPage } from '../pages/Today/TodayPage';
import { CalendarPage } from '../pages/Calendar/CalendarPage';
import { TasksPage } from '../pages/Tasks/TasksPage';
import { MeetingsPage } from '../pages/Meetings/MeetingsPage';
import { RemindersPage } from '../pages/Reminders/RemindersPage';
import { ArchivePage } from '../pages/Archive/ArchivePage';
import { IntegrationsPage } from '../pages/Integrations/IntegrationsPage';
import { SettingsPage } from '../pages/Settings/SettingsPage';
export function App() {
  return <BrowserRouter useTransitions={false}><PreferencesProvider><DataProvider><EventActionsProvider><Routes><Route element={<AppShell/>}>
    <Route index element={<TodayPage/>}/><Route path="calendar" element={<CalendarPage/>}/><Route path="tasks" element={<TasksPage/>}/><Route path="meetings" element={<MeetingsPage/>}/><Route path="reminders" element={<RemindersPage/>}/><Route path="archive" element={<ArchivePage/>}/><Route path="integrations" element={<IntegrationsPage/>}/><Route path="settings" element={<SettingsPage/>}/>
    <Route path="*" element={<div className="page"><h1>Страница не найдена</h1><a href="/">Вернуться на Сегодня</a></div>}/>
  </Route></Routes></EventActionsProvider></DataProvider></PreferencesProvider></BrowserRouter>;
}
