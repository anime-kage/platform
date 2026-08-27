import { describe, it, expect } from 'vitest';
import { providerLabel, sourceName } from './providers';

describe('providerLabel', () => {
	it('maps known slugs to their real casing', () => {
		expect(providerLabel('vtbe')).toBe('VTBE');
		expect(providerLabel('mp4upload')).toBe('Mp4Upload');
		expect(providerLabel('doodstream')).toBe('DoodStream');
		expect(providerLabel('voe')).toBe('VOE');
		expect(providerLabel('calameo')).toBe('Calaméo');
	});

	it('treats aliases of one service as the same name', () => {
		// dood.li, doods.pro and d0000d.com are all DoodStream.
		expect(providerLabel('dood')).toBe(providerLabel('doodstream'));
	});

	it('normalises case and whitespace', () => {
		expect(providerLabel('  VTBE  ')).toBe('VTBE');
		expect(providerLabel('SibNet')).toBe('Sibnet');
	});

	it('title-cases a slug it has not been taught', () => {
		// An import script may know a provider before this file does; the
		// source button still has to render a name.
		expect(providerLabel('newhost')).toBe('Newhost');
	});

	it('returns empty string for an empty slug', () => {
		expect(providerLabel('')).toBe('');
		expect(providerLabel('   ')).toBe('');
	});
});

describe('sourceName', () => {
	it('prefers the provider column', () => {
		expect(sourceName({ provider: 'vtbe', hostingUrl: 'https://other.com/x' })).toBe('VTBE');
	});

	it('falls back to the host for rows stored before provider was written', () => {
		expect(sourceName({ hostingUrl: 'https://www.sibnet.ru/video123' })).toBe('Sibnet');
		expect(sourceName({ hostingUrl: 'https://mp4upload.com/x' })).toBe('Mp4Upload');
	});

	it('strips a www prefix before matching', () => {
		expect(sourceName({ hostingUrl: 'https://www.vtbe.to/e/abc' })).toBe('VTBE');
	});

	it('never renders a blank button', () => {
		expect(sourceName({}, 0)).toBe('Sursa 1');
		expect(sourceName({}, 2)).toBe('Sursa 3');
		expect(sourceName({ provider: '', hostingUrl: '' }, 1)).toBe('Sursa 2');
	});

	it('falls through to the positional name on a malformed url', () => {
		expect(sourceName({ hostingUrl: 'not a url' }, 0)).toBe('Sursa 1');
	});
});
