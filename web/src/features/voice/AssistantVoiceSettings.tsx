import { useEffect, useState } from 'react';
import { Volume2 } from 'lucide-react';
import { request, errorMessage } from '../../api/client';
import { usePreferences } from '../../app/PreferencesContext';
import { Alert, Button } from '../../components/UI';
import { SpeakButton } from './SpeakButton';
import { stopSpeaking } from './tts';

type Voice = { id: string; name: string };
const sample = 'Привет! Я Гена, твой персональный помощник. Напомню о важных делах и помогу организовать твой день';
export function AssistantVoiceSettings() {
  const { preferences, update } = usePreferences();
  const [voices, setVoices] = useState<Voice[]>([]); const [error, setError] = useState(''); const [loading, setLoading] = useState(true); const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    const controller = new AbortController(); setLoading(true); setError('');
    void request<{ voices: Voice[]; default_voice: string }>('/api/voice/voices', { signal: controller.signal }).then(data => { setVoices(data.voices || []); }).catch(e => { if (!controller.signal.aborted) setError(errorMessage(e)); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => { controller.abort(); stopSpeaking(); };
  }, [attempt]);
  return <section className="settings-card"><h2><Volume2 size={22}/>Голос ассистента</h2><p className="muted">Нажмите на динамик под ответом Гены. Автоматическая озвучка выключена.</p>{loading && <p role="status">Проверяю доступные голоса…</p>}{error && <Alert action={<Button variant="ghost" onClick={() => setAttempt(x => x + 1)}>Повторить</Button>}>{error}</Alert>}<div role="radiogroup" aria-label="Голос ассистента">{voices.map(voice => <div className="assistant-voice-option" key={voice.id}><label><input type="radio" name="assistant-voice" value={voice.id} checked={preferences.ttsVoice === voice.id} onChange={() => { stopSpeaking(); update({ ttsVoice: voice.id }); }}/>{voice.name}{voice.id === 'ru-RU-DmitryNeural' ? ' (по умолчанию)' : ''}</label><SpeakButton id={`preview:${voice.id}`} text={sample} voice={voice.id} preview/></div>)}</div>{!loading && !error && !voices.length && <p role="status">Русские голоса сейчас недоступны в выбранном регионе.</p>}{!loading && voices.length > 0 && !voices.some(v => v.id === preferences.ttsVoice) && <p role="alert">Выбранный голос сейчас недоступен. Выберите другой.</p>}</section>;
}
