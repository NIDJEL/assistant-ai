import { LoaderCircle, Square, Volume2 } from 'lucide-react';
import { Button, IconButton } from '../../components/UI';
import { speak, useTTS } from './tts';

export function SpeakButton({ id, text, voice, preview = false }: { id: string; text: string; voice: string; preview?: boolean }) {
  const state = useTTS(); const active = state.id === id;
  const loading = active && state.phase === 'loading'; const playing = active && state.phase === 'playing';
  const label = loading ? 'Отменить загрузку голоса' : playing ? 'Остановить озвучивание' : preview ? 'Прослушать' : 'Озвучить ответ';
  const Icon = loading ? LoaderCircle : playing ? Square : Volume2;
  const className = `tts-button${loading ? ' tts-loading' : ''}`;
  return <>{preview ? <Button variant="soft" className={className} aria-label={label} aria-pressed={loading || playing} onClick={() => void speak(text, voice, id)}><Icon size={18}/>{loading ? 'Отменить' : playing ? 'Остановить' : 'Прослушать'}</Button> : <IconButton className={className} icon={Icon} label={label} aria-pressed={loading || playing} onClick={() => void speak(text, voice, id)}/>}{loading && <small role="status">Загружаю голос…</small>}{active && state.error && <small role="alert">{state.error}</small>}</>;
}
