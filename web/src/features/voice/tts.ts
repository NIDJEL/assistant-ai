import { useSyncExternalStore } from 'react';
import { ApiError } from '../../api/client';

export const defaultTTSVoice = 'ru-RU-DmitryNeural';
export type TTSState = { id: string; phase: 'idle' | 'loading' | 'playing' | 'error'; error: string };
const idle: TTSState = { id: '', phase: 'idle', error: '' };
let state = idle;
let controller: AbortController | undefined;
let audio: HTMLAudioElement | undefined;
let objectURL: string | undefined;
let generation = 0;
const listeners = new Set<() => void>();
const publish = (next: TTSState) => { state = next; listeners.forEach(fn => fn()); };
export const ttsAvailable = () => typeof window !== 'undefined' && typeof Audio !== 'undefined';
export const ttsSnapshot = () => state;
export function useTTS() { return useSyncExternalStore(fn => { listeners.add(fn); return () => { listeners.delete(fn); }; }, ttsSnapshot, () => idle); }
export function stopSpeaking() {
  generation++;
  controller?.abort(); controller = undefined;
  if (audio) { audio.onended = null; audio.onerror = null; audio.pause(); audio.removeAttribute('src'); audio.load(); audio = undefined; }
  if (objectURL) { URL.revokeObjectURL(objectURL); objectURL = undefined; }
  publish(idle);
}
export async function speak(text: string, voice = defaultTTSVoice, id = 'preview') {
  if (state.id === id && (state.phase === 'loading' || state.phase === 'playing')) { stopSpeaking(); return; }
  stopSpeaking();
  const run = generation;
  const abort = new AbortController(); controller = abort;
  const player = new Audio(); audio = player;
  publish({ id, phase: 'loading', error: '' });
  try {
    const response = await fetch('/api/voice/synthesize', {
      method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', Accept: 'audio/mpeg' },
      body: JSON.stringify({ text, voice }), signal: AbortSignal.any([abort.signal, AbortSignal.timeout(35000)]),
    });
    if (run !== generation) return;
    if (!response.ok) {
      if (response.status === 401) window.dispatchEvent(new Event('rhythm:unauthorized'));
      const body = await response.json().catch(() => ({}));
      throw new ApiError(body.error?.message || 'Не удалось загрузить голос. Попробуйте ещё раз.', body.error?.code || 'tts_error', response.status);
    }
    if (!response.headers.get('Content-Type')?.startsWith('audio/mpeg')) throw new Error('Сервер не вернул аудио');
    const blob = await response.blob();
    if (run !== generation) return;
    if (!blob.size || blob.size > 4 * 1024 * 1024) throw new Error('Некорректное аудио');
    objectURL = URL.createObjectURL(blob); player.src = objectURL;
    player.onended = () => { if (run === generation) stopSpeaking(); };
    player.onerror = () => { if (run === generation) { stopSpeaking(); publish({ id, phase: 'error', error: 'Браузер не смог воспроизвести звук. Попробуйте ещё раз.' }); } };
    await player.play();
    if (run !== generation) { player.pause(); return; }
    publish({ id, phase: 'playing', error: '' });
  } catch (error) {
    if (run !== generation) return;
    stopSpeaking();
    const message = error instanceof ApiError ? error.message : error instanceof DOMException && error.name === 'NotAllowedError' ? 'Браузер заблокировал звук. Нажмите на динамик ещё раз.' : 'Не удалось озвучить ответ. Проверьте соединение и попробуйте ещё раз.';
    publish({ id, phase: 'error', error: message });
  }
}
