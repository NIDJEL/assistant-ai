import { chromium } from '@playwright/test';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';

const baseURL = process.env.P1_BASE_URL || 'http://localhost:8081';
if (!['localhost', '127.0.0.1', '[::1]'].includes(new URL(baseURL).hostname) && process.env.ALLOW_SCREENSHOT_REMOTE !== '1') throw new Error('Use a disposable local installation. Remote account creation requires ALLOW_SCREENSHOT_REMOTE=1.');
const directory = fileURLToPath(new URL('../../docs/screenshots/current/', import.meta.url));
await mkdir(directory, { recursive: true });
const legalSource = await readFile(new URL('../src/pages/Legal/documents.tsx', import.meta.url), 'utf8');
const legalVersion = /export const LEGAL_VERSION = '([^']+)'/.exec(legalSource)?.[1];
if (!legalVersion) throw new Error('Current legal version not found');
const acceptance = { terms_accepted: true, personal_data_consent: true, legal_version: legalVersion };
const browser = await chromium.launch();
const screenshots = [];
try {
  const context = await browser.newContext({ baseURL, viewport: { width: 1440, height: 1000 }, locale: 'ru-RU', timezoneId: 'Europe/Moscow', reducedMotion: 'reduce', serviceWorkers: 'block' });
  const page = await context.newPage(); const errors = []; const peers = [];
  page.on('pageerror', error => errors.push(error.message));
  async function call(path, data, method = data === undefined ? 'GET' : 'POST', session = context) {
    if (path === '/api/social/operations') data = { ...data, request_id: crypto.randomUUID() };
    const response = await session.request.fetch(path, { method, data });
    if (!response.ok()) throw new Error(`${method} ${path}: ${response.status()} ${await response.text()}`);
    return response.status() === 204 ? undefined : response.json();
  }
  async function settle() {
    await page.evaluate(async () => { await document.fonts.ready; await Promise.allSettled(document.getAnimations().filter(a => a.effect?.getComputedTiming().iterations !== Infinity).map(a => a.finished)); });
    await page.waitForTimeout(100);
  }
  async function shot(name, path, waitFor = '.main-panel > .page') {
    if (path) await page.goto(path);
    await page.locator(waitFor).waitFor(); await settle();
    const file = `${directory}/${name}.png`; await page.screenshot({ path: file });
    const size = page.viewportSize(); const hash = createHash('sha256').update(await readFile(file)).digest('hex');
    screenshots.push({ file: `${name}.png`, route: new URL(page.url()).pathname, viewport: size, sha256: hash });
    console.log(`Captured ${name}`);
  }
  await shot('auth', '/auth', '.auth-card');
  const password = `Demo-Review-${crypto.randomUUID()}!`; const stamp = Date.now();
  const { user } = await call('/api/auth/register', { ...acceptance, name: 'Анна Морозова', email: `anna-demo-${stamp}@example.test`, password, password_confirmation: password });
  await shot('onboarding', '/onboarding', '.gena-onboarding');
  await call('/api/auth/onboarding', {});
  const health = await call('/health');
  const today = new Intl.DateTimeFormat('en-CA', { timeZone: health.timezone, year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date(health.now));
  const shifted = offset => { const value = new Date(`${today}T12:00:00Z`); value.setUTCDate(value.getUTCDate() + offset); return value.toISOString().slice(0, 10); };
  for (const event of [
    { kind: 'task', title: 'Подготовить демонстрацию Гены', date: shifted(1), priority: 'high', tags: ['Проект'], subtasks: [{ id: 'outline', title: 'Собрать сценарий показа', done: true }, { id: 'slides', title: 'Проверить голосовой ввод', done: false }] },
    { kind: 'task', title: 'Проверить документацию проекта', date: shifted(1), priority: 'high', tags: ['Работа'] },
    { kind: 'task', title: 'Отправить заявку организаторам', date: shifted(2), tags: ['Проект'] },
    { kind: 'task', title: 'Купить продукты к ужину', date: today, tags: ['Личное'] },
    { kind: 'task', title: 'Прочитать обратную связь команды', date: shifted(3), priority: 'low' },
    { kind: 'meeting', title: 'Обсуждение проекта с командой', date: shifted(1), time: '14:00', duration_minutes: 45, location: 'Переговорная', participants: [{ name: 'Алексей Волков' }, { name: 'Мария Орлова' }] },
    { kind: 'meeting', title: 'Английский с преподавателем', date: shifted(2), time: '18:30', duration_minutes: 60 },
    { kind: 'meeting', title: 'Презентация для организаторов', date: shifted(4), time: '12:00', duration_minutes: 30 },
    { kind: 'reminder', title: 'Забрать заказ', date: shifted(1), time: '18:00', reminder_minutes: 60 },
    { kind: 'reminder', title: 'Позвонить в учебный отдел', date: shifted(2), time: '10:00', reminder_minutes: 15 },
    { kind: 'reminder', title: 'Записаться к стоматологу', date: shifted(3), time: '11:30', reminder_minutes: 30 },
  ]) await call('/api/events', { ...event, force: true });
  const completed = await call('/api/events', { kind: 'task', title: 'Собрать прототип ассистента', date: today });
  await call(`/api/events/${completed.id}`, { status: 'done' }, 'PATCH');
  await call(`/api/preferences/${user.id}`, { name: user.name, theme: 'dark', motion: 'reduced', widgets: true, revision: 1 }, 'PUT');
  await page.goto('/');
  await page.getByRole('textbox', { name: 'Сообщение ассистенту' }).fill('Что у меня на завтра?');
  const reply = page.waitForResponse(r => new URL(r.url()).pathname === '/api/chat' && r.request().method() === 'POST');
  await page.getByRole('button', { name: 'Отправить сообщение' }).click();
  const response = await reply; if (!response.ok() || !(await response.json()).events?.length) throw new Error('Real demo chat did not list the seeded events');
  await page.locator('.assistant-reply').first().waitFor(); await shot('assistant', '/');
  await page.getByRole('button', { name: 'Быстрые команды', exact: true }).click(); await shot('quick-commands', undefined, '.composer-commands');
  for (const [name, route] of [['calendar', `/calendar?date=${shifted(1)}`], ['tasks', '/tasks'], ['meetings', '/meetings'], ['reminders', '/reminders'], ['integrations', '/integrations'], ['settings', '/settings'], ['archive', '/archive']]) await shot(name, route);
  await page.goto('/settings'); await page.getByRole('radio', { name: 'Светлая', exact: true }).check();
  await page.waitForFunction(() => document.documentElement.dataset.theme === 'light');
  await shot('assistant-light', '/'); await shot('calendar-light', `/calendar?date=${shifted(1)}`);
  await page.goto('/settings'); await page.getByRole('radio', { name: 'Тёмная', exact: true }).check();
  await page.setViewportSize({ width: 390, height: 844 });
  await shot('mobile', '/tasks'); await shot('mobile-calendar', `/calendar?date=${shifted(1)}`);
  await page.goto('/integrations'); await page.locator('.integration-card').first().getByRole('button', { name: 'Настроить', exact: true }).click();
  await shot('mobile-integration', undefined, 'dialog[open]');
  await page.goto('/'); await page.getByRole('button', { name: 'Быстрые команды', exact: true }).click(); await shot('mobile-commands', undefined, '.composer-commands');
  await page.emulateMedia({ colorScheme: 'dark' }); await shot('error-404', '/errors/404', '.error-screen'); await shot('error-503', '/errors/503', '.error-screen');
  for (const [index, name] of ['Алексей Волков', 'Мария Орлова'].entries()) {
    const peer = await browser.newContext({ baseURL, serviceWorkers: 'block' }); peers.push(peer);
    const peerUser = (await call('/api/auth/register', { ...acceptance, name, email: `peer-${index}-${stamp}@example.test`, password, password_confirmation: password }, 'POST', peer)).user;
    await call('/api/auth/onboarding', {}, 'POST', peer);
    const code = (await call('/api/social/people', undefined, 'GET', peer)).profile.code;
    await call('/api/social/operations', { action: 'request', code });
    const incoming = (await call('/api/social/people', undefined, 'GET', peer)).incoming.find(item => item.person.id === user.id);
    await call('/api/social/operations', { action: 'accept_request', id: incoming.id }, 'POST', peer);
    await call('/api/social/operations', { action: 'tags', user_id: peerUser.id, tags: [index === 0 ? 'Команда' : 'Дизайн'] });
  }
  await page.setViewportSize({ width: 1440, height: 1000 }); await shot('friends', '/friends');
  if (errors.length) throw new Error(errors.join('\n'));
  const commit = execFileSync('git', ['rev-parse', 'HEAD'], { cwd: fileURLToPath(new URL('../../', import.meta.url)), encoding: 'utf8' }).trim();
  const dirty = !!execFileSync('git', ['status', '--porcelain', '--untracked-files=no'], { cwd: fileURLToPath(new URL('../../', import.meta.url)), encoding: 'utf8' }).trim();
  await writeFile(`${directory}/manifest.json`, JSON.stringify({ captured_at: new Date().toISOString(), source_commit: commit, working_tree_changes: dirty, base_url: baseURL, source: 'real local Go/PostgreSQL application; synthetic accounts and records; no mocked API responses', nlp_provider: health.nlp_provider, screenshots }, null, 2) + '\n');
  for (const peer of peers) await peer.close(); await context.close();
} finally { await browser.close(); }
