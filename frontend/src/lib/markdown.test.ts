import { describe, it, expect } from 'vitest';
import {
	parseMarkdown,
	markdownExcerpt,
	parseText,
	splitSpoilers,
	isGifUrl,
	type Block,
	type Inline
} from './markdown';

/** Collapse a token tree back to its visible text — what a reader would see. */
function textOf(nodes: Inline[]): string {
	return nodes
		.map((n) =>
			n.t === 'text' || n.t === 'code' ? n.v : textOf((n as { v: Inline[] }).v)
		)
		.join('');
}

function firstPara(src: string): Inline[] {
	const b = parseMarkdown(src)[0];
	if (!b || b.t !== 'p') throw new Error(`expected a paragraph, got ${b?.t}`);
	return b.v;
}

// ─────────────────────────────────────────────────────────────────────────────
// The security properties. These are the tests that matter: the parser is the
// reason a news post cannot inject markup, and every one of these asserts a
// promise made in the module's own header comment.
// ─────────────────────────────────────────────────────────────────────────────
describe('markdown — no markup can be produced', () => {
	it('renders a script tag as literal text', () => {
		const blocks = parseMarkdown('<script>alert(1)</script>');
		expect(blocks).toEqual([
			{ t: 'p', v: [{ t: 'text', v: '<script>alert(1)</script>' }] }
		]);
	});

	it('emits tokens, never an html string', () => {
		// Every leaf is a tagged token. If someone ever swaps this parser for
		// marked+DOMPurify, this test is what fails first.
		for (const b of parseMarkdown('# Titlu\n\n**bold** and `code`')) {
			expect(b).toHaveProperty('t');
			expect(JSON.stringify(b)).not.toMatch(/<[a-z]/i);
		}
	});

	it('keeps html attributes inert inside emphasis', () => {
		expect(textOf(firstPara('**<img src=x onerror=alert(1)>**'))).toBe(
			'<img src=x onerror=alert(1)>'
		);
	});
});

describe('markdown — link allowlist', () => {
	const linkOf = (src: string) => firstPara(src).find((n) => n.t === 'link');

	it.each([
		['/anime/91-days', 'internal path'],
		['/anunturi/ceva', 'internal path with segments'],
		['https://anilist.co/anime/1', 'https url']
	])('allows %s (%s)', (href) => {
		expect(linkOf(`[text](${href})`)).toMatchObject({ t: 'link', href });
	});

	// No parentheses in these: the link regex captures `([^)\s]+)`, so a `)`
	// inside a URL ends the href early and leaves the rest as stray text. That
	// is covered on its own below; here we assert the scheme allowlist alone.
	it.each([
		['javascript:alert', 'javascript scheme'],
		['http://example.com', 'plaintext http'],
		['//evil.com', 'protocol-relative'],
		['data:text/html,hello', 'data uri'],
		['vbscript:msgbox', 'vbscript scheme'],
		['JavaScript:alert', 'mixed-case javascript scheme'],
		['ftp://example.com/x', 'ftp scheme']
	])('refuses %s (%s) and keeps only the words', (href) => {
		const nodes = firstPara(`[click me](${href})`);
		expect(nodes.find((n) => n.t === 'link')).toBeUndefined();
		expect(textOf(nodes)).toBe('click me');
	});

	it('does not strip-and-render — a refused link leaves no anchor at all', () => {
		// The point of the design: there is no filtered anchor to bypass.
		const nodes = firstPara('[x](javascript:alert(1))');
		expect(nodes.every((n) => n.t !== 'link')).toBe(true);
	});

	it('ends an href at the first closing paren', () => {
		// Documented so nobody "fixes" it by accident: a URL containing `)` is
		// cut short, and the remainder falls through as text. Harmless here
		// because a truncated href still has to clear the allowlist, but it
		// does mean wikipedia-style URLs cannot be linked.
		const nodes = firstPara('[x](https://ex.com/a(b))');
		expect(nodes.find((n) => n.t === 'link')).toMatchObject({
			href: 'https://ex.com/a(b'
		});
	});
});

describe('markdown — image allowlist', () => {
	it('allows our own uploads', () => {
		expect(parseMarkdown('![cover](/uploads/announcements/a.png)')).toEqual([
			{ t: 'img', src: '/uploads/announcements/a.png', alt: 'cover' }
		]);
	});

	it('allows the giphy cdn the picker draws from', () => {
		const blocks = parseMarkdown('![gif](https://media.giphy.com/media/x/giphy.gif)');
		expect(blocks[0]).toMatchObject({ t: 'img' });
	});

	it.each([
		'https://evil.com/tracker.gif',
		'http://media.giphy.com/x.gif',
		'/etc/passwd',
		'//evil.com/x.gif'
	])('drops a third-party image source (%s)', (src) => {
		// A remote <img> reports every reader's IP to whoever owns the host.
		expect(parseMarkdown(`![x](${src})`)).toEqual([]);
	});

	it('is not fooled by a lookalike host', () => {
		expect(isGifUrl('https://giphy.com.evil.com/x.gif')).toBe(false);
		expect(isGifUrl('https://media.giphy.com/x.gif')).toBe(true);
	});
});

