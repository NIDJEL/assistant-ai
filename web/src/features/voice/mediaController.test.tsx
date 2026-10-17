import { useState } from 'react';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { Composer } from '../assistant/Composer';
import { MediaVoiceController, type AudioCapture, type MediaVoiceSnapshot } from './mediaController';

class FakeRecorder {
  mimeType = 'audio/webm;codecs=opus'; state = 'inactive';
  ondataavailable: ((event: { data: Blob }) => void) | null = null;
  onstop: (() => void) | null = null; onerror: (() => void) | null = null;
  start = vi.fn(() => { this.state = 'recording'; });
  stop = vi.fn(() => { this.state = 'inactive'; this.ondataavailable?.({ data: new Blob(['audio']) }); this.onstop?.(); });
}
function setup() {
  const recorder = new FakeRecorder(); const trackStop = vi.fn();
  const stream = { getTracks: () => [{ stop: trackStop }] } as unknown as MediaStream;
  let resolve!: (text: string) => void; let reject!: (error: Error) => void;
  const transcribe = vi.fn((_blob: Blob, _signal: AbortSignal) => new Promise<string>((yes, no) => { resolve = yes; reject = no; }));
  const audio: AudioCapture = { available: () => true, getStream: vi.fn(async () => stream), recorder: () => recorder as unknown as MediaRecorder, transcribe };
  const changed = vi.fn(); const controller = new MediaVoiceController(changed, audio);
  return { controller, recorder, trackStop, audio, changed, resolve: (text: string) => resolve(text), reject: (error: Error) => reject(error), transcribe };
}
it('starts, stops, releases tracks and transcribes into the draft without any chat send', async () => {
  const v = setup(); const fetch = vi.spyOn(window, 'fetch'); v.controller.setDraft('Черновик');
  await v.controller.start(); expect(v.recorder.start).toHaveBeenCalled(); expect(v.controller.snapshot().phase).toBe('recording');
  v.controller.stop(); expect(v.trackStop).toHaveBeenCalled(); expect(v.controller.snapshot().phase).toBe('transcribing');
  expect(v.transcribe).toHaveBeenCalledWith(expect.any(Blob), expect.any(AbortSignal));
  v.resolve('Сдать работу до двадцатого');
  await waitFor(() => expect(v.controller.snapshot()).toMatchObject({ draft: 'Черновик Сдать работу до двадцатого', phase: 'idle' }));
  expect(fetch).not.toHaveBeenCalled();
});
it('errors preserve typed text and retries remain possible', async () => {
  const v = setup(); v.controller.setDraft('Уже введённый текст'); await v.controller.start(); v.controller.stop();
  v.reject(new Error('Распознавание временно недоступно'));
  await waitFor(() => expect(v.controller.snapshot()).toMatchObject({ draft: 'Уже введённый текст', phase: 'error' }));
  v.controller.setDraft('Поправленный текст', true); expect(v.controller.snapshot().phase).toBe('idle');
});
it('editing or disposing cancels in-flight transcription and ignores a late response', async () => {
  const v = setup(); await v.controller.start(); v.controller.stop();
  const signal = v.transcribe.mock.calls[0][1]; v.controller.setDraft('Новая правка', true);
  expect(signal.aborted).toBe(true); v.resolve('Запоздалый transcript'); await Promise.resolve();
  expect(v.controller.snapshot().draft).toBe('Новая правка');
  await v.controller.start(); v.controller.dispose(); expect(v.recorder.state).toBe('inactive'); expect(v.trackStop).toHaveBeenCalled();
});
it('cancels pending microphone permission and closes a stream returned after cancellation', async () => {
  const v = setup(); let grant!: (stream: MediaStream) => void;
  v.audio.getStream = () => new Promise(resolve => { grant = resolve; });
  const started = v.controller.start(); v.controller.stop();
  grant({ getTracks: () => [{ stop: v.trackStop }] } as unknown as MediaStream); await started;
  expect(v.trackStop).toHaveBeenCalled(); expect(v.recorder.start).not.toHaveBeenCalled(); expect(v.controller.snapshot().phase).toBe('idle');
});
it('uses the production MediaRecorder and sends only multipart to the transcription endpoint', async () => {
  const recorder = new FakeRecorder();
  const Constructor = vi.fn(function () { return recorder; });
  Object.assign(Constructor, { isTypeSupported: vi.fn(type => type === 'audio/webm;codecs=opus') });
  vi.stubGlobal('MediaRecorder', Constructor);
  Object.defineProperty(navigator, 'mediaDevices', { configurable: true, value: { getUserMedia: vi.fn(async () => ({ getTracks: () => [{ stop: vi.fn() }] })) } });
  const fetch = vi.spyOn(window, 'fetch').mockResolvedValue(new Response(JSON.stringify({ text: 'Проверить смету' }), { status: 200, headers: { 'Content-Type': 'application/json' } }));
  const controller = new MediaVoiceController(() => {}); await controller.start();
  expect(Constructor).toHaveBeenCalledWith(expect.anything(), { mimeType: 'audio/webm;codecs=opus' });
  controller.stop(); await waitFor(() => expect(controller.snapshot().draft).toBe('Проверить смету'));
  expect(fetch).toHaveBeenCalledTimes(1); expect(fetch.mock.calls[0][0]).toBe('/api/voice/transcribe');
  const init = fetch.mock.calls[0][1]!; expect(init.body).toBeInstanceOf(FormData);
  expect(new Headers(init.headers).get('Content-Type')).toBeNull(); controller.dispose();
});
it('shows recording/transcribing, inserts transcript in the existing input and waits for manual Send', async () => {
  const v = setup(); const onSend = vi.fn(async () => {});
  function Harness() {
    const [snapshot, setSnapshot] = useState<MediaVoiceSnapshot>(v.controller.snapshot());
    const [controller] = useState(() => new MediaVoiceController(setSnapshot, v.audio));
    const voice = { ...snapshot, toggle: () => snapshot.listening ? controller.stop() : void controller.start(), stop: () => controller.stop(), setDraft: (s: string) => controller.setDraft(s), editDraft: (s: string) => controller.setDraft(s, true), commit: () => {}, currentDraft: () => controller.snapshot().draft, currentSource: (): 'voice' | 'text' => 'voice' };
    return <Composer voice={voice} busy={false} onSend={onSend} />;
  }
  render(<Harness />); fireEvent.click(screen.getByRole('button', { name: 'Начать диктовку' }));
  await waitFor(() => expect(v.recorder.start).toHaveBeenCalled()); expect(screen.getByRole('status')).toHaveTextContent('Слушаю');
  fireEvent.click(screen.getByRole('button', { name: 'Остановить диктовку' })); expect(screen.getByRole('status')).toHaveTextContent('Распознаю');
  expect(screen.getByRole('button', { name: 'Отправить сообщение' })).toBeDisabled();
  await act(async () => { v.resolve('Подготовить проект до 20 числа'); });
  expect(screen.getByRole('textbox')).toHaveValue('Подготовить проект до 20 числа'); expect(onSend).not.toHaveBeenCalled();
  fireEvent.change(screen.getByRole('textbox'), { target: { value: 'Подготовить проект до 25 числа' } });
  fireEvent.click(screen.getByRole('button', { name: 'Отправить сообщение' })); expect(onSend).toHaveBeenCalledTimes(1);
});
