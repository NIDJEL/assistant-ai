// Keep the same Unicode name rules as internal/auth/name.go.
const namePattern = /^\p{L}[\p{L}\p{M}]*(?:[ '\u2010\u2011\u2019-]\p{L}[\p{L}\p{M}]*)*$/u;
export const normalizeName = (value: string) => value.replace(/^ +| +$/g, '');
export function nameError(value: string) {
  const name = normalizeName(value);
  return [...name].length <= 100 && namePattern.exec(name)?.[0] === name ? '' : 'Укажите имя до 100 символов: только буквы, пробелы, дефисы и апострофы';
}
export function passwordError(value: string) {
  if (new TextEncoder().encode(value).length > 1024) return 'Пароль слишком длинный: используйте до 1024 байт';
  if (/[\p{Cc}\p{Cf}]/u.test(value)) return 'Уберите управляющие и невидимые символы из пароля';
  const compact = [...value].filter(char => !/\p{White_Space}/u.test(char)).map(char => char.toLowerCase());
  if (compact.length < 10) return 'Используйте не меньше 10 символов без учёта пробелов';
  const unique = new Set(compact).size;
  let weak = unique < 4;
  for (let period = 1; period <= compact.length / 2 && !weak; period++) {
    if (compact.length % period === 0 && compact.every((char, index) => char === compact[index % period])) weak = true;
  }
  const candidate = compact.join('');
  weak ||= ['01234567890123456789', '98765432109876543210', 'abcdefghijklmnopqrstuvwxyz', 'zyxwvutsrqponmlkjihgfedcba', 'qwertyuiopasdfghjklzxcvbnm', 'йцукенгшщзхъфывапролджэячсмитьбю'].some(sequence => sequence.includes(candidate));
  weak ||= ['password', 'пароль', 'qwerty', 'йцукен', 'letmein', 'welcome', 'admin', 'administrator', 'iloveyou'].some(common => candidate.startsWith(common) && !/[\p{L}\p{M}]/u.test(candidate.slice(common.length)));
  if (weak) return 'Пароль слишком простой: избегайте повторов, последовательностей и популярных сочетаний';
  const words = value.split(/\p{White_Space}+/u).filter(Boolean);
  const phrase = words.length >= 4 && compact.length >= 16 && unique >= 8 && words.every(word => [...word].length >= 2) && new Set(words.map(word => word.toLowerCase())).size >= 4;
  if (!(/\p{L}/u.test(candidate) && /[^\p{L}\p{M}]/u.test(candidate)) && !phrase) return 'Добавьте к буквам цифру или символ либо используйте длинную фразу из четырёх разных слов';
  return '';
}
