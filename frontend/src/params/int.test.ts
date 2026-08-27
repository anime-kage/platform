import { describe, it, expect } from 'vitest';
import { match } from './int';

// This matcher is what lets /liste/[id=int] (real lists) win over
// /liste/[slug] (seeded community lists). If it starts accepting slugs,
// the wrong route loads and the page 404s.
describe('int param matcher', () => {
	it.each(['1', '42', '0', '999999'])('accepts %s', (p) => {
		expect(match(p)).toBe(true);
	});

	it.each([
		'top-50',
		'1a',
		'a1',
		'-1',
		'1.5',
		'',
		' 1',
		'1 ',
		'١٢٣' // arabic-indic digits: \d must not match these
	])('rejects %s', (p) => {
		expect(match(p)).toBe(false);
	});

	it('rejects a multiline string that starts with digits', () => {
		// ^ and $ are line anchors without the m flag on some engines; make
		// sure a newline cannot smuggle a slug past the matcher.
		expect(match('12\nnot-a-number')).toBe(false);
	});
});
