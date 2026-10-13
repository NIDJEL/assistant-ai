export const ttsAvailable = () => typeof window !== 'undefined' && 'speechSynthesis' in window && 'SpeechSynthesisUtterance' in window;
export function stopSpeaking() { if (ttsAvailable()) window.speechSynthesis.cancel(); }
export function speak(text: string) {
  if (!ttsAvailable()) return;
  stopSpeaking();
  const utterance = new SpeechSynthesisUtterance(text.slice(0, 700));
  utterance.lang = 'ru-RU'; utterance.rate = 1;
  const voice = window.speechSynthesis.getVoices().find(v => v.lang.toLowerCase().startsWith('ru'));
  if (voice) utterance.voice = voice;
  window.speechSynthesis.speak(utterance);
}
