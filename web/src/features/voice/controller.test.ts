import { VoiceController, type Recognition, type VoiceSnapshot } from './controller';
class FakeRecognition implements Recognition {
  lang = ''; continuous = false; interimResults = false; onresult: Recognition['onresult'] = null;
  onend: Recognition['onend'] = null; onerror: Recognition['onerror'] = null;
  start = vi.fn(); abort = vi.fn();
  result(...text: [string, boolean][]) { this.onresult?.({ results: text.map(([transcript, isFinal]) => ({ isFinal, 0: { transcript } })) }); }
}
function setup() {
  const runs: FakeRecognition[] = []; let state: VoiceSnapshot;
  const voice = new VoiceController(() => { const run = new FakeRecognition(); runs.push(run); return run; }, s => { state = s; });
  return { voice, runs, state: () => state! };
}
afterEach(() => vi.useRealTimers());
it('pause, repeated onend and microphone stop preserve draft without any HTTP', () => {
  vi.useFakeTimers(); const fetch = vi.spyOn(window, 'fetch'); const { voice, runs, state } = setup();
  voice.start(); runs[0].result(['Напомни завтра', true]); runs[0].onend?.(); runs[0].onend?.();
  expect(state().draft).toBe('Напомни завтра'); expect(fetch).not.toHaveBeenCalled();
  vi.advanceTimersByTime(150); expect(runs).toHaveLength(2);
  runs[1].result(['в десять', false]); runs[1].result(['в десять позвонить Ивану', true]);
  expect(state().draft).toBe('Напомни завтра в десять позвонить Ивану');
  voice.stop(); runs[1].onend?.(); runs[1].result(['дубликат', true]);
  vi.runAllTimers(); expect(state().draft).toBe('Напомни завтра в десять позвонить Ивану'); expect(fetch).not.toHaveBeenCalled();
});
it('partial/final replacement and keyboard editing never duplicate recognized text', () => {
  const { voice, runs, state } = setup(); voice.setDraft('Напомни'); voice.start();
  runs[0].result(['завт', false]); runs[0].result(['завтра', true], ['позвон', false]);
  runs[0].result(['завтра', true], ['позвонить Ивану', true]);
  expect(state().draft).toBe('Напомни завтра позвонить Ивану');
  voice.setDraft('Напомни завтра позвонить Марии'); voice.start(); runs[1].result(['в 10:00', true]);
  expect(state().draft).toBe('Напомни завтра позвонить Марии в 10:00');
});
it.each(['network', 'not-allowed'])('error %s never sends and retains editable draft', error => {
  const fetch = vi.spyOn(window, 'fetch'); const { voice, runs, state } = setup();
  voice.start(); runs[0].result(['Черновик', false]); runs[0].onerror?.({ error }); runs[0].onend?.();
  expect(state().draft).toBe('Черновик'); expect(state().listening).toBe(false); expect(state().error).not.toBe(''); expect(fetch).not.toHaveBeenCalled();
});
it('unsupported browser keeps text entry usable', () => {
  const voice = new VoiceController(undefined, () => {}); voice.setDraft('Задача'); voice.start();
  expect(voice.snapshot()).toMatchObject({ draft: 'Задача', available: false, listening: false });
});
it('recognition initialization failure preserves draft and returns to idle', () => {
  const voice = new VoiceController(() => { throw new Error('unavailable'); }, () => {});
  voice.setDraft('Проверить почту'); voice.start();
  expect(voice.snapshot()).toMatchObject({ draft: 'Проверить почту', listening: false, error: 'Распознавание речи недоступно. Введите текст.' });
});
