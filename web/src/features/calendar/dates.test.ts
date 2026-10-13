import { addDays, addMonths, dayKey, isCivilDay, monthDays, weekStart } from './dates';
it('uses the server timezone near UTC midnight', () => { expect(dayKey('2026-10-12T23:30:00Z', 'Europe/Moscow')).toBe('2026-10-13'); });
it('handles leap day and Monday weeks as civil dates', () => { expect(addDays('2024-02-28', 1)).toBe('2024-02-29'); expect(weekStart('2026-10-13')).toBe('2026-10-12'); expect(monthDays('2026-10-13')).toContain('2026-10-31'); });
it('rejects impossible and zero-year route dates', () => { expect(isCivilDay('2026-02-31')).toBe(false); expect(isCivilDay('0000-01-01')).toBe(false); expect(isCivilDay('0001-01-01')).toBe(true); });
it('renders every Go-supported year without extended-ISO crashes or duplicate keys', () => {
  expect(addMonths('9999-12-31', 1)).toBe('9999-12-01'); expect(addDays('9999-12-31', 1)).toBe('9999-12-31'); expect(addDays('0001-01-01', -1)).toBe('0001-01-01');
  const days = monthDays('9999-12-31'); expect(days).toContain('9999-12-31'); expect(days.every(isCivilDay)).toBe(true); expect(new Set(days).size).toBe(days.length);
});
