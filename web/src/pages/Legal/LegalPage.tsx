import { useEffect } from 'react';
import { ArrowLeft, ArrowUpRight, FileText } from 'lucide-react';
import { BrandMark } from '../../components/Brand';
import { legalDocuments, LEGAL_DATE, type LegalDocument } from './documents';
import './legal.css';

export function LegalPage({ document }: { document: LegalDocument }) {
  useEffect(() => {
    const previous = window.document.title;
    window.document.title = `${document.title} — Ассистент Гена`;
    return () => { window.document.title = previous; };
  }, [document.title]);
  return <main className="legal-page">
    <header className="legal-header"><a className="legal-brand" href="/auth" aria-label="Гена — вернуться в приложение"><BrandMark gradient/><span>Гена</span></a><a className="legal-back" href="/auth"><ArrowLeft size={17}/>В приложение</a></header>
    <div className="legal-shell">
      <aside className="legal-sidebar"><span className="legal-eyebrow"><FileText size={16}/>Документы сервиса</span><nav className="legal-documents" aria-label="Документы">{legalDocuments.map(item => <a key={item.path} href={item.path} aria-current={item.path === document.path ? 'page' : undefined}>{item.label}<ArrowUpRight size={15}/></a>)}</nav><nav className="legal-toc" aria-label="Содержание"><h2>На этой странице</h2>{document.sections.map(section => <a key={section.id} href={`#${section.id}`}>{section.title}</a>)}</nav><p className="legal-contact">Вопросы по документам?<br/><a href="mailto:nikitaryabip@gmail.com">nikitaryabip@gmail.com</a></p></aside>
      <article className="legal-article"><header className="legal-title"><p className="legal-eyebrow">Ассистент Гена · Правовая информация</p><h1>{document.title}</h1><p className="legal-intro">{document.intro}</p><p className="legal-version">Редакция от <time dateTime="2026-10-20">{LEGAL_DATE}</time></p></header>
        <details className="legal-mobile-toc"><summary>Содержание документа</summary><nav aria-label="Содержание">{document.sections.map(section => <a key={section.id} href={`#${section.id}`}>{section.title}</a>)}</nav></details>
        {document.sections.map(section => <section id={section.id} key={section.id} className="legal-section" aria-labelledby={`heading-${section.id}`}><h2 id={`heading-${section.id}`}>{section.title}</h2>{section.content}</section>)}
        <footer className="legal-footer"><a href="#top" onClick={e => { e.preventDefault(); window.scrollTo({ top: 0, behavior: 'instant' }); }}>К началу документа ↑</a><a href="/auth">Вернуться к Гене</a></footer>
      </article>
    </div>
  </main>;
}
