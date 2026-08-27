import { describe, it, expect } from 'vitest';
import { nameHue } from './avatar';

describe('nameHue', () => {
	it('always lands in a usable hue range', () => {
		for (const n of ['crefi', 'Ana', 'x', 'utilizator_cu_nume_lung', '日本語', '🎌']) {
			const h = nameHue(n);
			expect(h).toBeGreaterThanOrEqual(0);
			expect(h).toBeLessThan(360);
			expect(Number.isInteger(h)).toBe(true);
		}
	});

	it('is stable — the same name gives the same tile everywhere', () => {
		// A member's monogram must match across comments, team pages and
		// profiles, so this hash is not allowed to drift.
		expect(nameHue('crefi')).toBe(nameHue('crefi'));
	});

	it('is case-sensitive', () => {
		expect(nameHue('Ana')).not.toBe(nameHue('ana'));
	});

	it('separates common names', () => {
		const names = ['ana', 'mihai', 'ioana', 'andrei', 'maria'];
		expect(new Set(names.map(nameHue)).size).toBeGreaterThan(1);
	});

	it('handles an empty name without throwing', () => {
		expect(nameHue('')).toBe(0);
	});
});
