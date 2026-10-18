import { useEffect, useRef, useState } from 'react';
import { Volume2 } from 'lucide-react';
import { request, errorMessage } from '../../api/client';
import { usePreferences } from '../../app/PreferencesContext';
import { Alert, Button } from '../../components/UI';
import { SpeakButton } from './SpeakButton';
import { setTTSVolume, stopSpeaking } from './tts';

type Voice = { id: string; name: string };
const sample = 'Привет! Я Гена, твой персональный помощник. Напомню о важных делах и помогу организовать твой день';
function VoiceVolume() {
  const { preferences, update, ready } = usePreferences();
  const [volume, setVolume] = useState(preferences.ttsVolume);
  const pending = useRef<number | undefined>(undefined); const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const save = useRef(update); save.current = update;
  function flush() {
    clearTimeout(timer.current);
    if (pending.current !== undefined) { save.current({ ttsVolume: pending.current }); pending.current = undefined; }
  }
  useEffect(() => { if (pending.current === undefined) setVolume(preferences.ttsVolume); }, [preferences.ttsVolume]);
  useEffect(() => () => { clearTimeout(timer.current); if (pending.current !== undefined) save.current({ ttsVolume: pending.current }); }, []);
  return <div className="assistant-voice-volume"><label htmlFor="assistant-voice-volume"><span>Громкость голоса</span><output htmlFor="assistant-voice-volume">{volume}%</output></label><input id="assistant-voice-volume" type="range" min={0} max={100} step={1} value={volume} disabled={!ready} aria-valuetext={volume === 0 ? '0%, звук выключен' : `${volume}%`} onChange={e => { const value = e.target.valueAsNumber; setVolume(value); setTTSVolume(value); pending.current = value; clearTimeout(timer.current); timer.current = setTimeout(flush, 400); }} onBlur={flush}/><p className="muted">{volume === 0 ? 'Звук выключен.' : 'Меняет громкость ответов и прослушивания голоса.'}</p></div>;
}
export function AssistantVoiceSettings() {
  const [voices, setVoices] = useState<Voice[]>([]); const [error, setError] = useState(''); const [loading, setLoading] = useState(true); const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    const controller = new AbortController(); setLoading(true); setError('');
    void request<{ voices: Voice[]; default_voice: string }>('/api/voice/voices', { signal: controller.signal }).then(data => { setVoices(data.voices || []); }).catch(e => { if (!controller.signal.aborted) setError(errorMessage(e)); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => { controller.abort(); stopSpeaking(); };
  }, [attempt]);
  return <section className="settings-card"><h2><Volume2 size={22}/>Голос ассистента</h2><p className="muted">Нажмите на динамик под ответом Гены. Автоматическая озвучка выключена.</p>{loading && <p role="status">Проверяю доступность голоса…</p>}{error && <Alert action={<Button variant="ghost" onClick={() => setAttempt(x => x + 1)}>Повторить</Button>}>{error}</Alert>}{voices.map(voice => <div className="assistant-voice-option" key={voice.id}><span>{voice.name}</span><SpeakButton id={`preview:${voice.id}`} text={sample} voice={voice.id} preview/></div>)}{!loading && !error && !voices.length && <p role="status">Голос Дмитрия сейчас недоступен в выбранном регионе.</p>}<VoiceVolume/></section>;
}