// ─────────────────────────────────────────────────────────────────────────────
// Ordinary parsing.
// ─────────────────────────────────────────────────────────────────────────────
describe('markdown — blocks', () => {
	it('starts headings at h2 so the post title keeps the only h1', () => {
		const levels = parseMarkdown('# unu\n\n## doi\n\n### trei').map(
			(b) => (b as Extract<Block, { t: 'h' }>).level
		);
		expect(levels).toEqual([2, 3, 4]);
	});

	it('needs a space after the hashes', () => {
		expect(parseMarkdown('#fara-spatiu')[0].t).toBe('p');
	});

	it('collects consecutive bullets into one list', () => {
		const [block] = parseMarkdown('- unu\n- doi\n- trei');
		expect(block.t).toBe('ul');
		expect((block as Extract<Block, { t: 'ul' }>).items).toHaveLength(3);
	});

	it('joins wrapped lines into one paragraph and splits on a blank line', () => {
		const blocks = parseMarkdown('linia unu\nlinia doi\n\nalt paragraf');
		expect(blocks).toHaveLength(2);
		expect(textOf((blocks[0] as Extract<Block, { t: 'p' }>).v)).toBe('linia unu linia doi');
	});

	it('reads quotes and dividers', () => {
		expect(parseMarkdown('> citat')[0].t).toBe('quote');
		expect(parseMarkdown('---')[0].t).toBe('hr');
	});

	it('survives empty and whitespace-only input', () => {
		expect(parseMarkdown('')).toEqual([]);
		expect(parseMarkdown('   \n\n  ')).toEqual([]);
		expect(parseMarkdown(undefined as unknown as string)).toEqual([]);
	});

	it('normalises windows line endings', () => {
		expect(parseMarkdown('unu\r\n\r\ndoi')).toHaveLength(2);
	});
});

describe('markdown — inline', () => {
	it('parses bold, italic, code and spoiler', () => {
		const kinds = firstPara('**b** *i* `c` ||s||').map((n) => n.t);
		expect(kinds).toContain('bold');
		expect(kinds).toContain('italic');
		expect(kinds).toContain('code');
		expect(kinds).toContain('spoiler');
	});

	it('nests emphasis inside a spoiler', () => {
		const spoiler = firstPara('||un **secret**||').find((n) => n.t === 'spoiler');
		expect(spoiler).toBeDefined();
		expect(textOf((spoiler as Extract<Inline, { t: 'spoiler' }>).v)).toBe('un secret');
	});

	it('honours backslash escapes', () => {
		expect(textOf(firstPara('\\*\\*nu e bold\\*\\*'))).toBe('**nu e bold**');
	});

	it('leaves emoji untouched', () => {
		// Emoji are ordinary characters; the editor's picker just inserts them.
		expect(textOf(firstPara('bravo 🎉 echipă'))).toBe('bravo 🎉 echipă');
	});

	it('does not treat an unmatched marker as emphasis', () => {
		expect(textOf(firstPara('2 * 3 = 6'))).toBe('2 * 3 = 6');
	});
});

// ─────────────────────────────────────────────────────────────────────────────
describe('markdownExcerpt', () => {
	it('takes plain text from paragraphs only', () => {
		expect(markdownExcerpt('# Titlu\n\nCorpul postării.')).toBe('Corpul postării.');
	});

	it('masks spoilers — an excerpt has no way to hide them again', () => {
		expect(markdownExcerpt('A murit ||personajul principal||.')).toBe('A murit •••.');
	});

	it('truncates on a word boundary', () => {
		const out = markdownExcerpt('cuvinte '.repeat(60), 40);
		expect(out.length).toBeLessThanOrEqual(41);
		expect(out.endsWith('…')).toBe(true);
		expect(out).not.toMatch(/cuv…$/);
	});

	it('leaves short text alone', () => {
		expect(markdownExcerpt('scurt')).toBe('scurt');
	});
});

// ─────────────────────────────────────────────────────────────────────────────
describe('parseText — comments, reviews, chat', () => {
	it('does not run the markdown parser', () => {
		// A comment that can mint headings and images is a different feature.
		const parts = parseText('# nu e titlu **nu e bold**');
		expect(parts).toEqual([{ kind: 'text', text: '# nu e titlu **nu e bold**' }]);
	});

	it('splits spoilers out of the surrounding text', () => {
		expect(parseText('a ||b|| c')).toEqual([
			{ kind: 'text', text: 'a ' },
			{ kind: 'spoiler', text: 'b' },
			{ kind: 'text', text: ' c' }
		]);
	});

	it('turns a bare giphy link into a gif', () => {
		const parts = parseText('uite https://media.giphy.com/media/x/giphy.gif');
		expect(parts[1]).toEqual({
			kind: 'gif',
			url: 'https://media.giphy.com/media/x/giphy.gif'
		});
	});

	it('leaves a non-giphy link as text', () => {
		expect(parseText('vezi https://example.com/x.gif')).toEqual([
			{ kind: 'text', text: 'vezi https://example.com/x.gif' }
		]);
	});

	it('keeps a gif hidden when it is inside a spoiler', () => {
		const parts = parseText('||https://media.giphy.com/media/x/giphy.gif||');
		expect(parts).toHaveLength(1);
		expect(parts[0].kind).toBe('spoiler');
	});

	it('returns nothing for an empty string', () => {
		expect(parseText('')).toEqual([]);
	});
});

describe('splitSpoilers — back-compat shim', () => {
	it('marks which halves are spoilers', () => {
		expect(splitSpoilers('vizibil ||ascuns||')).toEqual([
			{ text: 'vizibil ', spoiler: false },
			{ text: 'ascuns', spoiler: true }
		]);
	});

	it('drops gifs, since its callers only render text', () => {
		expect(splitSpoilers('a https://media.giphy.com/x.gif b').every((p) => 'text' in p)).toBe(
			true
		);
	});
});
