import { AnimatePresence, m as motion } from 'motion/react';
import { useEffect, useState } from 'react';
import type { Theme } from '../../app/theme';
import ghost from '../../assets/onboarding/ghost.webp';
import tasks from '../../assets/onboarding/tasks.webp';
import board from '../../assets/onboarding/board.webp';
import reminder from '../../assets/onboarding/reminder.webp';
import yandex from '../../assets/onboarding/yandex.webp';
import google from '../../assets/onboarding/google.webp';
import zoom from '../../assets/onboarding/zoom.webp';
import sparkle from '../../assets/onboarding/sparkle.webp';
import logo from '../../assets/onboarding/logo.webp';
import lightGhost from '../../assets/onboarding/light/ghost.webp';
import lightTasks from '../../assets/onboarding/light/tasks.webp';
import lightPlanner from '../../assets/onboarding/light/planner.webp';
import lightReminder from '../../assets/onboarding/light/reminder.webp';
import lightYandex from '../../assets/onboarding/light/yandex.webp';
import lightGoogle from '../../assets/onboarding/light/google.webp';
import lightZoom from '../../assets/onboarding/light/zoom.webp';
import lightLogo from '../../assets/onboarding/light/logo.webp';

const artwork = {
  dark: { ghost, tasks, board, reminder, yandex, google, zoom, logo },
  light: { ghost: lightGhost, tasks: lightTasks, board: lightPlanner, reminder: lightReminder, yandex: lightYandex, google: lightGoogle, zoom: lightZoom, logo: lightLogo },
};
// Decode before a theme crossfade so a cold cache cannot leave an empty scene.
export function useOnboardingArtwork(theme: Theme) {
  const [readyTheme, setReadyTheme] = useState(theme);
  useEffect(() => {
    let current = true;
    const images = Object.values(artwork[theme]).map(src => {
      const image = new Image(); image.src = src;
      return image.decode ? image.decode() : new Promise<void>((resolve, reject) => { image.onload = () => resolve(); image.onerror = reject; });
    });
    void Promise.all(images).then(() => { if (current) setReadyTheme(theme); }).catch(() => { /* Retain the loaded artwork if an asset is unavailable. */ });
    return () => { current = false; };
  }, [theme]);
  return readyTheme;
}

export function OnboardingBrand({ theme, reduce }: { theme: Theme; reduce: boolean }) {
  return <div className="ob-brand" aria-label="Гена"><AnimatePresence initial={false}>
    <motion.span className="ob-brand-lockup" key={theme} aria-hidden="true" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={{ duration: reduce ? 0 : .35 }}>
      {theme === 'light' ? <img className="ob-brand-light" src={lightLogo} width="200" height="57" alt=""/> : <><img src={logo} width="56" height="31" alt=""/><span>Гена</span></>}
    </motion.span>
  </AnimatePresence></div>;
}

/** Artwork is decorative; every control is a real button in the surrounding page. */
export function OnboardingScene({ step, reduce, theme }: { step: number; reduce: boolean; theme: Theme }) {
  const assets = artwork[theme];
  return <div className="ob-scene" aria-hidden="true" data-artwork-theme={theme}>
    <div className="ob-orbit ob-orbit-outer"/><div className="ob-orbit ob-orbit-inner"/><div className="ob-scene-halo"/>
    <div className="ob-spark ob-spark-one"><img src={sparkle} alt="" width="120" height="120"/></div><div className="ob-spark ob-spark-two"><img src={sparkle} alt="" width="120" height="120"/></div>
    <span className="ob-particle ob-particle-one"/><span className="ob-particle ob-particle-two"/>
    <AnimatePresence mode="sync" initial={false}>
      <motion.div key={`${step}-${theme}`} className={`ob-art ob-art-${step} ob-art-${theme}`} initial={{ opacity: 0, scale: reduce ? 1 : .98, y: reduce ? 0 : 8 }} animate={{ opacity: 1, scale: 1, y: 0 }} exit={{ opacity: 0 }} transition={{ duration: reduce ? 0 : .5, ease: [.22, 1, .36, 1] }}>
        {step === 0 && <>
          <div className="ob-art-piece ob-welcome-tasks"><img src={assets.tasks} alt="" width="580" height="435"/></div>
          <div className="ob-art-piece ob-ghost"><img src={assets.ghost} alt="" width="700" height="700" fetchPriority="high"/></div>
          <div className="ob-scene-caption"><span/>Ваш персональный ассистент</div>
        </>}
        {step === 1 && <>
          <div className="ob-art-piece ob-board"><img src={assets.board} alt="" width="800" height="600"/></div>
          {theme === 'dark' && <><div className="ob-art-piece ob-plan-tasks"><img src={tasks} alt="" width="580" height="435"/></div>
          <div className="ob-mini-ghost"><img src={ghost} alt="" width="700" height="700"/></div></>}
        </>}
        {step === 2 && <>
          <svg className="ob-connectors" viewBox="0 0 540 470" fill="none"><path d="M124 126C124 220 185 205 220 270M272 103V270M422 126C422 219 363 214 322 270" stroke={`url(#ob-connection-${theme})`} strokeWidth="1.5" strokeDasharray="5 8"/><defs><linearGradient id={`ob-connection-${theme}`} x2="0" y2="1"><stop stopColor="#0E7AFF"/><stop offset="1" stopColor="#6edaff" stopOpacity=".25"/></linearGradient></defs></svg>
          <div className="ob-art-piece ob-service ob-service-yandex"><img src={assets.yandex} alt="" width="220" height="220"/></div>
          <div className="ob-art-piece ob-service ob-service-google"><img src={assets.google} alt="" width="220" height="220"/></div>
          <div className="ob-art-piece ob-service ob-service-zoom"><img src={assets.zoom} alt="" width="220" height="220"/></div>
          <div className="ob-art-piece ob-reminder"><img src={assets.reminder} alt="" width="760" height="570"/></div>
          <div className="ob-scene-caption"><span/>На одной волне с вашими сервисами</div>
        </>}
      </motion.div>
    </AnimatePresence>
    <div className="ob-wave ob-wave-back"/><div className="ob-wave ob-wave-front"/>
  </div>;
}
