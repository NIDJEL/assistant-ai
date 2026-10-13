import { test, expect } from './fixtures';
import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { ready, screenshot } from './helpers';
import { installSpeechMock } from './speech';
import type { ChatResponse } from '../src/types/api';
test('voice proof: pause → zero POST; continue → draft; Send → exactly one POST', async ({ page, request }, info) => {
  await installSpeechMock(page); const posts: string[] = []; page.on('request', r => { if (r.url().endsWith('/api/chat') && r.method() === 'POST') posts.push(r.postData()!); });
  await ready(page); await page.getByRole('button', { name: 'Начать диктовку' }).click();
  await page.evaluate(() => { const run = window.__rhythmSpeech.runs[0]; run.result('Поставь задачу проверить', true); run.end(); run.end(); });
  // Wait beyond recognition's restart interval to detect delayed auto-send regressions.
  await page.waitForTimeout(700); expect(posts).toHaveLength(0); await expect(page.getByRole('textbox', { name: 'Сообщение ассистенту' })).toHaveValue('Поставь задачу проверить'); await screenshot(page, 'voice-paused', info);
  await page.evaluate(() => window.__rhythmSpeech.runs[1].result('голосовую диктовку', false)); await expect(page.getByRole('textbox', { name: 'Сообщение ассистенту' })).toHaveValue('Поставь задачу проверить голосовую диктовку');
  await page.evaluate(() => window.__rhythmSpeech.runs[1].result('голосовую диктовку', true)); expect(posts).toHaveLength(0); await screenshot(page, 'voice-continued', info);
  const pending = page.waitForResponse(r => r.url().endsWith('/api/chat') && r.request().method() === 'POST'); await page.getByRole('button', { name: 'Отправить сообщение' }).click(); const response = await (await pending).json() as ChatResponse;
  await expect(page.getByRole('textbox', { name: 'Сообщение ассистенту' })).toHaveValue(''); await page.evaluate(() => { window.__rhythmSpeech.runs[1].end(); window.__rhythmSpeech.runs[1].result('дубликат'); }); await page.waitForTimeout(350); expect(posts).toHaveLength(1); expect(await page.evaluate(() => window.__rhythmSpeech.runs[1].abortCalls)).toBe(1); await screenshot(page, 'voice-sent', info);
  const proof = { recognition: 'Controlled Web Speech API mock; real React UI and Go HTTP', pause_wait_ms: 700, posts_after_pause: 0, continued_draft: 'Поставь задачу проверить голосовую диктовку', posts_before_send: 0, posts_after_send_and_late_onend: posts.length, response_action: response.action, recognition_abort_calls: 1 };
  const directory = process.env.P1_CAPTURE_SCREENSHOTS ? resolve('../docs/screenshots/p1') : info.outputDir; await mkdir(directory, { recursive: true }); await writeFile(resolve(directory, 'voice-proof.json'), JSON.stringify(proof, null, 2) + '\n'); await info.attach('voice-proof', { body: JSON.stringify(proof), contentType: 'application/json' });
  const created = response.events[0]; await page.getByRole('checkbox', { name: `Завершить «${created.title}»`, exact: true }).check(); await expect(page.getByRole('checkbox', { name: `Возобновить «${created.title}»`, exact: true })).toBeChecked(); expect(posts).toHaveLength(1);
  for (const event of response.events) await request.delete(`/api/events/${event.id}`);
});
for (const error of ['network', 'not-allowed']) test(`speech ${error} error does not send`, async ({ page }) => {
  await installSpeechMock(page); let posts = 0; page.on('request', r => { if (r.url().endsWith('/api/chat') && r.method() === 'POST') posts++; }); await ready(page); await page.getByRole('button', { name: 'Начать диктовку' }).click();
  await page.evaluate(code => { window.__rhythmSpeech.runs[0].result('Сохранённый черновик', false); window.__rhythmSpeech.runs[0].error(code); }, error); await page.waitForTimeout(350); expect(posts).toBe(0); await expect(page.getByRole('textbox', { name: 'Сообщение ассистенту' })).toHaveValue('Сохранённый черновик'); await expect(page.getByRole('button', { name: 'Начать диктовку' })).toBeVisible();
});
test('microphone stop is draft only; Enter sends once after keyboard editing', async ({ page, request }) => {
  await installSpeechMock(page); let posts = 0; page.on('request', r => { if (r.url().endsWith('/api/chat') && r.method() === 'POST') posts++; }); await ready(page); await page.getByRole('button', { name: 'Начать диктовку' }).click(); await page.evaluate(() => window.__rhythmSpeech.runs[0].result('Поставь задачу')); await page.getByRole('button', { name: 'Остановить диктовку' }).click(); await page.waitForTimeout(350); expect(posts).toBe(0);
  const input = page.getByRole('textbox', { name: 'Сообщение ассистенту' }); await input.fill('Поставь задачу проверить ручную отправку'); const pending = page.waitForResponse(r => r.url().endsWith('/api/chat') && r.request().method() === 'POST'); await input.press('Enter'); const response = await (await pending).json() as ChatResponse; await expect(input).toHaveValue(''); expect(posts).toBe(1); for (const event of response.events) await request.delete(`/api/events/${event.id}`);
});
