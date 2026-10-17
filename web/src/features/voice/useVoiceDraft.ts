import { useEffect, useRef, useState } from 'react';
import { MediaVoiceController, type MediaVoiceSnapshot } from './mediaController';
import { usePreferences } from '../../app/PreferencesContext';
import { learnCorrection, rememberCorrection } from './adaptation';
export function useVoiceDraft() {
  const { preferences, update } = usePreferences();
  const context = useRef({ preferences }); context.current = { preferences };
  const ref = useRef<MediaVoiceController | null>(null);
  const [state, setState] = useState<MediaVoiceSnapshot>({ draft: '', phase: 'idle', listening: false, available: !!navigator.mediaDevices?.getUserMedia && typeof MediaRecorder !== 'undefined', error: '' });
  if (!ref.current) ref.current = new MediaVoiceController(setState);
  const controller = ref.current;
  useEffect(() => () => controller.dispose(), [controller]);
  function commit() {
    const pair = controller.correctionCandidate(); if (!pair) return;
    const rule = learnCorrection(pair.original, pair.edited);
    if (rule) update({ speechCorrections: rememberCorrection(context.current.preferences.speechCorrections, rule) });
  }
  return { ...state, setDraft: (value: string) => controller.setDraft(value), editDraft: (value: string) => controller.setDraft(value, true), commit, toggle: () => state.listening ? controller.stop() : controller.start(), stop: () => controller.stop(), currentDraft: () => controller.snapshot().draft, currentSource: (): 'text' | 'voice' => controller.correctionCandidate() ? 'voice' : 'text' };
}
