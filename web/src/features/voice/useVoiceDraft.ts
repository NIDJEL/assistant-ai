import { useEffect, useRef, useState } from 'react';
import { browserRecognition, VoiceController, type VoiceSnapshot } from './controller';
import { usePreferences } from '../../app/PreferencesContext';
import { useData } from '../../app/DataContext';
import { learnCorrection, rememberCorrection, recognitionPhrases } from './adaptation';
export function useVoiceDraft() {
  const { preferences, update } = usePreferences(); const { events } = useData();
  const context = useRef({ preferences, events }); context.current = { preferences, events };
  const [state, setState] = useState<VoiceSnapshot>({ draft: '', listening: false, available: !!browserRecognition(), error: '' });
  const ref = useRef<VoiceController | null>(null);
  if (!ref.current) ref.current = new VoiceController(browserRecognition(), setState, {
    corrections: () => context.current.preferences.speechCorrections,
    phrases: () => recognitionPhrases(context.current.preferences.speechCorrections, [...context.current.events].sort((a, b) => b.updated_at.localeCompare(a.updated_at)).slice(0, 50).map(event => event.title)),
  });
  const controller = ref.current;
  useEffect(() => () => controller.dispose(), [controller]);
  function commit() {
    const pair = controller.correctionCandidate(); if (!pair) return;
    const rule = learnCorrection(pair.original, pair.edited);
    if (rule) update({ speechCorrections: rememberCorrection(context.current.preferences.speechCorrections, rule) });
  }
  return { ...state, setDraft: (value: string) => controller.setDraft(value), editDraft: (value: string) => controller.setDraft(value, true), commit, toggle: () => state.listening ? controller.stop() : controller.start(), stop: () => controller.stop(), currentDraft: () => controller.snapshot().draft, currentSource: (): 'text' | 'voice' => controller.correctionCandidate() ? 'voice' : 'text' };
}
