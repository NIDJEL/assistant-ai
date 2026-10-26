import { useEffect, useId, useLayoutEffect, useRef, useState } from 'react';
import { ArrowUp, Bell, CalendarPlus, ListTodo, Mic, Plus, Square } from 'lucide-react';
import { Button } from '../../components/UI';
import type { useVoiceDraft } from '../voice/useVoiceDraft';
import { stopSpeaking } from '../voice/tts';
type Voice = ReturnType<typeof useVoiceDraft>;
const commands = [
  { label: 'Напомнить', icon: Bell, prefix: 'Напомни мне ', template: 'Напомни мне [о чём] [когда]' },
  { label: 'Добавить в календарь', icon: CalendarPlus, prefix: 'Добавь в календарь ', template: 'Добавь в календарь [событие] [дата и время]' },
  { label: 'Создать задачу', icon: ListTodo, prefix: 'Создай задачу ', template: 'Создай задачу [что нужно сделать]' },
];
export function Composer({ voice, busy, onSend }: { voice: Voice; busy: boolean; onSend: () => Promise<void> }) {
  const input = useRef<HTMLTextAreaElement>(null);
  const form = useRef<HTMLFormElement>(null); const trigger = useRef<HTMLButtonElement>(null);
  const [commandsOpen, setCommandsOpen] = useState(false); const menuId = useId();
  const selection = useRef<[number, number] | null>(null);
  useEffect(() => {
    if (!commandsOpen) return;
    const outside = (event: PointerEvent) => { const target = event.target as Node; if (!trigger.current?.contains(target) && !form.current?.querySelector('.composer-commands')?.contains(target)) setCommandsOpen(false); };
    const escape = (event: KeyboardEvent) => { if (event.key === 'Escape') { event.preventDefault(); setCommandsOpen(false); trigger.current?.focus(); } };
    document.addEventListener('pointerdown', outside); document.addEventListener('keydown', escape);
    return () => { document.removeEventListener('pointerdown', outside); document.removeEventListener('keydown', escape); };
  }, [commandsOpen]);
  useLayoutEffect(() => {
    if (!selection.current || !input.current) return;
    input.current.focus(); input.current.setSelectionRange(...selection.current); selection.current = null;
  }, [voice.draft, commandsOpen]);
  function choose(command: typeof commands[number]) {
    const current = voice.draft.trim();
    const value = current && !commands.some(item => item.template === current) ? command.prefix + current : command.template;
    const placeholder = value.indexOf('[');
    selection.current = placeholder >= 0 ? [placeholder, value.indexOf(']', placeholder) + 1] : [value.length, value.length];
    voice.setDraft(value); setCommandsOpen(false); input.current?.focus();
  }
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
  useEffect(() => { if (busy || capturing) setCommandsOpen(false); }, [busy, capturing]);
  const send = () => { if (!capturing && !busy && !tooLong && voice.draft.trim()) void onSend(); };
  const status = tooLong ? 'Сократите сообщение до 2000 символов' : voice.error || (busy ? 'Отправляю…' : voice.phase === 'transcribing' ? 'Распознаю…' : voice.listening ? 'Слушаю… Нажмите микрофон ещё раз, чтобы закончить.' : '');
  return <form ref={form} className="composer" onSubmit={e => { e.preventDefault(); setCommandsOpen(false); send(); }}>
    {commandsOpen && <div id={menuId} className="composer-commands" role="group" aria-label="Быстрые команды">{commands.map(({ icon: Icon, ...command }) => <button type="button" key={command.label} onClick={() => choose({ ...command, icon: Icon })}><Icon size={17} aria-hidden="true"/><span>{command.label}</span></button>)}</div>}
    <div className="composer-box"><button ref={trigger} type="button" className="composer-plus" aria-label="Быстрые команды" aria-expanded={commandsOpen} aria-controls={commandsOpen ? menuId : undefined} disabled={busy || capturing} onClick={() => setCommandsOpen(open => !open)} onKeyDown={event => { if (event.key === 'ArrowUp' || event.key === 'ArrowDown' || event.key === 'Tab' && commandsOpen && !event.shiftKey) { event.preventDefault(); setCommandsOpen(true); requestAnimationFrame(() => form.current?.querySelector<HTMLButtonElement>('.composer-commands button')?.focus()); } }}><Plus size={21} aria-hidden="true"/></button><textarea ref={input} id="assistant-draft" aria-label="Сообщение ассистенту" placeholder="Спросите Гену…" value={voice.draft} rows={1} disabled={busy} onFocus={() => setCommandsOpen(false)} onChange={e => voice.editDraft(e.target.value)} onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); send(); } }}/><button type="button" className={`microphone ${voice.listening ? 'listening' : ''}`} aria-label={voice.listening ? 'Остановить диктовку' : 'Начать диктовку'} aria-pressed={voice.listening} title={!voice.available ? 'Диктовка недоступна в этом браузере' : undefined} disabled={busy || !voice.available || voice.phase === 'transcribing'} onClick={() => { setCommandsOpen(false); stopSpeaking(); voice.toggle(); }}>{voice.listening ? <Square size={19}/> : <Mic size={21}/>}</button><Button type="submit" className="send-button" aria-label="Отправить сообщение" disabled={busy || capturing || !voice.draft.trim() || tooLong}><ArrowUp size={21}/></Button></div>
    <div className={`composer-status ${voice.error || tooLong ? 'error-text' : ''}`} role="status" aria-live="polite">{status}</div>
  </form>;
}
