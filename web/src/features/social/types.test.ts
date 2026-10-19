import { describe, expect, it } from 'vitest';
import { localSlot } from './types';
describe('planning civil times', () => {
  it('uses the selected IANA zone independently of the device timezone', () => { expect(localSlot('2026-10-21', '15:00', 30, 'Europe/Moscow')).toEqual({ start: '2026-10-21T12:00:00.000Z', end: '2026-10-21T12:30:00.000Z' }); });
  it('handles a DST offset and rejects a nonexistent local hour', () => { expect(localSlot('2026-07-01', '15:00', 60, 'America/New_York').start).toBe('2026-07-01T19:00:00.000Z'); expect(() => localSlot('2026-03-08', '02:30', 30, 'America/New_York')).toThrow(); });
});
