export type Theme = 'dark' | 'light';
const key = 'gena.theme.v1';

export function savedTheme(): Theme {
  try { return localStorage.getItem(key) === 'light' ? 'light' : 'dark'; }
  catch { return 'dark'; }
}

export function applyTheme(theme: Theme) {
  document.documentElement.dataset.theme = theme;
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', theme === 'light' ? '#f3f7fc' : '#080f21');
  document.querySelector('meta[name="apple-mobile-web-app-status-bar-style"]')?.setAttribute('content', theme === 'light' ? 'default' : 'black-translucent');
  try { localStorage.setItem(key, theme); } catch { /* The account preference still saves when browser storage is unavailable. */ }
}
