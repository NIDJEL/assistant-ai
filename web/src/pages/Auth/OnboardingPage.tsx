import { useCallback, useEffect, useRef, useState } from 'react';
import { AnimatePresence, LazyMotion, domAnimation, MotionConfig, m as motion } from 'motion/react';
import { Navigate, useNavigate, useSearchParams } from 'react-router-dom';
import { useAuth } from '../../app/AuthContext';
import { errorMessage } from '../../api/client';
import { startCalendarConnection } from '../../api/integrations';
import { useYandexIntegration } from '../Integrations/useYandexIntegration';
import { usePointerSpring } from './useAuthMotion';
import { OnboardingScene, OnboardingBrand, useOnboardingArtwork } from './OnboardingScene';
import { usePreferences } from '../../app/PreferencesContext';
import { ThemePicker } from '../../components/ThemePicker';
import '../../styles/onboarding.css';

const steps = ['Знакомство', 'Ваш день', 'Интеграции'];
const ease = [.22, 1, .36, 1] as const;
function readStep(key: string, failed: boolean) {
  if (failed) return 2;
  try { const value = Number(sessionStorage.getItem(key)); return [0, 1, 2].includes(value) ? value : 0; }
  catch { return 0; }
}

export function OnboardingPage() {
  const { user, completeOnboarding } = useAuth();
  const { preferences, update, error: preferenceError } = usePreferences();
  const artworkTheme = useOnboardingArtwork(preferences.theme);
  const [params, setParams] = useSearchParams();
  const failed = !!params.get('error');
  const storageKey = `gena.onboarding.v2.${user?.id}`;
  const [step, setStep] = useState(() => readStep(storageKey, failed));
  const [direction, setDirection] = useState(1);
  const [busy, setBusy] = useState<'finish' | 'oauth' | ''>('');
  const [error, setError] = useState('');
  const locked = useRef(false);
  const oauthNavigation = useRef(false);
  const heading = useRef<HTMLHeadingElement>(null);
  const changedStep = useRef(false);
  const navigate = useNavigate();
  const sceneRef = usePointerSpring<HTMLElement>();
  const [systemReduced, setSystemReduced] = useState(() => window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false);
  const [appReduced, setAppReduced] = useState(() => document.documentElement.dataset.motion === 'reduced');
  const reduce = !!systemReduced || appReduced;
  const reveal = {
    enter: { opacity: 0, y: reduce ? 0 : 7 },
    visible: { opacity: 1, y: 0, transition: { duration: reduce ? 0 : .4, ease } },
    leave: { opacity: 0, y: 0, transition: { duration: reduce ? 0 : .16, ease } },
  };
  const { connection, loaded, error: integrationError } = useYandexIntegration();
  const connected = connection?.status === 'connected' || connection?.status === 'pending';

  useEffect(() => {
    // React to OS preference changes without remounting the onboarding or losing its state.
    const preference = window.matchMedia?.('(prefers-reduced-motion: reduce)');
    const changed = () => setSystemReduced(preference?.matches ?? false);
    preference?.addEventListener('change', changed);
    const observer = new MutationObserver(() => setAppReduced(document.documentElement.dataset.motion === 'reduced'));
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['data-motion'] });
    return () => { observer.disconnect(); preference?.removeEventListener('change', changed); };
  }, []);
  useEffect(() => { try { sessionStorage.setItem(storageKey, String(step)); } catch { /* Private browsing can disable storage. */ } }, [step, storageKey]);
  useEffect(() => { if (failed) setStep(2); }, [failed]);
  useEffect(() => {
    // Browser Back from the provider must restore working controls, including bfcache.
    const resume = () => { if (oauthNavigation.current) { oauthNavigation.current = false; locked.current = false; setBusy(''); } };
    window.addEventListener('pageshow', resume);
    return () => window.removeEventListener('pageshow', resume);
  }, []);

  const finish = useCallback(async (target = '/') => {
    if (locked.current) return;
    locked.current = true; setBusy('finish'); setError('');
    try {
      await completeOnboarding();
      try { sessionStorage.removeItem(storageKey); } catch { /* Optional persistence. */ }
      navigate(target, { replace: true });
    } catch (e) { setError(errorMessage(e)); locked.current = false; setBusy(''); }
  }, [completeOnboarding, navigate, storageKey]);
  useEffect(() => {
    if (user && !user.onboarding_completed && connected && !failed) void finish();
    // Attempt once when a connection arrives; failures remain retryable.
  }, [connected, user?.id, failed]);

  function go(next: number) {
    if (locked.current || next === step) return;
    changedStep.current = true; setDirection(next > step ? 1 : -1); setStep(next); setError('');
    if (failed) setParams({}, { replace: true });
  }
  function connect() {
    if (locked.current) return;
    locked.current = true; oauthNavigation.current = true; setBusy('oauth'); setError('');
    try { startCalendarConnection('onboarding'); }
    catch (e) { oauthNavigation.current = false; locked.current = false; setBusy(''); setError(errorMessage(e)); }
  }
  function focusHeading() { if (changedStep.current) { heading.current?.focus({ preventScroll: true }); changedStep.current = false; } }
  if (!user) return <Navigate to="/auth" replace/>;
  if (user.onboarding_completed && !busy) return <Navigate to="/" replace/>;

  const title = step === 0 ? <>Добро пожаловать.<br/><span>Я Гена.</span></> : step === 1 ? <>Ваш день.<br/><span>Всё по плану.</span></> : <>Важное — рядом.<br/><span>Рутина — на мне.</span></>;
  const description = step === 0
    ? `${user.name}, давайте освободим время для важного. Я помогу собрать задачи, спланировать день и ничего не упустить.`
    : step === 1 ? 'Собирайте задачи в удобный план, распределяйте их по времени и держите фокус на важном.'
      : 'Встречи, события и напоминания — в одном ритме. Подключите календарь, а я помогу держать всё под контролем.';
  return <LazyMotion features={domAnimation} strict><MotionConfig reducedMotion={reduce ? 'always' : 'user'} transition={{ duration: .55, ease }}>
    <main ref={sceneRef} className="gena-onboarding" data-step={step} data-reduced={reduce}>
      <div className="ob-ambient ob-ambient-one" aria-hidden="true"/><div className="ob-ambient ob-ambient-two" aria-hidden="true"/>
      <motion.section className="ob-card" aria-label="Знакомство с Геной" initial={reduce ? false : { opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }}>
        <header className="ob-header">
          <OnboardingBrand theme={artworkTheme} reduce={reduce}/>
        </header>
        <div className="ob-body">
          <div className="ob-copy-shell"><AnimatePresence mode="wait" custom={direction} initial={false}>
            <motion.div className="ob-copy" key={step} custom={direction}
              variants={{ enter: (d: number) => ({ opacity: 0, x: reduce ? 0 : 16 * d }), visible: { opacity: 1, x: 0, transition: { staggerChildren: reduce ? 0 : .045 } }, leave: (d: number) => ({ opacity: 0, x: reduce ? 0 : -12 * d }) }}
              initial="enter" animate="visible" exit="leave" transition={{ duration: reduce ? 0 : .24, ease }} onAnimationComplete={phase => { if (phase === 'visible') focusHeading(); }}>
              <motion.h1 variants={reveal} ref={heading} tabIndex={-1} id="onboarding-title">{title}</motion.h1>
              <motion.p variants={reveal} className="ob-description">{description}</motion.p>
              <motion.div variants={reveal} className="ob-detail"><span className="ob-detail-line"/><p>{step === 0 ? 'Несколько простых шагов — и мы знакомы.' : step === 1 ? 'Задачи, календарь и фокус — в одном месте.' : 'Яндекс Календарь, Google Календарь и Zoom.'}</p></motion.div>
              {step === 0 && <motion.div variants={reveal} className="ob-theme-card"><ThemePicker theme={preferences.theme} onChange={theme => update({ theme })} disabled={!!busy}/>{preferenceError && <p className="ob-theme-error" role="alert">Тема применена на этом устройстве. {preferenceError}</p>}</motion.div>}
              {step === 2 && <motion.p variants={reveal} className="ob-permission">Доступ к календарю вы подтвердите на странице Яндекса. Другие сервисы можно подключить в интеграциях.</motion.p>}
            </motion.div>
          </AnimatePresence></div>
          <div className="ob-scene-shell"><OnboardingScene step={step} reduce={reduce} theme={artworkTheme}/></div>
        </div>
        <footer className="ob-footer">
          <nav className="ob-progress" aria-label={`Шаг ${step + 1} из 3`}>{steps.map((label, value) => <button key={label} type="button" aria-label={`Шаг ${value + 1}: ${label}`} aria-current={value === step ? 'step' : undefined} disabled={!!busy} className={value === step ? 'is-active' : value < step ? 'is-done' : ''} onClick={() => go(value)}>
            <span className="ob-progress-track"><motion.span animate={{ scaleX: value <= step ? 1 : 0 }} transition={{ duration: reduce ? 0 : .55, ease }}/></span><span className="ob-progress-label"><b>0{value + 1}</b>{label}</span>
          </button>)}</nav>
          <div className="ob-controls">
            <div className="ob-feedback" aria-live="polite">
              {(error || failed) && <p role="alert">{error || 'Не удалось подключить Яндекс Календарь'}</p>}
              {step === 2 && integrationError && !error && !failed && <p role="alert">{integrationError} Можно повторить подключение или сделать это позже.</p>}
            </div>
            <div className="ob-actions">
              {step > 0 && <button type="button" className="ob-back" disabled={!!busy} onClick={() => go(step - 1)} aria-label="Назад"><svg width="23" height="23" viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M20 12H5m6-6-6 6 6 6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/></svg></button>}
              <motion.button type="button" className="ob-primary" aria-label={step === 2 && !busy && !failed ? 'Подключить Яндекс Календарь' : undefined} disabled={!!busy || (step === 2 && !loaded && !integrationError)} whileTap={reduce ? undefined : { scale: .98 }} onClick={() => step < 2 ? go(step + 1) : connect()}>
                <span>{busy === 'finish' ? 'Открываем приложение…' : busy === 'oauth' ? 'Переходим в Яндекс…' : step === 0 ? 'Начать' : step === 1 ? 'Далее' : failed ? 'Попробовать ещё раз' : <><span className="ob-label-desktop">Подключить Яндекс Календарь</span><span className="ob-label-mobile">Подключить календарь</span></>}</span>
                {busy ? <span className="ob-spinner" aria-hidden="true"/> : <svg className="ob-arrow" width="23" height="23" viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M4 12h15m-6-6 6 6-6 6" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"/></svg>}
              </motion.button>
            </div>
            <div className="ob-secondary">{step === 2 ? <><button type="button" disabled={!!busy} onClick={() => void finish('/integrations')}>Все интеграции</button><span aria-hidden="true">·</span><button type="button" disabled={!!busy} onClick={() => void finish()}>Сделать позже</button></> : <button type="button" disabled={!!busy} onClick={() => void finish()}>Пропустить знакомство</button>}</div>
          </div>
        </footer>
      </motion.section>
    </main>
  </MotionConfig></LazyMotion>;
}
