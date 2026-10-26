import { lazy, Suspense, useLayoutEffect } from 'react';
import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router-dom';
import { AuthProvider, useAuth } from './AuthContext';
import { AuthPage } from '../pages/Auth/AuthPage';
import { LegalPage } from '../pages/Legal/LegalPage';
import { legalDocumentAt } from '../pages/Legal/documents';
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
import { FriendsPage } from '../pages/Friends/FriendsPage';
import { ErrorPage } from '../pages/Errors/ErrorPage';
import { SocialProvider } from '../features/social/SocialContext';
const OnboardingPage = lazy(() => import('../pages/Auth/OnboardingPage').then(module => ({ default: module.OnboardingPage })));
export function App() {
  return <BrowserRouter useTransitions={false}><AuthProvider><AuthGate/></AuthProvider></BrowserRouter>;
}
function AuthGate() {
  const { user, loading, error, errorStatus, refresh } = useAuth(); const location = useLocation();
  const legalDocument = legalDocumentAt(location.pathname);
  const surface = legalDocument ? 'legal' : location.pathname === '/onboarding' || user && !user.onboarding_completed ? 'onboarding' : location.pathname === '/auth' || !user ? 'auth' : 'workspace';
  useLayoutEffect(() => {
    document.documentElement.dataset.surface = surface;
    return () => { delete document.documentElement.dataset.surface; };
  }, [surface]);
  if (legalDocument) return <LegalPage document={legalDocument}/>;
  const errorRoute = /^\/(?:errors\/)?([45]\d{2})\/?$/.exec(location.pathname);
  if (errorRoute) return <ErrorPage status={Number(errorRoute[1])}/>;
  if (loading) return <main className="auth-splash"><BrandMark/><span>Гена</span></main>;
  if (error) return <ErrorPage status={errorStatus} onRetry={() => void refresh()}/>;
  if (location.pathname === '/auth') return <AuthPage/>;
  if (!user) return <Navigate to="/auth" replace/>;
  if (!user.onboarding_completed || location.pathname === '/onboarding') return <PreferencesProvider key={user.id}><Suspense fallback={<main className="auth-splash"><BrandMark/><span>Гена</span></main>}><OnboardingPage/></Suspense></PreferencesProvider>;
  return <PreferencesProvider key={user.id}><DataProvider><SocialProvider><EventActionsProvider><Routes><Route element={<AppShell/>}>
    <Route index element={<TodayPage/>}/><Route path="calendar" element={<CalendarPage/>}/><Route path="tasks" element={<TasksPage/>}/><Route path="meetings" element={<MeetingsPage/>}/><Route path="reminders" element={<RemindersPage/>}/><Route path="archive" element={<ArchivePage/>}/><Route path="integrations" element={<IntegrationsPage/>}/><Route path="settings" element={<SettingsPage/>}/>
    <Route path="friends" element={<FriendsPage/>}/><Route path="*" element={<ErrorPage status={404} embedded/>}/>
  </Route></Routes></EventActionsProvider></SocialProvider></DataProvider></PreferencesProvider>;
}
