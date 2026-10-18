import { act, cleanup, render, screen, fireEvent, waitFor } from '@testing-library/react';
import { SpeakButton } from './SpeakButton';
import { speak, stopSpeaking, ttsSnapshot, setTTSVolume } from './tts';

const players: FakeAudio[] = [];
class FakeAudio {
  volume = 1; src = ''; onended: (() => void) | null = null; onerror: (() => void) | null = null;
  pause = vi.fn(); load = vi.fn(); removeAttribute = vi.fn(); play = vi.fn(async () => {});
  constructor() { players.push(this); }
}
beforeEach(() => {
  setTTSVolume(100); players.length = 0; vi.stubGlobal('Audio', FakeAudio);
  URL.createObjectURL = vi.fn(() => 'blob:tts'); URL.revokeObjectURL = vi.fn();
  vi.stubGlobal('fetch', vi.fn(async () => new Response(new Blob(['MP3']), { headers: { 'Content-Type': 'audio/mpeg' } })));
});
afterEach(() => { cleanup(); act(() => stopSpeaking()); vi.unstubAllGlobals(); });

it('sends only on click, toggles stop, cleans object URLs and switches messages', async () => {
  render(<><SpeakButton id="first" text="Первый" voice="ru-RU-DmitryNeural"/><SpeakButton id="second" text="Второй" voice="ru-RU-DmitryNeural"/></>);
  expect(fetch).not.toHaveBeenCalled();
  fireEvent.click(screen.getAllByRole('button', { name: 'Озвучить ответ' })[0]);
  await screen.findByRole('button', { name: 'Остановить озвучивание' });
  expect(JSON.parse(vi.mocked(fetch).mock.calls[0][1]!.body as string)).toEqual({ text: 'Первый', voice: 'ru-RU-DmitryNeural' });
  fireEvent.click(screen.getByRole('button', { name: 'Озвучить ответ' }));
  await waitFor(() => expect(players[1].play).toHaveBeenCalled());
  expect(players[0].pause).toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: 'Остановить озвучивание' }));
  expect(ttsSnapshot().phase).toBe('idle'); expect(players[1].pause).toHaveBeenCalled(); expect(URL.revokeObjectURL).toHaveBeenCalledTimes(2);
});
it('cancels pending requests and prevents late responses from playing', async () => {
  let finish!: (response: Response) => void;
  vi.mocked(fetch).mockImplementationOnce(() => new Promise(resolve => { finish = resolve; }));
  const pending = speak('Первый', undefined, 'first');
  expect(ttsSnapshot().phase).toBe('loading');
  await speak('Первый', undefined, 'first');
  expect(vi.mocked(fetch).mock.calls[0][1]!.signal!.aborted).toBe(true);
  finish(new Response(new Blob(['MP3']), { headers: { 'Content-Type': 'audio/mpeg' } })); await pending;
  expect(players[0].play).not.toHaveBeenCalled(); expect(ttsSnapshot().phase).toBe('idle');
});
it('shows provider failures and permits a subsequent successful attempt', async () => {
  vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify({ error: { message: 'Лимит озвучивания исчерпан' } }), { status: 429 }));
  render(<SpeakButton id="first" text="Привет" voice="ru-RU-DmitryNeural"/>);
  fireEvent.click(screen.getByRole('button')); expect(await screen.findByRole('alert')).toHaveTextContent('Лимит озвучивания');
  fireEvent.click(screen.getByRole('button')); await screen.findByRole('button', { name: 'Остановить озвучивание' });
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
});
it('releases resources at the end of playback', async () => {
  await speak('Привет'); players[0].onended?.();
  expect(ttsSnapshot().phase).toBe('idle'); expect(URL.revokeObjectURL).toHaveBeenCalled();
});
it('applies saved volume to new audio and adjusts active playback, including mute', async () => {
  setTTSVolume(35); await speak('Привет'); expect(players[0].volume).toBe(0.35);
  setTTSVolume(0); expect(players[0].volume).toBe(0);
  setTTSVolume(200); expect(players[0].volume).toBe(1);
  stopSpeaking(); await speak('Следующий ответ'); expect(players[1].volume).toBe(1);
});
