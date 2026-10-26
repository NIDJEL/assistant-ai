import { useEffect } from 'react';
import { ArrowLeft, ArrowRight, RefreshCw } from 'lucide-react';
import { BrandMark } from '../../components/Brand';
import { errorContent } from './content';

export function ErrorPage({ status = 500, onRetry, embedded = false }: { status?: number; onRetry?: () => void; embedded?: boolean }) {
  const { title, description } = errorContent(status);
  useEffect(() => {
    const previous = document.title;
    document.title = `${status || 'Нет связи'} — ${title} · Гена`;
    return () => { document.title = previous; };
  }, [status, title]);
  const retry = onRetry || (() => window.location.reload());
  const Container = embedded ? 'section' : 'main';
  return <Container className={`error-screen ${embedded ? 'page error-embedded' : ''}`}>
    <a className="error-brand" href="/" aria-label="Гена — Главная"><BrandMark/><span>Гена</span></a>
    <section className="error-content" aria-labelledby="error-title">
      <div className="error-orbit" aria-hidden="true"><span/><span/><BrandMark/></div>
      <p className="error-code">{status || 'Офлайн'}</p>
      <h1 id="error-title" tabIndex={-1}>{title}</h1><p className="error-description">{description}</p>
      <div className="error-actions">{status === 401 ? <a className="error-primary" href="/auth">Войти в аккаунт<ArrowRight size={18}/></a> : status >= 500 || status === 0 || status === 408 || status === 429 ? <button className="error-primary" type="button" onClick={retry}><RefreshCw size={18}/>Попробовать снова</button> : <a className="error-primary" href="/">На главную<ArrowRight size={18}/></a>}
        {status >= 500 || status === 0 || status === 408 || status === 429 || status === 401 ? <a className="error-secondary" href="/">На главную</a> : <button type="button" className="error-secondary" onClick={() => { if (window.history.length > 1) window.history.back(); else window.location.assign('/'); }}><ArrowLeft size={17}/>Назад</button>}
      </div>
    </section><p className="error-footnote">Рутина подождёт. Мы поможем вернуться к делам.</p>
  </Container>;
}
