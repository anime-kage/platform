import { describe, it, expect, afterEach, vi } from 'vitest';
import { mediaUrl } from './media';

// BASE is read from import.meta.env at module load, so the env-override case
// needs a fresh module rather than a stub applied afterwards.
async function withApiUrl(value: string) {
	vi.resetModules();
	vi.stubEnv('VITE_API_URL', value);
	return (await import('./media')).mediaUrl;
}

afterEach(() => {
	vi.unstubAllEnvs();
	vi.resetModules();
});

describe('mediaUrl', () => {
	it('prefixes a backend-relative upload path', () => {
		// Rendered as-is these resolve against the frontend origin and 404 —
		// the files live on the backend.
		expect(mediaUrl('/uploads/posters/x.png')).toBe('http://localhost:3000/uploads/posters/x.png');
	});

	it('passes absolute urls through untouched', () => {
		// Jikan/MAL posters are already absolute.
		const mal = 'https://cdn.myanimelist.net/images/anime/1/1.jpg';
		expect(mediaUrl(mal)).toBe(mal);
		expect(mediaUrl('http://example.com/x.png')).toBe('http://example.com/x.png');
		expect(mediaUrl('data:image/png;base64,iVBORw0KGgo=')).toBe(
			'data:image/png;base64,iVBORw0KGgo='
		);
	});

	it('returns empty string for missing input', () => {
		// Callers bind straight into src=""; undefined would render "undefined".
		expect(mediaUrl(undefined)).toBe('');
		expect(mediaUrl(null)).toBe('');
		expect(mediaUrl('')).toBe('');
	});

	it('honours VITE_API_URL', async () => {
		const url = await withApiUrl('https://api.anime-kage.ro');
		expect(url('/uploads/a.png')).toBe('https://api.anime-kage.ro/uploads/a.png');
	});

	it('strips a trailing slash so the path is not doubled', async () => {
		const url = await withApiUrl('https://api.anime-kage.ro/');
		expect(url('/uploads/a.png')).toBe('https://api.anime-kage.ro/uploads/a.png');
	});
});
