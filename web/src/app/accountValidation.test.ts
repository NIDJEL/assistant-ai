import { nameError, normalizeName, passwordError } from './accountValidation';

it.each(['Анна', 'Анна-Мария', "O'Connor", 'D’Arcy', 'Jose\u0301', '李 明', '𐐀𐐨', 'Мария‑Луиза', 'Я'.repeat(100)])('accepts an international name: %s', name => {
  expect(nameError(` ${name} `)).toBe(''); expect(normalizeName(` ${name} `)).toBe(name);
});
it.each(['', '  ', 'Анна123', 'Анна١', 'Анна１２', '<script>', 'Robert; DROP TABLE users', 'Анна\nМария', 'Анна\n', 'Анна\r\n', '\tАнна', 'Анна\0', 'Анна\u202e', 'Анна\u200b', '\u0301Анна', '-Анна', 'Анна-', 'Анна--Мария', 'Анна  Мария', '🙂', '𐐀'.repeat(101)])('rejects an invalid name: %s', name => {
  expect(nameError(name)).not.toBe('');
});
it('matches server password limits in code points and UTF-8 bytes', () => {
  expect(passwordError('𐐀'.repeat(5))).not.toBe(''); expect(passwordError('𐐀'.repeat(10))).toBe('');
  expect(passwordError('Я'.repeat(512))).toBe(''); expect(passwordError('Я'.repeat(513))).not.toBe('');
});
