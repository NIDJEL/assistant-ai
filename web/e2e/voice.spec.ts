import { test, expect } from './fixtures';
import { ready } from './helpers';
import type { Page } from '@playwright/test';

// Controlled capture/STT checks UI behavior only. Real microphone/Groq quality
// must be checked separately with a human; this mock is never quality evidence.
async function installCaptureMock(page: Page) {
  await page.addInitScript(() => {
    class Recorder {
      static isTypeSupported(type: string) { return type === 'audio/webm;codecs=opus'; }
      mimeType = 'audio/webm;codecs=opus'; state = 'inactive';
      ondataavailable?: (event: { data: Blob }) => void; onstop?: () => void;
      start() { this.state = 'recording'; }
      stop() { this.state = 'inactive'; this.ondataavailable?.({ data: new Blob(['mock audio'], { type: this.mimeType }) }); this.onstop?.(); }
    }
    Object.defineProperty(window, 'MediaRecorder', { configurable: true, value: Recorder });
    Object.defineProperty(navigator, 'mediaDevices', { configurable: true, value: { getUserMedia: async () => ({ getTracks: () => [{ stop() {} }] }) } });
    Object.defineProperty(window, 'webkitSpeechRecognition', { value: class { constructor() { throw new Error('SpeechRecognition must not start'); } } });
  });
}

test('MediaRecorder inserts transcript; only manual Send creates a task', async ({ page }) => {
  await installCaptureMock(page);
  const text = 'Мне нужно сделать задание по VK Education до 20 числа';
  let finish!: () => void; const release = new Promise<void>(resolve => { finish = resolve; });
  let uploads = 0, chats = 0;
  page.on('request', r => { if (r.url().endsWith('/api/chat') && r.method() === 'POST') chats++; });
  await page.route('**/api/voice/transcribe', async route => {
    uploads++; expect(route.request().headers()['content-type']).toContain('multipart/form-data');
    await release; await route.fulfill({ json: { text } });
  });
  await ready(page);
  const input = page.getByRole('textbox', { name: 'Сообщение ассистенту' });
  await page.getByRole('button', { name: 'Начать диктовку' }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Слушаю' })).toBeVisible();
  await page.getByRole('button', { name: 'Остановить диктовку' }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Распознаю' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Отправить сообщение' })).toBeDisabled(); expect(chats).toBe(0);
  finish(); await expect(input).toHaveValue(text); expect(uploads).toBe(1); expect(chats).toBe(0);
  const sent = page.waitForResponse(r => r.url().endsWith('/api/chat') && r.request().method() === 'POST');
  await page.getByRole('button', { name: 'Отправить сообщение' }).click();
  const result = await (await sent).json();
  expect(result.action).toBe('create'); expect(result.pending).toBe(false); expect(result.events[0]).toMatchObject({ kind: 'task', date: '2026-10-20', time: null });
  await expect(input).toHaveValue(''); expect(chats).toBe(1);
});

test('STT failure preserves draft without starting SpeechRecognition or chat', async ({ page }) => {
  await installCaptureMock(page); let chats = 0;
  page.on('request', r => { if (r.url().endsWith('/api/chat') && r.method() === 'POST') chats++; });
  await page.route('**/api/voice/transcribe', route => route.fulfill({ status: 503, json: { error: { code: 'stt_unavailable', message: 'Распознавание временно недоступно. Черновик сохранён' } } }));
  await ready(page); const input = page.getByRole('textbox', { name: 'Сообщение ассистенту' }); await input.fill('Мой черновик');
  await page.getByRole('button', { name: 'Начать диктовку' }).click(); await page.getByRole('button', { name: 'Остановить диктовку' }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Распознавание временно недоступно' })).toBeVisible(); await expect(input).toHaveValue('Мой черновик'); expect(chats).toBe(0);
  await input.fill('Мой исправленный черновик'); await expect(input).toHaveValue('Мой исправленный черновик');
});

test('permission denial preserves draft', async ({ page }) => {
  await installCaptureMock(page);
  await page.addInitScript(() => { navigator.mediaDevices.getUserMedia = async () => { throw new DOMException('denied', 'NotAllowedError'); }; });
  await ready(page); const input = page.getByRole('textbox', { name: 'Сообщение ассистенту' }); await input.fill('Сохранённый текст');
  await page.getByRole('button', { name: 'Начать диктовку' }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Нет доступа к микрофону' })).toBeVisible(); await expect(input).toHaveValue('Сохранённый текст');
});
