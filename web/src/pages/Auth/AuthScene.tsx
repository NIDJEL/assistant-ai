import { CalendarDays, Check, Link2, SquareCheck, type LucideIcon } from 'lucide-react';
import { BrandMark } from '../../components/Brand';
import { usePointerSpring } from './useAuthMotion';
import gena from '../../assets/auth/gena.webp';
import calendar from '../../assets/auth/calendar.webp';
import tasks from '../../assets/auth/tasks.webp';
import integrations from '../../assets/auth/integrations.webp';

const illustrations = { gena, calendar, tasks, integrations };
const asset = (name: keyof typeof illustrations) => illustrations[name];

export function GenaMascot({ compact = false }: { compact?: boolean }) {
  return <div className={`gena-mascot ${compact ? 'gena-mascot-compact' : ''}`} aria-hidden="true">
    <div className="gena-mascot-turn"><div className="gena-mascot-float">
      <img src={asset('gena')} width="640" height="640" alt="" fetchPriority={compact ? 'auto' : 'high'} draggable="false"/>
      <div className="gena-face"><span className="gena-eye"/><span className="gena-eye"/><span className="gena-smile"/></div>
    </div></div>
  </div>;
}

const features: { image: keyof typeof illustrations; title: string; text: string; icon: LucideIcon }[] = [
  { image: 'calendar', title: 'Календарь', text: 'Все события\nв одном месте', icon: CalendarDays },
  { image: 'tasks', title: 'Задачи', text: 'Под контролем.\nБез лишних усилий.', icon: SquareCheck },
  { image: 'integrations', title: 'Интеграции', text: 'Яндекс Календарь,\nZoom и другие сервисы', icon: Link2 },
];

function FeatureCard({ image, title, text, icon: Icon, index }: typeof features[number] & { index: number }) {
  const ref = usePointerSpring<HTMLLIElement>();
  return <li className={`auth-feature-reveal auth-feature-${index}`} ref={ref}>
    <div className="auth-feature-card">
      <div className="auth-feature-art"><img src={asset(image)} width="320" height="320" alt="" draggable="false"/></div>
      <div className="auth-feature-copy"><h3>{title}</h3><p>{text}</p><span className="auth-feature-symbol" aria-hidden="true"><Icon size={21} strokeWidth={1.9}/></span></div>
    </div>
  </li>;
}

export function AuthBackdrop() {
  return <div className="auth-backdrop" aria-hidden="true"><div className="auth-ambient"/><div className="auth-orbit auth-orbit-back"/><div className="auth-orbit auth-orbit-front"/><div className="auth-orbit auth-orbit-bottom"/></div>;
}

export function AuthScene() {
  return <section className="auth-information" aria-label="Ваш помощник Гена">
    <div className="auth-brand"><BrandMark gradient/><span>Гена</span></div>
    <div className="auth-pitch"><h1><span>Ваш личный</span><span>ИИ-помощник</span></h1><p className="auth-description">Планируйте, управляйте задачами<br/>и ничего не забывайте</p></div>
    <div className="auth-mascot-scene"><GenaMascot/><span className="auth-floating-check" aria-hidden="true"><Check strokeWidth={4}/></span><span className="auth-spark auth-spark-one" aria-hidden="true"/><span className="auth-spark auth-spark-two" aria-hidden="true"/></div>
    <ul className="auth-features">{features.map((feature, index) => <FeatureCard key={feature.image} {...feature} index={index}/>)}</ul>
  </section>;
}
