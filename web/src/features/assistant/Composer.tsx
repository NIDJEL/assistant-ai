import { useLayoutEffect, useRef } from 'react';
import { ArrowUp, Mic, Square } from 'lucide-react';
import { Button } from '../../components/UI';
import type { useVoiceDraft } from '../voice/useVoiceDraft';
import { stopSpeaking } from '../voice/tts';
type Voice = ReturnType<typeof useVoiceDraft>;
export function Composer({ voice, busy, onSend }: { voice: Voice; busy: boolean; onSend: () => Promise<void> }) {
  const input = useRef<HTMLTextAreaElement>(null);
  function resize() {
    const element = input.current;
    if (!element) return;
    element.style.height = '0px';
    const maximum = parseFloat(getComputedStyle(element).maxHeight) || 160;
    const height = Math.max(24, element.scrollHeight);
    element.style.height = `${Math.min(height, maximum)}px`;
    element.style.overflowY = height > maximum ? 'auto' : 'hidden';
  }
  useLayoutEffect(resize, [voice.draft]);
  useLayoutEffect(() => {
    const element = input.current!;
    let width = element.clientWidth;
    if (typeof ResizeObserver === 'undefined') {
      window.addEventListener('resize', resize); return () => window.removeEventListener('resize', resize);
    }
    const observer = new ResizeObserver(() => {
      if (element.clientWidth !== width) { width = element.clientWidth; resize(); }
    });
    observer.observe(element); return () => observer.disconnect();
  }, []);
  const tooLong = [...voice.draft].length > 2000;
  const capturing = voice.phase === 'recording' || voice.phase === 'transcribing';
  const send = () => { if (!capturing && !busy && !tooLong && voice.draft.trim()) void onSend(); };
  const status = tooLong ? 'Сократите сообщение до 2000 символов' : voice.error || (busy ? 'Отправляю…' : voice.phase === 'transcribing' ? 'Распознаю…' : voice.listening ? 'Слушаю… Нажмите микрофон ещё раз, чтобы закончить.' : '');
  return <form className="composer" onSubmit={e => { e.preventDefault(); send(); }}><div className="composer-box"><textarea ref={input} id="assistant-draft" aria-label="Сообщение ассистенту" placeholder="Спросите Гену…" value={voice.draft} rows={1} disabled={busy} onChange={e => voice.editDraft(e.target.value)} onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); send(); } }}/><button type="button" className={`microphone ${voice.listening ? 'listening' : ''}`} aria-label={voice.listening ? 'Остановить диктовку' : 'Начать диктовку'} aria-pressed={voice.listening} title={!voice.available ? 'Диктовка недоступна в этом браузере' : undefined} disabled={busy || !voice.available || voice.phase === 'transcribing'} onClick={() => { stopSpeaking(); voice.toggle(); }}>{voice.listening ? <Square size={19}/> : <Mic size={21}/>}</button><Button type="submit" className="send-button" aria-label="Отправить сообщение" disabled={busy || capturing || !voice.draft.trim() || tooLong}><ArrowUp size={21}/></Button></div>
    <div className={`composer-status ${voice.error || tooLong ? 'error-text' : ''}`} role="status" aria-live="polite">{status}</div>
  </form>;
}
