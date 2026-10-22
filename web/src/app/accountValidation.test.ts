import { nameError, normalizeName, passwordError } from './accountValidation';

it.each(['Анна', 'Анна-Мария', "O'Connor", 'D’Arcy', 'Jose\u0301', '李 明', '𐐀𐐨', 'Мария‑Луиза', 'Я'.repeat(100)])('accepts an international name: %s', name => {
  expect(nameError(` ${name} `)).toBe(''); expect(normalizeName(` ${name} `)).toBe(name);
});
it.each(['', '  ', 'Анна123', 'Анна١', 'Анна１２', '<script>', 'Robert; DROP TABLE users', 'Анна\nМария', 'Анна\n', 'Анна\r\n', '\tАнна', 'Анна\0', 'Анна\u202e', 'Анна\u200b', '\u0301Анна', '-Анна', 'Анна-', 'Анна--Мария', 'Анна  Мария', '🙂', '𐐀'.repeat(101)])('rejects an invalid name: %s', name => {
  expect(nameError(name)).not.toBe('');
});
it('matches server password limits in code points and UTF-8 bytes', () => {
  expect(passwordError('𐐀𐐁𐐂𐐃7!')).not.toBe(''); expect(passwordError('𐐀𐐁𐐂𐐃𐐄𐐅𐐆𐐇7!')).toBe('');
  expect(passwordError('Ab!7' + 'Я'.repeat(510))).toBe(''); expect(passwordError('Ab!7' + 'Я'.repeat(511))).not.toBe('');
});
it.each(['River!coast7', 'test password 12345', 'ёжик7СинийЛес!', 'my new Gena password', 'Лампа светит возле окна', '  River!coast7  ', '🪐Rivers7🌊moon'])('accepts a usable password or phrase: %s', value => {
  expect(passwordError(value)).toBe('');
});
it.each(['', ' '.repeat(10), '\u00a0'.repeat(20), 'abcdefgh9' + ' '.repeat(50), 'A'.repeat(20), 'abcabcabcabc', '1234567890', 'abcdefghij', 'Qwertyuiop', 'password12345', 'Password123!', 'пароль12345', 'welcome123456', '5816094732', 'riverforest', 'word word word word', 'Valid!pass7\u200b', 'Valid!pass7\n', 'x'.repeat(1025)])('rejects a weak or hidden-character password: %s', value => {
  expect(passwordError(value)).not.toBe('');
});
