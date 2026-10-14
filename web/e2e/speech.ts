import type { Page } from '@playwright/test';
declare global {
  interface Window { __rhythmSpeech: { runs: { result: (text: string, final?: boolean) => void; end: () => void; error: (code: string) => void; abortCalls: number }[] } }
}
export async function installSpeechMock(page: Page) {
  await page.addInitScript(() => {
    const runs: FakeSpeech[] = [];
    class FakeSpeech {
      lang = ''; continuous = false; interimResults = false; abortCalls = 0;
      onresult: ((event: unknown) => void) | null = null; onend: (() => void) | null = null; onerror: ((event: unknown) => void) | null = null;
      constructor() { runs.push(this); }
      start() {} abort() { this.abortCalls++; this.onend?.(); }
      result(text: string, final = true) { this.onresult?.({ results: [{ isFinal: final, 0: { transcript: text } }] }); }
      end() { this.onend?.(); } error(code: string) { this.onerror?.({ error: code }); this.onend?.(); }
    }
    Object.assign(window, { SpeechRecognition: FakeSpeech, __rhythmSpeech: { runs } });
  });
}
