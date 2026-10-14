import { adaptDictation, type SpeechCorrection } from './adaptation';
export interface SpeechAlternative { transcript: string; confidence?: number }
export interface SpeechResult { isFinal: boolean; length?: number; [index: number]: SpeechAlternative }
export interface SpeechResults { length: number; [index: number]: SpeechResult }
export interface Recognition {
  lang: string; continuous: boolean; interimResults: boolean; maxAlternatives?: number; phrases?: unknown[];
  onresult: ((event: { results: SpeechResults }) => void) | null;
  onend: (() => void) | null; onerror: ((event: { error: string }) => void) | null;
  start(): void; abort(): void;
}
export type RecognitionFactory = () => Recognition;
export interface SpeechAdaptation { corrections: () => readonly SpeechCorrection[]; phrases: () => string[] }
export interface VoiceSnapshot { draft: string; listening: boolean; available: boolean; error: string }
export function browserRecognition(): RecognitionFactory | undefined {
  const host = window as unknown as { SpeechRecognition?: new () => Recognition; webkitSpeechRecognition?: new () => Recognition };
  const Constructor = host.SpeechRecognition || host.webkitSpeechRecognition;
  return Constructor ? () => new Constructor() : undefined;
}
const messages: Record<string, string> = {
  'not-allowed': 'Нет доступа к микрофону. Разрешите его в настройках браузера или введите текст.',
  'service-not-allowed': 'Распознавание речи недоступно. Введите текст.',
  'audio-capture': 'Микрофон не найден. Проверьте подключение.',
  network: 'Не удалось распознать речь: проверьте соединение. Черновик сохранён.',
};

// This controller owns only a draft. It has no API client or submit callback.
export class VoiceController {
  private draft = ''; private wanted = false; private error = '';
  private run = 0; private recognition?: Recognition; private restart?: ReturnType<typeof setTimeout>;
  private learningSource?: string; private biasEnabled = true;
  constructor(private factory: RecognitionFactory | undefined, private changed: (state: VoiceSnapshot) => void, private adaptation?: SpeechAdaptation) {}
  snapshot(): VoiceSnapshot { return { draft: this.draft, listening: this.wanted, available: !!this.factory, error: this.error }; }
  private emit() { this.changed(this.snapshot()); }
  setDraft(value: string, userEdit = false) { this.stop(); if (!userEdit) this.learningSource = undefined; this.draft = value; this.error = ''; this.emit(); }
  correctionCandidate() { return this.learningSource ? { original: this.learningSource, edited: this.draft } : undefined; }
  start() {
    if (this.wanted) return;
    if (!this.factory) { this.error = 'Распознавание речи не поддерживается этим браузером. Введите текст.'; this.emit(); return; }
    this.wanted = true; this.error = ''; this.begin(); this.emit();
  }
  private begin() {
    if (!this.wanted || !this.factory) return;
    const id = ++this.run; const base = this.draft.trimEnd(); const learningBase = this.learningSource ?? base; let ended = false;
    let recognition: Recognition;
    try { recognition = this.factory(); }
    catch { this.stop(); this.error = 'Распознавание речи недоступно. Введите текст.'; this.emit(); return; }
    this.recognition = recognition;
    recognition.lang = 'ru-RU'; recognition.continuous = true; recognition.interimResults = true;
    recognition.maxAlternatives = 5;
    const Phrase = (window as unknown as { SpeechRecognitionPhrase?: new (phrase: string, boost: number) => unknown }).SpeechRecognitionPhrase;
    if (this.biasEnabled && 'phrases' in recognition && Phrase && this.adaptation) {
      try { recognition.phrases = this.adaptation.phrases().map(phrase => new Phrase(phrase, 3)); }
      catch { this.biasEnabled = false; }
    }
    recognition.onresult = event => {
      if (id !== this.run || !this.wanted || ended) return;
      const pieces: string[] = [];
      const corrections = this.adaptation?.corrections() || [];
      for (let i = 0; i < event.results.length; i++) {
        pieces.push(event.results[i][0]?.transcript.trim() || '');
      }
      const raw = pieces.filter(Boolean).join(' ');
      const final = event.results[event.results.length - 1]?.isFinal;
      let text = final ? adaptDictation(raw, corrections) : raw;
      // Compare alternatives with the complete corrected utterance. Never feed
      // already corrected text back through the rules (which could cascade).
      if (final) for (let i = 0; i < event.results.length; i++) {
        const result = event.results[i], top = result[0];
        for (let j = 1; j < Math.min(result.length || 1, 5); j++) {
          const alternative = result[j]; if (!alternative || !top) continue;
          const candidate = pieces.map((piece, index) => index === i ? alternative.transcript.trim() : piece).filter(Boolean).join(' ');
          if (candidate.toLocaleLowerCase('ru-RU') === text.toLocaleLowerCase('ru-RU') &&
            (top.confidence === undefined || alternative.confidence === undefined || top.confidence - alternative.confidence <= .15)) { text = candidate; break; }
        }
      }
      this.draft = [base, text].filter(Boolean).join(' ');
      this.learningSource = [learningBase, text].filter(Boolean).join(' '); this.emit();
    };
    recognition.onerror = event => {
      if (id !== this.run || !this.wanted || ended) return;
      if (event.error === 'no-speech') return;
      if (event.error === 'phrases-not-supported' && this.biasEnabled) {
        this.biasEnabled = false; ended = true;
        try { recognition.abort(); } catch { /* Retry without optional hints. */ }
        this.restart = setTimeout(() => this.begin(), 150); return;
      }
      this.stop(); this.error = messages[event.error] || 'Диктовка остановлена. Черновик можно отредактировать и отправить вручную.'; this.emit();
    };
    recognition.onend = () => {
      if (id !== this.run || !this.wanted || ended) return;
      ended = true;
      // Browser silence can end one recognition run. Continue in a fresh run,
      // appending to the editable draft; no submission occurs here.
      this.restart = setTimeout(() => this.begin(), 150);
    };
    try { recognition.start(); }
    catch { this.stop(); this.error = 'Не удалось включить микрофон. Введите текст или попробуйте ещё раз.'; this.emit(); }
  }
  stop() {
    this.wanted = false; this.run++; clearTimeout(this.restart); this.restart = undefined;
    const recognition = this.recognition; this.recognition = undefined;
    try { recognition?.abort(); } catch { /* Late onend/result callbacks are already invalidated. */ }
    this.emit();
  }
  dispose() { this.stop(); }
}
