import { chromium } from '@playwright/test';
import { mkdir } from 'node:fs/promises';
const base = process.env.P1_BASE_URL || 'http://127.0.0.1:8084';
const out = new URL('../../docs/screenshots/p1-1/actual/', import.meta.url).pathname.replace(/^\/(\w:)/, '$1');
await mkdir(out, { recursive: true });
const browser = await chromium.launch(); const page = await browser.newPage({ viewport: { width: 1440, height: 900 }, locale: 'ru-RU', timezoneId: 'Europe/Moscow' });
const errors = []; page.on('pageerror', e => errors.push(e.message));
if (process.env.P1_VISUAL_FIXTURES === 'true') {
  if (base !== 'http://127.0.0.1:8084') throw new Error('Visual fixtures require the isolated visual QA server on 8084.');
  const call = async (path, method = 'GET', body) => { const response = await page.request.fetch(base + path, { method, data: body }); if (!response.ok()) throw new Error(await response.text()); return response.json(); };
  const health = await call('/health'); const today = new Intl.DateTimeFormat('en-CA', { timeZone: health.timezone, year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date(health.now));
  const task = (await call(`/api/events?kind=task&start=${today}&limit=200`)).events.find(e => e.date === today);
  if (task) await call(`/api/events/${task.id}`, 'PATCH', { description: 'Проверить показатели, сравнить результаты и записать выводы.', tags: ['Аналитика', 'Работа'], priority: 'high', subtasks: [{ id: 'qa-data', title: 'Проверить исходные показатели', done: true }, { id: 'qa-compare', title: 'Сравнить результаты за неделю', done: false }, { id: 'qa-summary', title: 'Записать выводы для отчёта', done: false }] });
  const meeting = (await call(`/api/events?kind=meeting&start=${today}&limit=200`)).events.find(e => e.date === today);
  if (meeting) await call(`/api/events/${meeting.id}`, 'PATCH', { location: 'Переговорная', description: 'Обсудим результаты проверки, уточним сроки и подготовим следующий шаг.', participants: [{ name: 'Олег Смирнов' }, { name: 'Алёна Никитина' }, { name: 'Елена', email: 'elena@example.org' }], tags: ['Работа'] });
  for (const [title, time] of [['Сделать перерыв и выпить воды', '08:15'], ['Проверить журнал тренировки', '07:45'], ['Обновить рабочий план', '13:15'], ['Разобрать почту по заявкам', '16:20'], ['Прочитать главу руководства', '20:30']]) {
    const found = (await call(`/api/events?kind=reminder&q=${encodeURIComponent(title)}`)).events.some(e => e.title === title && e.date === today);
    if (!found) await call('/api/events', 'POST', { kind: 'reminder', title, date: today, time });
  }
  const tomorrow = new Date(`${today}T12:00:00Z`); tomorrow.setUTCDate(tomorrow.getUTCDate() + 1); const day = tomorrow.toISOString().slice(0,10);
  const title = 'Проверить заметки к релизу';
  if (!(await call(`/api/events?q=${encodeURIComponent(title)}`)).events.some(e => e.title === title)) await call('/api/events','POST',{ kind: 'task', title, date: day, time: '14:00', tags: ['Планирование'] });
}
for (const [name, path] of [['today','/'],['calendar','/calendar'],['tasks','/tasks'],['meetings','/meetings'],['reminders','/reminders'],['archive','/archive'],['integrations','/integrations?service=yandex'],['settings','/settings']]) {
  await page.goto(base + path); await page.locator('h1').waitFor();
  if (name === 'reminders') { const title = page.locator('.reminder-title').first(); if (await title.count()) await title.click(); }
  if (name === 'tasks') { const title = page.locator('.task-title').first(); if (await title.count() && await title.getAttribute('aria-expanded') !== 'true') await title.click(); }
  if (name === 'today') { const response = page.waitForResponse(r => r.url().endsWith('/api/chat') && r.request().method() === 'POST'); await page.getByRole('textbox', { name: 'Сообщение ассистенту' }).fill('Напомни завтра в 11:45 проверить качество данных'); await page.getByRole('button', { name: 'Отправить сообщение' }).click(); await response; await page.waitForFunction(() => document.querySelector('.composer textarea').value === ''); }
  await page.screenshot({ path: `${out}/${name}.png` });
  console.log(name, await page.evaluate(() => ({ width: innerWidth, scroll: document.documentElement.scrollWidth, main: document.querySelector('.main-panel').getBoundingClientRect().width })));
}
await page.goto(base + '/calendar'); await page.locator('h1').waitFor();
await page.getByRole('button', { name: 'Переместить: Календарь', exact: true }).focus(); await page.keyboard.press('Space'); await page.keyboard.press('ArrowDown'); await page.locator('.widget-drag-overlay').waitFor();
await page.screenshot({ path: `${out}/widgets-drag.png` }); await page.keyboard.press('Escape');
await browser.close(); if (errors.length) throw new Error(errors.join('\n'));
