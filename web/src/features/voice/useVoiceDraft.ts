import { useEffect, useRef, useState } from 'react';
import { browserRecognition, VoiceController, type VoiceSnapshot } from './controller';
export function useVoiceDraft() {
  const [state, setState] = useState<VoiceSnapshot>({ draft: '', listening: false, available: !!browserRecognition(), error: '' });
  const ref = useRef<VoiceController | null>(null);
  if (!ref.current) ref.current = new VoiceController(browserRecognition(), setState);
  const controller = ref.current;
  useEffect(() => () => controller.dispose(), [controller]);
  return { ...state, setDraft: (value: string) => controller.setDraft(value), toggle: () => state.listening ? controller.stop() : controller.start(), stop: () => controller.stop(), currentDraft: () => controller.snapshot().draft };
}
