import { assistantReplyText } from './replyText';
import { fixture } from '../../test/server';

const event = fixture(1233, 'meeting', 'Обсуждение проекта');
it('hides record numbers before titles while preserving dates, times and response identifiers', () => {
  const response = { reply: 'Изменено: #1233 «Обсуждение проекта» — 23.10.2026 в 19:30.', events: [event] };
  expect(assistantReplyText(response)).toBe('Изменено: «Обсуждение проекта» — 23.10.2026 в 19:30.');
  expect(response.events[0].id).toBe(1233); expect(response.reply).toContain('#1233');
});
it('formats old saved replies even when their event metadata is unavailable', () => {
  expect(assistantReplyText({ reply: 'Создано: #82 «Подготовить отчёт» — завтра.\n#83 «Проверить смету» — в 09:00.' })).toBe('Создано: «Подготовить отчёт» — завтра.\n«Проверить смету» — в 09:00.');
});
it('uses titles in confirmations instead of leaving an empty record reference', () => {
  expect(assistantReplyText({ reply: 'Удалить #1233?', candidates: [event] })).toBe('Удалить «Обсуждение проекта»?');
});
it('keeps numbered titles, unrelated references, URLs and ordinary numbers intact', () => {
  const reply = 'Проект #2026: «Проверить #1233 за 2 часа». https://example.test/#1233 — 23.10.2026, 19:30.';
  expect(assistantReplyText({ reply, events: [event] })).toBe(reply);
});
it('resolves standalone references from conflict and Zoom cards', () => {
  expect(assistantReplyText({ reply: 'Пересечение с #1233. Открыть #88?', conflicts: [event], zoom_meetings: [{ event_id: 88, title: 'Созвон команды', timezone: 'Europe/Moscow', status: 'scheduled' }] })).toBe('Пересечение с «Обсуждение проекта». Открыть «Созвон команды»?');
});
