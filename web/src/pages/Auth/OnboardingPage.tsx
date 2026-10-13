import { useState } from 'react';
import { CalendarDays, Check, ArrowRight, LoaderCircle } from 'lucide-react';
import { Navigate, useNavigate } from 'react-router-dom';
import { BrandMark } from '../../components/Brand';
import { useAuth } from '../../app/AuthContext';
import { errorMessage } from '../../api/client';
import { AuthWaves } from './AuthPage';
export function OnboardingPage() {
  const { user, completeOnboarding } = useAuth(); const [step, setStep] = useState(0); const [calendar, setCalendar] = useState(false); const [busy, setBusy] = useState(false); const [error, setError] = useState(''); const navigate = useNavigate();
  if (!user) return <Navigate to="/auth" replace/>; if (user.onboarding_completed && !busy) return <Navigate to="/" replace/>;
  async function finish() { if (busy) return; setBusy(true); try { await completeOnboarding(); navigate(calendar ? '/integrations?service=yandex' : '/', { replace: true }); } catch (e) { setError(errorMessage(e)); setBusy(false); } }
  return <main className="auth-page onboarding-page"><section className="onboarding-card"><div className="auth-brand"><BrandMark/><span>Ритм</span></div><div className="onboarding-step" key={step}><span className="onboarding-symbol">{step === 0 ? <BrandMark/> : step === 1 ? <CalendarDays size={40}/> : <Check size={42}/>}</span><h1>{step === 0 ? 'Добро пожаловать в Ритм' : step === 1 ? 'Подключить Яндекс Календарь?' : 'Всё готово'}</h1><p>{step === 0 ? `${user.name}, несколько шагов — и всё готово.` : step === 1 ? 'Синхронизируйте встречи и события с Ритм.' : 'Планируйте день, создавайте задачи и ничего не забывайте.'}</p>{step === 1 && <p className="onboarding-note">Календарь подключается отдельно. Мы откроем настройки подключения после знакомства с Ритмом.</p>}{error && <p role="alert" className="auth-error">{error}</p>}<button className="auth-primary" disabled={busy} onClick={() => step === 2 ? void finish() : (step === 1 && setCalendar(true), setStep(step + 1))}>{busy ? 'Открываем Ритм…' : step === 0 ? 'Начать' : step === 1 ? 'Подключить Яндекс Календарь' : 'Перейти в Ритм'}{busy ? <LoaderCircle className="auth-spinner" size={20}/> : <ArrowRight size={20}/>}</button>{step === 1 && <button className="onboarding-later" onClick={() => { setCalendar(false); setStep(2); }}>Сделать позже</button>}{step === 1 && <input type="hidden" value={String(calendar)}/>}</div><div className="onboarding-dots" aria-label={`Шаг ${step + 1} из 3`}>{[0, 1, 2].map(value => <span key={value} className={value === step ? 'active' : ''}/>)}</div><AuthWaves/></section></main>;
}
