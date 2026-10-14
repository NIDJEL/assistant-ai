import { adaptDictation, learnCorrection, recognitionPhrases, rememberCorrection, speechCorrections } from './adaptation';

it('repairs the planner homophone while preserving ordinary оставь', () => {
  expect(adaptDictation('мне сегодня надо в универ Оставь пожалуйста уведомление в календарь что у меня пара')).toContain('поставь пожалуйста уведомление');
  expect(adaptDictation('Оставь посылку у двери')).toBe('Оставь посылку у двери');
  expect(adaptDictation('Оставь задачу без изменений')).toBe('Оставь задачу без изменений');
  expect(adaptDictation('Поставь задачу оставить посылку')).toBe('Поставь задачу оставить посылку');
});
it('learns a local correction and applies it only in its surrounding context', () => {
  const rule = learnCorrection('Поставь задачу позвонить Ане завтра', 'Поставь задачу позвонить Анне завтра')!;
  expect(rule).toEqual({ from: 'Ане', to: 'Анне', before: 'позвонить', after: 'завтра' });
  expect(adaptDictation('Поставь задачу позвонить Ане завтра', [rule])).toBe('Поставь задачу позвонить Анне завтра');
  expect(adaptDictation('Передать Ане письмо', [rule])).toBe('Передать Ане письмо');
  expect(rememberCorrection([rule], rule)).toHaveLength(1);
});
it.each([
  ['Поставь задачу на завтра', 'Оставь задачу на завтра'],
  ['Напомни завтра в 10:00 позвонить', 'Напомни завтра в 11:00 позвонить'],
  ['Напомни завтра в десять позвонить', 'Напомни завтра в девять позвонить'],
  ['Напомни сегодня позвонить маме', 'Напомни завтра позвонить маме'],
  ['Поставь задачу купить хлеб', 'Поставь задачу купить молоко'],
  ['Поставь задачу позвонить маме', 'Не ставь задачу позвонить маме'],
  ['Поставь задачу позвонить Ане', 'Совсем другое сообщение'],
])('does not learn a change of meaning from %s', (original, edited) => {
  expect(learnCorrection(original, edited)).toBeUndefined();
});
it('validates persisted rules and bounds both the profile and browser hints', () => {
  expect(speechCorrections([null, { from: '10', to: '11', before: 'в', after: '' }, { from: 'ане', to: 'анне' }])).toEqual([]);
  const rules = Array.from({ length: 100 }, () => ({ from: 'ане', to: 'анне', before: 'позвонить', after: '' }));
  expect(speechCorrections(rules)).toHaveLength(40);
  const hints = recognitionPhrases(rules, Array.from({ length: 100 }, (_, i) => `Встреча ${i}`));
  expect(hints).toHaveLength(64); expect(hints[0]).toBe('анне');
});
