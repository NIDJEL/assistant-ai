/* Browser speech adapter, tested separately from planning and persistence. */
export class VoiceInput {
  constructor({Recognition, onState, onTranscript, onFinal, onError}) {
    this.supported = Boolean(Recognition);
    this.recording = false;
    this.finalText = '';
    this.failed = false;
    this.onError = onError;
    if (!Recognition) return;
    this.recognition = new Recognition();
    this.recognition.lang = 'ru-RU';
    this.recognition.interimResults = true;
    this.recognition.continuous = false;
    this.recognition.onstart = () => {this.recording=true; onState(true)};
    this.recognition.onresult = event => {
      let provisional = '';
      for (let i=event.resultIndex; i<event.results.length; i++) {
        if (event.results[i].isFinal) this.finalText += event.results[i][0].transcript+' ';
        else provisional += event.results[i][0].transcript;
      }
      onTranscript((this.finalText+provisional).trim());
    };
    this.recognition.onerror = event => {this.failed=true; onError(event.error)};
    this.recognition.onend = () => {
      this.recording=false;
      onState(false);
      const text=this.finalText.trim();
      this.finalText='';
      if (!this.failed && text) onFinal(text);
    };
  }
  toggle() {
    if (!this.supported) {this.onError('unsupported'); return}
    if (this.recording) {this.recognition.stop(); return}
    this.failed=false;
    this.finalText='';
    try {this.recognition.start()} catch {this.onError('already-started')}
  }
}

export function speakText(text, synthesis, Utterance) {
  if (!synthesis || !Utterance) return;
  synthesis.cancel();
  const utterance=new Utterance(text.slice(0,1200));
  utterance.lang='ru-RU';
  const voice=synthesis.getVoices().find(v=>v.lang.startsWith('ru'));
  if (voice) utterance.voice=voice;
  utterance.rate=1;
  synthesis.speak(utterance);
}
