import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { reltime, waitedFor } from './reltime';

// Both functions read Date.now(), so every case is pinned to a fixed clock.
const NOW = new Date('2026-08-25T12:00:00Z');
const ago = (ms: number) => new Date(NOW.getTime() - ms).toISOString();
const MIN = 60_000;
const HOUR = 60 * MIN;
const DAY = 24 * HOUR;

beforeEach(() => {
	vi.useFakeTimers();
	vi.setSystemTime(NOW);
});
afterEach(() => vi.useRealTimers());

describe('reltime', () => {
	it.each([
		[0, 'acum'],
		[30_000, 'acum'],
		[MIN, 'acum 1 min'],
		[20 * MIN, 'acum 20 min'],
		[59 * MIN, 'acum 59 min'],
		[HOUR, 'acum 1 h'],
		[5 * HOUR, 'acum 5 h'],
		[23 * HOUR, 'acum 23 h'],
		[DAY, 'ieri'],
		[2 * DAY, 'acum 2 zile'],
		[29 * DAY, 'acum 29 zile']
	])('renders %i ms ago as "%s"', (delta, expected) => {
		expect(reltime(ago(delta))).toBe(expected);
	});

	it('falls back to a date past 30 days', () => {
		// Locale output varies by ICU build, so assert the shape, not the string.
		const out = reltime(ago(60 * DAY));
		expect(out).not.toMatch(/acum|ieri/);
		expect(out).toMatch(/\d/);
	});

	it('returns empty string for an unparseable date', () => {
		expect(reltime('not a date')).toBe('');
		expect(reltime('')).toBe('');
	});

	it('does not invent a future tense', () => {
		// A clock-skewed row yields a negative delta; it must not crash.
		expect(() => reltime(new Date(NOW.getTime() + HOUR).toISOString())).not.toThrow();
	});
});

describe('waitedFor', () => {
	it.each([
		[0, 'sub o oră'],
		[59 * MIN, 'sub o oră'],
		[HOUR, '1 h'],
		[3 * HOUR, '3 h'],
		[23 * HOUR, '23 h'],
		[DAY, 'o zi'],
		[2 * DAY, '2 zile'],
		[40 * DAY, '40 zile']
	])('renders %i ms of waiting as "%s"', (delta, expected) => {
		expect(waitedFor(ago(delta))).toBe(expected);
	});

	it('returns empty string for an unparseable date', () => {
		expect(waitedFor('nope')).toBe('');
	});
});
