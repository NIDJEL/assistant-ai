import { useEffect, useState } from 'react';
import { CalendarDays, ArrowRight, LoaderCircle } from 'lucide-react';
import { Navigate, useNavigate, useSearchParams } from 'react-router-dom';
import { BrandMark } from '../../components/Brand';
import { useAuth } from '../../app/AuthContext';
import { errorMessage } from '../../api/client';
import { startCalendarConnection } from '../../api/integrations';
import { useYandexIntegration } from '../Integrations/useYandexIntegration';
import { AuthWaves } from './AuthPage';
export function OnboardingPage() {
  const { user, completeOnboarding } = useAuth();
  const [params] = useSearchParams(); const failed = !!params.get('error');
  const [step, setStep] = useState(failed ? 1 : 0);
  const [busy, setBusy] = useState(false); const [error, setError] = useState(''); const navigate = useNavigate();
  const { connection, loaded, error: integrationError } = useYandexIntegration();
  const connected = connection?.status === 'connected' || connection?.status === 'pending';
  async function finish() {
    if (busy) return; setBusy(true);
    try { await completeOnboarding(); navigate('/', { replace: true }); }
    catch (e) { setError(errorMessage(e)); setBusy(false); }
  }
  useEffect(() => { if (user && !user.onboarding_completed && connected && !failed) void finish(); }, [connected, user?.id]);
  if (!user) return <Navigate to="/auth" replace/>;
  if (user.onboarding_completed && !busy) return <Navigate to="/" replace/>;
  return <main className="auth-page onboarding-page"><section className="onboarding-card">
    <div className="auth-brand"><BrandMark/><span>Гена</span></div>
    <div className="onboarding-step" key={step}>
      <span className="onboarding-symbol">{step === 0 ? <BrandMark/> : <CalendarDays size={40}/>}</span>
      <h1>{step === 0 ? 'Добро пожаловать! Я Гена' : 'Подключить Яндекс Календарь?'}</h1>
      <p>{step === 0 ? `${user.name}, Ассистент Гена поможет спланировать день. Несколько шагов — и всё готово.` : 'Синхронизируйте встречи и события с Геной.'}</p>
      {step === 1 && <p className="onboarding-note">Разрешите Гене доступ к календарю в Яндексе. После подключения откроется чат.</p>}
      {(error || failed) && <p role="alert" className="auth-error">{error || 'Не удалось подключить Яндекс Календарь'}</p>}
      <button className="auth-primary" disabled={busy || (step===1 && !loaded && !integrationError)} onClick={() => {
        if (step === 0) setStep(1); else { setBusy(true); startCalendarConnection('onboarding'); }
      }}>{busy ? 'Открываем приложение…' : step === 0 ? 'Начать' : failed ? 'Попробовать ещё раз' : 'Подключить Яндекс Календарь'}{busy ? <LoaderCircle className="auth-spinner" size={20}/> : <ArrowRight size={20}/>}</button>
      {step === 1 && <button className="onboarding-later" disabled={busy} onClick={() => void finish()}>Сделать позже</button>}
    </div>
    <div className="onboarding-dots" aria-label={`Шаг ${step + 1} из 2`}>{[0, 1].map(value => <span key={value} className={value === step ? 'active' : ''}/>)}</div>
    <AuthWaves variant="onboarding"/>
  </section></main>;
}
