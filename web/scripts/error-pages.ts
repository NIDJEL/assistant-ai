import { readFileSync } from 'node:fs';
import { errorContent } from '../src/pages/Errors/content.ts';

const mark = '<svg viewBox="0 0 64 42" aria-hidden="true"><path d="M6 31 Q26 -3 31 18 T58 12" fill="none" stroke="currentColor" stroke-width="8" stroke-linecap="round"/></svg>';
export function staticErrorPages() {
  const css = readFileSync(new URL('../src/styles/errors.css', import.meta.url), 'utf8');
  return Array.from({ length: 200 }, (_, index) => 400 + index).map(status => {
    const { title, description } = errorContent(status);
    const retry = status >= 500 || status === 408 || status === 429;
    const primary = status === 401 ? '<a class="error-primary" href="/auth">Войти в аккаунт →</a>' : retry ? '<a class="error-primary" href="">Попробовать снова ↻</a>' : '<a class="error-primary" href="/">На главную →</a>';
    return { type: 'asset' as const, fileName: `errors/${status}.html`, source: `<!doctype html><html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover"><meta name="robots" content="noindex"><title>${status} — ${title} · Гена</title><style>html,body{margin:0} ${css}</style></head><body><main class="error-screen"><a class="error-brand" href="/" aria-label="Гена — Главная">${mark}<span>Гена</span></a><section class="error-content" aria-labelledby="error-title"><div class="error-orbit" aria-hidden="true"><span></span><span></span>${mark}</div><p class="error-code">${status}</p><h1 id="error-title">${title}</h1><p class="error-description">${description}</p><div class="error-actions">${primary}${retry || status === 401 ? '<a class="error-secondary" href="/">На главную</a>' : '<a class="error-secondary" href="/auth">Войти в аккаунт</a>'}</div></section><p class="error-footnote">Рутина подождёт. Мы поможем вернуться к делам.</p></main></body></html>` };
  });
}
