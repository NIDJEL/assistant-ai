// Keep the same Unicode name rules as internal/auth/name.go.
const namePattern = /^\p{L}[\p{L}\p{M}]*(?:[ '\u2010\u2011\u2019-]\p{L}[\p{L}\p{M}]*)*$/u;
export const normalizeName = (value: string) => value.replace(/^ +| +$/g, '');
export function nameError(value: string) {
  const name = normalizeName(value);
  return [...name].length <= 100 && namePattern.exec(name)?.[0] === name ? '' : 'Укажите имя до 100 символов: только буквы, пробелы, дефисы и апострофы';
}
export function passwordError(value: string) {
  return [...value].length < 10 ? 'Пароль должен содержать от 10 символов' : new TextEncoder().encode(value).length > 1024 ? 'Пароль слишком длинный: используйте до 1024 байт' : '';
}
