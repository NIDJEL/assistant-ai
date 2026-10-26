import { test, expect } from './fixtures';

test.skip(process.env.AZURE_TTS_E2E !== '1', 'Real Azure playback is opt-in; ordinary CI has no Speech credentials.');

test('Azure Dmitry, persisted volume, real MP3 playback, stop and error recovery', async ({ page, request }) => {
  await page.addInitScript(() => {
    const NativeAudio = window.Audio;
    const players: HTMLAudioElement[] = [];
    Object.assign(window, { ttsPlayers: players });
    window.Audio = new Proxy(NativeAudio, { construct(target, args) { const player = Reflect.construct(target, args) as HTMLAudioElement; players.push(player); return player; } });
  });
  const audioState = () => page.evaluate(() => ((window as unknown as { ttsPlayers: HTMLAudioElement[] }).ttsPlayers).map(a => ({ paused: a.paused, time: a.currentTime, volume: a.volume, src: a.getAttribute('src') })));
  const user = (await (await request.get('/api/auth/me')).json()).user;
  await request.put(`/api/preferences/${user.id}`, { data: { tts: true, ttsVoice: 'ru-RU-DariyaNeural', ttsVolume: 65, revision: Date.now() } });
  let synthesisRequests = 0;
  page.on('request', req => { if (req.url().endsWith('/api/voice/synthesize')) synthesisRequests++; });
  await page.goto('/settings');
  const settings = page.locator('.settings-card').filter({ has: page.getByRole('heading', { name: 'Голос ассистента' }) });
  await expect(settings.getByText('Гена, мужской')).toBeVisible();
  await expect(settings.getByText(/Светлана|Дарья/)).toHaveCount(0);
  const volume = settings.getByRole('slider', { name: 'Громкость голоса' });
  await expect(volume).toHaveValue('65');
  const response = page.waitForResponse(r => r.url().endsWith('/api/voice/synthesize'));
  await settings.getByRole('button', { name: 'Прослушать' }).click();
  const mp3 = await response; expect(mp3.status()).toBe(200); expect(mp3.request().postDataJSON().voice).toBe('ru-RU-DmitryNeural');
  await expect(settings.getByRole('button', { name: 'Остановить озвучивание' })).toBeVisible();
  await expect.poll(async () => (await audioState()).at(-1)?.time).toBeGreaterThan(0);
  expect((await audioState()).at(-1)?.volume).toBe(0.65);
  await volume.press('Home'); for (let i = 0; i < 30; i++) await volume.press('ArrowRight');
  await expect.poll(async () => (await audioState()).at(-1)?.volume).toBe(0.3);
  await expect.poll(async () => (await (await request.get(`/api/preferences/${user.id}`)).json()).ttsVolume).toBe(30);
  await volume.press('Home'); await expect.poll(async () => (await audioState()).at(-1)?.volume).toBe(0);
  await expect(settings.getByText('Звук выключен.')).toBeVisible();
  await volume.press('Home'); for (let i = 0; i < 30; i++) await volume.press('ArrowRight');
  await settings.getByRole('button', { name: 'Остановить озвучивание' }).click();
  await expect.poll(async () => (await (await request.get(`/api/preferences/${user.id}`)).json()).ttsVolume).toBe(30);
  expect((await (await request.get(`/api/preferences/${user.id}`)).json()).ttsVoice).toBe('ru-RU-DmitryNeural');
  await page.reload(); await expect(volume).toHaveValue('30');
  const before = synthesisRequests;
  await page.goto('/');
  for (const text of ['Создай задачу проверить договор завтра', 'Создай задачу купить цветы завтра']) {
    await page.getByRole('textbox', { name: 'Сообщение ассистенту' }).fill(text);
    const reply = page.waitForResponse(r => r.url().endsWith('/api/chat') && r.request().method() === 'POST');
    await page.getByRole('button', { name: 'Отправить сообщение' }).click(); await reply;
  }
  await expect(page.getByRole('button', { name: 'Озвучить ответ' })).toHaveCount(2);
  expect(synthesisRequests).toBe(before); // Legacy tts:true must never auto-play.
  await page.getByRole('button', { name: 'Озвучить ответ' }).first().click();
  await expect(page.getByRole('button', { name: 'Остановить озвучивание' })).toBeVisible();
  const previousIndex = (await audioState()).length - 1;
  await page.getByRole('button', { name: 'Озвучить ответ' }).click();
  await expect(page.getByRole('button', { name: 'Остановить озвучивание' })).toHaveCount(1);
  expect((await audioState())[previousIndex].paused).toBe(true);
  expect((await audioState()).at(-1)?.volume).toBe(0.3);
  await page.getByRole('button', { name: 'Остановить озвучивание' }).click();
  await page.route('**/api/voice/synthesize', route => route.fulfill({ status: 503, json: { error: { message: 'Озвучивание временно недоступно. Попробуйте позже.' } } }));
  await page.getByRole('button', { name: 'Озвучить ответ' }).first().click();
  await expect(page.getByRole('alert').filter({ hasText: 'Озвучивание временно недоступно' })).toBeVisible();
  await page.unroute('**/api/voice/synthesize');
  await page.getByRole('button', { name: 'Озвучить ответ' }).first().click();
  await expect(page.getByRole('button', { name: 'Остановить озвучивание' })).toBeVisible();
  await page.goto('/settings'); expect((await audioState()).every(a => a.paused)).toBe(true);
});
