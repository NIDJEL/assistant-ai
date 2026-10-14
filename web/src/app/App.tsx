import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router-dom';
import { AuthProvider, useAuth } from './AuthContext';
import { AuthPage } from '../pages/Auth/AuthPage';
import { OnboardingPage } from '../pages/Auth/OnboardingPage';
import { BrandMark } from '../components/Brand';
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
  return <BrowserRouter useTransitions={false}><AuthProvider><AuthGate/></AuthProvider></BrowserRouter>;
}
function AuthGate() {
  const { user, loading, error, refresh } = useAuth(); const location = useLocation();
  if (loading || error) return <main className="auth-splash"><BrandMark/><span>{error || 'Гена'}</span>{error && <button className="auth-primary" onClick={() => void refresh()}>Повторить</button>}</main>;
  if (location.pathname === '/terms' || location.pathname === '/privacy') return <main className="auth-page"><article className="auth-policy"><h1>{location.pathname === '/terms' ? 'Условия использования' : 'Конфиденциальность'}</h1><p>Гена — приложение для личного планирования. Вы управляете своими задачами, встречами и напоминаниями.</p><p>Данные аккаунта и история сохраняются на сервере владельца этой установки. Для входа используются почта и хеш пароля либо идентификатор выбранного OAuth-сервиса. Календарь подключается отдельно. Cookies нужны для сессии и безопасного входа; рекламные cookies не используются.</p><p>Перед использованием с личными данными уточните у владельца установки условия хранения и удаления данных. Эти сведения описывают работу приложения и должны быть дополнены оператором публичной установки.</p><a href="/auth">Вернуться к Гене</a></article></main>;
  if (location.pathname === '/auth') return <AuthPage/>;
  if (!user) return <Navigate to="/auth" replace/>;
  if (!user.onboarding_completed || location.pathname === '/onboarding') return <OnboardingPage/>;
  return <PreferencesProvider key={user.id}><DataProvider><EventActionsProvider><Routes><Route element={<AppShell/>}>
    <Route index element={<TodayPage/>}/><Route path="calendar" element={<CalendarPage/>}/><Route path="tasks" element={<TasksPage/>}/><Route path="meetings" element={<MeetingsPage/>}/><Route path="reminders" element={<RemindersPage/>}/><Route path="archive" element={<ArchivePage/>}/><Route path="integrations" element={<IntegrationsPage/>}/><Route path="settings" element={<SettingsPage/>}/>
    <Route path="*" element={<div className="page"><h1>Страница не найдена</h1><a href="/">Вернуться на Сегодня</a></div>}/>
  </Route></Routes></EventActionsProvider></DataProvider></PreferencesProvider>;
}
