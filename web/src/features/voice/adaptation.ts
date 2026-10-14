export interface SpeechCorrection { from: string; to: string; before: string; after: string }
type Token = { key: string; start: number; end: number };
const key = (text: string) => text.toLocaleLowerCase('ru-RU').replaceAll('ё', 'е');
const tokens = (text: string): Token[] => [...text.matchAll(/[\p{L}\p{N}]+/gu)].map(m => ({ key: key(m[0]), start: m.index!, end: m.index! + m[0].length }));
// Actions, negations, numbers and dates are intentional edits, never a reusable
// pronunciation rule. Learn only short, similar lexical corrections.
const protectedWord = /^(?:\d|не$|нет$|да$|отмен|стоп$|удал|убер|убир|добав|созд|постав|остав|став|запи|перен|сдви|измен|заверш|выполн|напом|забуд|покаж|най|сегод|завтр|послезавтр|вчер|позавчер|понедель|вторник|сред[ау]|четверг|пятниц|суббот|воскрес|январ|феврал|март|апрел|ма[йя]|июн|июл|август|сентябр|октябр|ноябр|декабр|нул|ноль|один|одн|дв[аеи]|три|четыр|пят|шест|сем|восем|девят|десят|двадц|сорок|пол|час|минут|утр|вечер|дня$|ноч|недел)/u;
function distance(a: string, b: string) {
  let row = Array.from({ length: b.length + 1 }, (_, i) => i);
  for (let i = 1; i <= a.length; i++) {
    const next = [i];
    for (let j = 1; j <= b.length; j++) next[j] = Math.min(next[j - 1] + 1, row[j] + 1, row[j - 1] + Number(a[i - 1] !== b[j - 1]));
    row = next;
  }
  return row[b.length];
}
function safePair(from: string, to: string) {
  const a = tokens(from), b = tokens(to);
  if (!a.length || !b.length || a.length > 3 || b.length > 3 || from.length > 60 || to.length > 60) return false;
  if ([...a, ...b].some(t => protectedWord.test(t.key))) return false;
  const left = a.map(t => t.key).join(''), right = b.map(t => t.key).join('');
  return left.length >= 2 && right.length >= 2 && left !== right && distance(left, right) <= Math.max(1, Math.floor(Math.max(left.length, right.length) * .35));
}
export function speechCorrections(value: unknown): SpeechCorrection[] {
  if (!Array.isArray(value)) return [];
  return value.filter((v): v is SpeechCorrection => !!v && typeof v === 'object' &&
    ['from', 'to', 'before', 'after'].every(field => typeof v[field] === 'string' && v[field].length <= 60) &&
    !!(v.before || v.after) && safePair(v.from, v.to)).slice(-40);
}
export function learnCorrection(original: string, edited: string): SpeechCorrection | undefined {
  const a = tokens(original), b = tokens(edited);
  let start = 0, end = 0;
  while (start < Math.min(a.length, b.length) && a[start].key === b[start].key) start++;
  while (end < Math.min(a.length, b.length) - start && a[a.length - end - 1].key === b[b.length - end - 1].key) end++;
  const from = a.slice(start, a.length - end), to = b.slice(start, b.length - end);
  if (start + end < 2 || !from.length || !to.length) return;
  const source = original.slice(from[0].start, from.at(-1)!.end), target = edited.slice(to[0].start, to.at(-1)!.end);
  if (!safePair(source, target)) return;
  return { from: source, to: target, before: a[start - 1]?.key || '', after: end ? a[a.length - end].key : '' };
}
export function rememberCorrection(current: SpeechCorrection[], rule: SpeechCorrection): SpeechCorrection[] {
  return speechCorrections([...current.filter(c => !(key(c.from) === key(rule.from) && c.before === rule.before && c.after === rule.after)), rule]);
}
export function adaptDictation(text: string, corrections: readonly SpeechCorrection[] = []): string {
  // Apply at most one learned rule at each original token, never cascading rules.
  const words = tokens(text); let output = '', cursor = 0;
  for (let i = 0; i < words.length; i++) {
    const rule = [...corrections].reverse().find(c => {
      const source = tokens(c.from);
      return source.every((t, j) => words[i + j]?.key === t.key) &&
        (!c.before || words[i - 1]?.key === c.before) && (!c.after || words[i + source.length]?.key === c.after);
    });
    if (!rule) continue;
    const last = i + tokens(rule.from).length - 1;
    output += text.slice(cursor, words[i].start) + rule.to; cursor = words[last].end; i = last;
  }
  text = output + text.slice(cursor);
  return text.replace(/(^|[^\p{L}\p{N}_])оставь(?=[\s,]+(?:(?:пожалуйста|мне)[\s,]+)*(?:напоминание|уведомление|задачу|событие|встречу|в\s+(?:(?:мой|наш)\s+)?календарь)(?![\p{L}\p{N}_]))/giu, (match: string, boundary: string, offset: number) =>
    /(?:без\s+изменений|как\s+есть|на\s+месте|в\s+покое)(?![\p{L}\p{N}_])/iu.test(text.slice(offset + match.length)) ? match : `${boundary}поставь`);
}
export const plannerPhrases = ['поставь напоминание', 'добавь задачу', 'запиши в календарь', 'напомни', 'перенеси встречу', 'сегодня', 'завтра'];
export function recognitionPhrases(corrections: readonly SpeechCorrection[], titles: string[]): string[] {
  return [...new Set([...corrections.map(c => c.to), ...plannerPhrases, ...titles.filter(t => t.length <= 80)])].slice(0, 64);
}
