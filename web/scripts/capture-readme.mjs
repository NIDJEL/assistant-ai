import { chromium } from '@playwright/test';
import { mkdir } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const baseURL = process.env.P1_BASE_URL || 'http://localhost:8081';
const directory = fileURLToPath(new URL('../../docs/screenshots/current/', import.meta.url));
await mkdir(directory, { recursive: true });
const browser = await chromium.launch();
try {
  const context = await browser.newContext({ baseURL, viewport: { width: 1440, height: 960 }, locale: 'ru-RU', timezoneId: 'Europe/Moscow', reducedMotion: 'reduce' });
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  async function call(path, data) {
    const response = data === undefined ? await context.request.get(path) : await context.request.post(path, { data });
    if (!response.ok()) throw new Error(`${path}: ${response.status()}`);
    return response.json();
  }
  // Always use a fresh account: screenshots never expose a real user's data.
  const password = `Readme-${crypto.randomUUID()}!`;
  await call('/api/auth/register', { name: 'Анна', email: `readme-${Date.now()}@example.test`, password, password_confirmation: password });
  await call('/api/auth/onboarding', {});
  const health = await call('/health');
  const today = new Intl.DateTimeFormat('en-CA', { timeZone: health.timezone, year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date(health.now));
  const shifted = offset => { const date = new Date(`${today}T12:00:00Z`); date.setUTCDate(date.getUTCDate() + offset); return date.toISOString().slice(0, 10); };
  for (const event of [
    { kind: 'task', title: 'Подготовить презентацию проекта', date: today, priority: 'high', tags: ['Работа'], subtasks: [{ id: 'outline', title: 'Собрать основные идеи', done: true }, { id: 'slides', title: 'Оформить слайды', done: false }] },
    { kind: 'task', title: 'Купить продукты на ужин', date: today, tags: ['Личное'] },
    { kind: 'task', title: 'Отправить документы в университет', date: shifted(1), tags: ['Учёба'] },
    { kind: 'meeting', title: 'Обсудить новый проект', date: today, time: '17:00', duration_minutes: 45, location: 'Переговорная', participants: [{ name: 'Алексей' }, { name: 'Мария' }] },
    { kind: 'meeting', title: 'Английский с преподавателем', date: shifted(2), time: '18:30', duration_minutes: 60 },
    { kind: 'reminder', title: 'Забрать заказ', date: shifted(1), time: '18:00', reminder_minutes: 60 },
    { kind: 'reminder', title: 'Записаться к стоматологу', date: shifted(3), time: '10:00', reminder_minutes: 30 },
  ]) await call('/api/events', { ...event, force: true });
  async function capture(name, path) {
    await page.goto(path);
    await page.locator('h1').waitFor();
    await page.evaluate(async () => { await Promise.allSettled(document.getAnimations().filter(animation => animation.effect?.getComputedTiming().iterations !== Infinity).map(animation => animation.finished)); });
    await page.screenshot({ path: `${directory}/${name}.png` });
    console.log(`Captured ${name}`);
  }
  await page.goto('/');
  await page.getByRole('textbox', { name: 'Сообщение ассистенту' }).fill('У меня завтра пара в 12:50. Поставь уведомление на 12:00, чтобы не опоздать');
  const saved = page.waitForResponse(response => response.url().endsWith('/api/chat') && response.request().method() === 'POST');
  await page.getByRole('button', { name: 'Отправить сообщение' }).click();
  if ((await (await saved).json()).action !== 'create') throw new Error('The example event was not created');
  await capture('assistant', '/');
  await capture('calendar', `/calendar?date=${today}`);
  await capture('tasks', '/tasks');
  await capture('integrations', '/integrations');
  await page.setViewportSize({ width: 390, height: 844 });
  await capture('mobile', '/tasks');
  if (errors.length) throw new Error(errors.join('\n'));
  await context.close();
} finally {
  await browser.close();
}
