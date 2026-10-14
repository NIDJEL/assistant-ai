import { VoiceController, type Recognition, type VoiceSnapshot } from './controller';
class FakeRecognition implements Recognition {
  lang = ''; continuous = false; interimResults = false; maxAlternatives = 1; phrases: unknown[] = []; onresult: Recognition['onresult'] = null;
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
it('keeps manual corrections across keystrokes and resumed dictation; programmatic drafts reset learning', () => {
  const { voice, runs } = setup(); voice.start(); runs[0].result(['Поставь задачу позвонить Ане', true]);
  voice.setDraft('Поставь задачу позвонить Ан', true); voice.setDraft('Поставь задачу позвонить Анне', true);
  voice.start(); runs[1].result(['завтра', true]);
  expect(voice.correctionCandidate()).toEqual({ original: 'Поставь задачу позвонить Ане завтра', edited: 'Поставь задачу позвонить Анне завтра' });
  voice.setDraft('Новый черновик'); expect(voice.correctionCandidate()).toBeUndefined();
});
it('uses contextual corrections and requests multiple alternatives without sending', () => {
  const changed = vi.fn(); const run = new FakeRecognition();
  const voice = new VoiceController(() => run, changed);
  voice.start(); expect(run.maxAlternatives).toBe(5);
  run.onresult?.({ results: { length: 1, 0: { isFinal: true, length: 2, 0: { transcript: 'Оставь уведомление завтра в 10:00', confidence: .8 }, 1: { transcript: 'Поставь уведомление завтра в 10:00', confidence: .75 } } } });
  expect(voice.snapshot().draft).toBe('Поставь уведомление завтра в 10:00');
});
it('retries unsupported phrase bias once without losing the draft or restarting forever', () => {
  vi.useFakeTimers(); const runs: FakeRecognition[] = [];
  const voice = new VoiceController(() => { const run = new FakeRecognition(); runs.push(run); return run; }, () => {}, { corrections: () => [], phrases: () => ['универ'] });
  const Phrase = class { constructor(public phrase: string, public boost: number) {} };
  vi.stubGlobal('SpeechRecognitionPhrase', Phrase);
  voice.start(); expect(runs[0].phrases).toEqual([new Phrase('универ', 3)]);
  runs[0].result(['Напомни', true]); runs[0].onerror?.({ error: 'phrases-not-supported' }); runs[0].onend?.();
  vi.advanceTimersByTime(150); expect(runs).toHaveLength(2); expect(runs[1].phrases).toEqual([]);
  runs[1].result(['завтра', true]); expect(voice.snapshot().draft).toBe('Напомни завтра');
  runs[1].onerror?.({ error: 'phrases-not-supported' }); vi.runAllTimers(); expect(voice.snapshot().listening).toBe(false);
  vi.unstubAllGlobals();
});
it('does not apply learned corrections twice when an alternative already contains the correction', () => {
  const run = new FakeRecognition();
  const voice = new VoiceController(() => run, () => {}, { corrections: () => [
    { from: 'Ане', to: 'Анне', before: 'позвонить', after: '' },
    { from: 'Анне', to: 'Анны', before: 'позвонить', after: '' },
  ], phrases: () => [] });
  voice.start(); run.onresult?.({ results: { length: 1, 0: { isFinal: true, length: 2, 0: { transcript: 'Напомни позвонить Ане' }, 1: { transcript: 'Напомни позвонить Анне' } } } });
  expect(voice.snapshot().draft).toBe('Напомни позвонить Анне');
});
