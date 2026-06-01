import { describe, it, expect } from 'vitest';
import { formatReconnectDeadline } from './format';

describe('formatReconnectDeadline', () => {
  it('formats a valid ISO timestamp as HH:MM', () => {
    const out = formatReconnectDeadline('2024-01-01T09:05:00Z');
    expect(out).toMatch(/\d{1,2}[:.]\d{2}/);
  });

  it('yields "Invalid Date" for an unparseable timestamp', () => {
    // new Date(invalid) does not throw — toLocaleTimeString returns "Invalid Date".
    expect(formatReconnectDeadline('not-a-date')).toBe('Invalid Date');
  });
});
