import { AnimatePresence, m as motion } from 'motion/react';
import ghost from '../../assets/onboarding/ghost.webp';
import tasks from '../../assets/onboarding/tasks.webp';
import board from '../../assets/onboarding/board.webp';
import reminder from '../../assets/onboarding/reminder.webp';
import yandex from '../../assets/onboarding/yandex.webp';
import google from '../../assets/onboarding/google.webp';
import zoom from '../../assets/onboarding/zoom.webp';
import sparkle from '../../assets/onboarding/sparkle.webp';
import logo from '../../assets/onboarding/logo.webp';
export const onboardingAssets = { logo };

/** Artwork is decorative; every control is a real button in the surrounding page. */
export function OnboardingScene({ step, reduce }: { step: number; reduce: boolean }) {
  return <div className="ob-scene" aria-hidden="true">
    <div className="ob-orbit ob-orbit-outer"/><div className="ob-orbit ob-orbit-inner"/><div className="ob-scene-halo"/>
    <div className="ob-spark ob-spark-one"><img src={sparkle} alt="" width="120" height="120"/></div><div className="ob-spark ob-spark-two"><img src={sparkle} alt="" width="120" height="120"/></div>
    <span className="ob-particle ob-particle-one"/><span className="ob-particle ob-particle-two"/>
    <AnimatePresence mode="sync" initial={false}>
      <motion.div key={step} className={`ob-art ob-art-${step}`} initial={{ opacity: 0, scale: reduce ? 1 : .94, y: reduce ? 0 : 12 }} animate={{ opacity: 1, scale: 1, y: 0 }} exit={{ opacity: 0, scale: reduce ? 1 : 1.035 }} transition={{ duration: reduce ? 0 : .5, ease: [.22, 1, .36, 1] }}>
        {step === 0 && <>
          <div className="ob-art-piece ob-welcome-tasks"><img src={tasks} alt="" width="580" height="435"/></div>
          <div className="ob-art-piece ob-ghost"><img src={ghost} alt="" width="700" height="700" fetchPriority="high"/></div>
          <div className="ob-scene-caption"><span/>Ваш персональный ассистент</div>
        </>}
        {step === 1 && <>
          <div className="ob-art-piece ob-board"><img src={board} alt="" width="800" height="600"/></div>
          <div className="ob-art-piece ob-plan-tasks"><img src={tasks} alt="" width="580" height="435"/></div>
          <div className="ob-mini-ghost"><img src={ghost} alt="" width="700" height="700"/></div>
        </>}
        {step === 2 && <>
          <svg className="ob-connectors" viewBox="0 0 540 470" fill="none"><path d="M124 126C124 220 185 205 220 270M272 103V270M422 126C422 219 363 214 322 270" stroke="url(#ob-connection)" strokeWidth="1.5" strokeDasharray="5 8"/><defs><linearGradient id="ob-connection" x2="0" y2="1"><stop stopColor="#0E7AFF"/><stop offset="1" stopColor="#6edaff" stopOpacity=".25"/></linearGradient></defs></svg>
          <div className="ob-art-piece ob-service ob-service-yandex"><img src={yandex} alt="" width="220" height="220"/></div>
          <div className="ob-art-piece ob-service ob-service-google"><img src={google} alt="" width="220" height="220"/></div>
          <div className="ob-art-piece ob-service ob-service-zoom"><img src={zoom} alt="" width="220" height="220"/></div>
          <div className="ob-art-piece ob-reminder"><img src={reminder} alt="" width="760" height="570"/></div>
          <div className="ob-scene-caption"><span/>На одной волне с вашими сервисами</div>
        </>}
      </motion.div>
    </AnimatePresence>
    <div className="ob-wave ob-wave-back"/><div className="ob-wave ob-wave-front"/>
  </div>;
}
