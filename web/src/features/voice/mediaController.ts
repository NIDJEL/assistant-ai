import { request } from '../../api/client';

export type VoicePhase = 'idle' | 'recording' | 'transcribing' | 'error';
export interface MediaVoiceSnapshot { draft: string; listening: boolean; available: boolean; error: string; phase: VoicePhase }
export interface AudioCapture {
  available(): boolean;
  getStream(): Promise<MediaStream>;
  recorder(stream: MediaStream): MediaRecorder;
  transcribe(audio: Blob, signal: AbortSignal): Promise<string>;
}
const maxAudioBytes = 15 * 1024 * 1024;
const capture: AudioCapture = {
  available: () => !!navigator.mediaDevices?.getUserMedia && typeof MediaRecorder !== 'undefined',
  getStream: () => navigator.mediaDevices.getUserMedia({ audio: true }),
  recorder: stream => {
    const mimeType = ['audio/webm;codecs=opus', 'audio/webm', 'audio/ogg;codecs=opus', 'audio/mp4'].find(type => MediaRecorder.isTypeSupported(type));
    return new MediaRecorder(stream, mimeType ? { mimeType } : undefined);
  },
  transcribe: async (audio, signal) => {
    const extensions: Record<string, string> = { 'audio/webm': 'webm', 'audio/ogg': 'ogg', 'audio/mp4': 'mp4', 'audio/wav': 'wav', 'audio/mpeg': 'mp3' };
    const extension = extensions[audio.type.split(';')[0]];
    if (!extension) throw new Error('Формат записи не поддерживается. Введите текст.');
    const body = new FormData(); body.append('file', audio, `recording.${extension}`);
    return (await request<{ text: string }>('/api/voice/transcribe', { method: 'POST', body, signal }, 40000)).text;
  },
};

// Owns only a draft and audio capture. No chat submission callback exists here.
// Browser SpeechRecognition is deliberately not used as an automatic fallback.
export class MediaVoiceController {
  private draft = ''; private phase: VoicePhase = 'idle'; private error = '';
  private run = 0; private recording?: MediaRecorder; private stream?: MediaStream;
  private abort?: AbortController; private timer?: ReturnType<typeof setTimeout>;
  private learningSource?: string;
  constructor(private changed: (state: MediaVoiceSnapshot) => void, private audio: AudioCapture = capture) {}
  snapshot(): MediaVoiceSnapshot { return { draft: this.draft, phase: this.phase, listening: this.phase === 'recording', available: this.audio.available(), error: this.error }; }
  private emit() { this.changed(this.snapshot()); }
  correctionCandidate() { return this.learningSource ? { original: this.learningSource, edited: this.draft } : undefined; }
  setDraft(value: string, userEdit = false) { this.cancel(); this.draft = value; if (!userEdit) this.learningSource = undefined; this.error = ''; this.emit(); }
  private releaseStream() { this.stream?.getTracks().forEach(track => track.stop()); this.stream = undefined; clearTimeout(this.timer); }
  private fail(error: unknown) {
    this.cancel(); this.phase = 'error';
    this.error = error instanceof DOMException && error.name === 'NotAllowedError'
      ? 'Нет доступа к микрофону. Разрешите его в настройках браузера или введите текст.'
      : error instanceof Error ? error.message : 'Не удалось распознать речь. Черновик сохранён.';
    this.emit();
  }
  async start() {
    if (this.phase === 'recording' || this.phase === 'transcribing') return;
    if (!this.audio.available()) { this.fail(new Error('Запись недоступна в этом браузере. Введите текст.')); return; }
    const id = ++this.run; this.phase = 'recording'; this.error = ''; this.emit();
    try {
      const stream = await this.audio.getStream();
      if (id !== this.run) { stream.getTracks().forEach(track => track.stop()); return; }
      this.stream = stream;
      const recorder = this.audio.recorder(stream); this.recording = recorder;
      let chunks: Blob[] = []; let size = 0;
      recorder.ondataavailable = event => {
        if (id !== this.run) return;
        size += event.data.size;
        if (size > maxAudioBytes) { chunks = []; this.fail(new Error('Запись должна быть не больше 15 МБ.')); return; }
        if (event.data.size) chunks.push(event.data);
      };
      recorder.onerror = () => { if (id === this.run) { chunks = []; this.fail(new Error('Не удалось записать звук. Черновик сохранён.')); } };
      recorder.onstop = () => {
        if (id !== this.run) { chunks = []; return; }
        this.releaseStream();
        const blob = new Blob(chunks, { type: recorder.mimeType || chunks[0]?.type }); chunks = [];
        this.recording = undefined;
        if (!blob.size) { this.fail(new Error('Запись пуста. Попробуйте ещё раз.')); return; }
        this.phase = 'transcribing'; this.emit();
        const abort = new AbortController(); this.abort = abort;
        void this.audio.transcribe(blob, abort.signal).then(text => {
          if (id !== this.run) return;
          if (!text.trim()) throw new Error('Речь не распознана. Попробуйте ещё раз.');
          this.draft = [this.draft.trimEnd(), text.trim()].filter(Boolean).join(' ');
          this.learningSource = this.draft; this.abort = undefined; this.phase = 'idle'; this.emit();
        }).catch(error => { if (id === this.run) this.fail(error); });
      };
      recorder.start(1000);
      this.timer = setTimeout(() => this.stop(), 120000);
    } catch (error) { if (id === this.run) this.fail(error); }
  }
  stop() {
    if (this.phase !== 'recording') return;
    if (!this.recording) { this.cancel(); this.emit(); return; }
    this.phase = 'transcribing'; this.emit();
    if (this.recording.state !== 'inactive') this.recording.stop();
    this.releaseStream();
  }
  cancel() {
    ++this.run; this.abort?.abort(); this.abort = undefined;
    const recorder = this.recording; this.recording = undefined;
    if (recorder) {
      recorder.ondataavailable = null; recorder.onstop = null; recorder.onerror = null;
      if (recorder.state !== 'inactive') recorder.stop();
    }
    this.releaseStream(); this.phase = 'idle';
  }
  dispose() { this.cancel(); }
}
