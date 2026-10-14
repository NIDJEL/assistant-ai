import { Mic, Send, Square } from 'lucide-react';
import { Button } from '../../components/UI';
import type { useVoiceDraft } from '../voice/useVoiceDraft';
import { stopSpeaking } from '../voice/tts';
type Voice = ReturnType<typeof useVoiceDraft>;
export function Composer({ voice, busy, onSend }: { voice: Voice; busy: boolean; onSend: () => Promise<void> }) {
  const tooLong = [...voice.draft].length > 2000;
  return <form className="composer" onSubmit={e => { e.preventDefault(); void onSend(); }}><div className="composer-box"><textarea id="assistant-draft" aria-label="Сообщение ассистенту" placeholder="Скажите, что нужно сделать…" value={voice.draft} rows={2} disabled={busy} onChange={e => voice.editDraft(e.target.value)} onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); void onSend(); } }}/><button type="button" className={`microphone ${voice.listening ? 'listening' : ''}`} aria-label={voice.listening ? 'Остановить диктовку' : 'Начать диктовку'} aria-pressed={voice.listening} disabled={busy || !voice.available} onClick={() => { stopSpeaking(); voice.toggle(); }}>{voice.listening ? <Square size={23}/> : <Mic size={27}/>}</button><Button type="submit" className="send-button" aria-label="Отправить сообщение" disabled={busy || !voice.draft.trim() || tooLong}><Send size={25}/></Button></div>
    <div className={`composer-status ${voice.error || tooLong ? 'error-text' : ''}`} role="status" aria-live="polite">{tooLong ? 'Сократите сообщение до 2000 символов' : voice.error || (busy ? 'Отправляю…' : voice.listening ? 'Слушаю… Текст отправится только по Send или Enter.' : !voice.available ? 'Диктовка недоступна в этом браузере. Введите текст.' : 'Enter — отправить · Shift+Enter — перенос строки')}</div>
  </form>;
}
