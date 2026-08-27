import { describe, it, expect } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import Markdown from './Markdown.svelte';

const html = (source: string) => {
	const { container } = render(Markdown, { props: { source } });
	return container.querySelector('.md') as HTMLElement;
};

// The other half of the security story: lib/markdown.test.ts proves the parser
// never produces markup, and these prove the renderer never reintroduces it.

describe('Markdown — nothing reaches the DOM as markup', () => {
	it('renders a script tag as text, not an element', () => {
		const md = html('<script>alert(1)</script>');
		expect(md.querySelector('script')).toBeNull();
		expect(md.textContent).toContain('<script>alert(1)</script>');
	});

	it('renders an img tag in the body as text', () => {
		const md = html('<img src=x onerror=alert(1)>');
		expect(md.querySelector('img')).toBeNull();
		expect(md.textContent).toContain('<img src=x onerror=alert(1)>');
	});

	it('produces no anchor for a javascript: link', () => {
		const md = html('[click](javascript:alert)');
		expect(md.querySelector('a')).toBeNull();
		expect(md.textContent).toContain('click');
	});

	it('drops a third-party image entirely', () => {
		expect(html('![x](https://evil.com/tracker.gif)').querySelector('img')).toBeNull();
	});
});

describe('Markdown — links', () => {
	it('opens an external link in a new tab, without leaking the referrer', () => {
		const a = html('[AniList](https://anilist.co/anime/1)').querySelector('a')!;
		expect(a).toHaveAttribute('href', 'https://anilist.co/anime/1');
		expect(a).toHaveAttribute('target', '_blank');
		expect(a.getAttribute('rel')).toContain('noopener');
		expect(a.getAttribute('rel')).toContain('noreferrer');
		expect(a.getAttribute('rel')).toContain('nofollow');
	});

	it('keeps an internal link in the same tab', () => {
		const a = html('[91 Days](/anime/91-days)').querySelector('a')!;
		expect(a).toHaveAttribute('href', '/anime/91-days');
		expect(a).not.toHaveAttribute('target');
		expect(a).not.toHaveAttribute('rel');
	});
});

describe('Markdown — blocks', () => {
	it('renders headings as h2–h4, leaving h1 to the post title', () => {
		const md = html('# unu\n\n## doi\n\n### trei');
		expect(md.querySelector('h1')).toBeNull();
		expect(md.querySelector('h2')).toHaveTextContent('unu');
		expect(md.querySelector('h3')).toHaveTextContent('doi');
		expect(md.querySelector('h4')).toHaveTextContent('trei');
	});

	it('renders emphasis, code, lists, quotes and dividers', () => {
		const md = html('**bold** *italic* `cod`\n\n- unu\n- doi\n\n> citat\n\n---');
		expect(md.querySelector('strong')).toHaveTextContent('bold');
		expect(md.querySelector('em')).toHaveTextContent('italic');
		expect(md.querySelector('code')).toHaveTextContent('cod');
		expect(md.querySelectorAll('li')).toHaveLength(2);
		expect(md.querySelector('blockquote')).toHaveTextContent('citat');
		expect(md.querySelector('hr')).not.toBeNull();
	});

	it('renders an upload as a lazy figure resolved against the backend', () => {
		const img = html('![coperta](/uploads/announcements/a.png)').querySelector('img')!;
		expect(img).toHaveAttribute('loading', 'lazy');
		expect(img).toHaveAttribute('alt', 'coperta');
		// mediaUrl prefixes the backend origin — a bare path 404s from :5173.
		expect(img.getAttribute('src')).toMatch(/\/uploads\/announcements\/a\.png$/);
		expect(img.getAttribute('src')).not.toBe('/uploads/announcements/a.png');
	});

	it('renders nothing for an empty body', () => {
		expect(html('').textContent?.trim()).toBe('');
	});
});

describe('Markdown — spoilers in a post body', () => {
	it('hides them behind a reveal button', async () => {
		render(Markdown, { props: { source: 'Moare ||personajul||.' } });
		const button = screen.getByRole('button', { name: /spoiler/i });
		await fireEvent.click(button);
		expect(screen.getByText('personajul')).toBeInTheDocument();
	});
});
