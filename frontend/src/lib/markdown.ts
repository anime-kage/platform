/**
 * A deliberately small Markdown subset for news posts.
 *
 * It parses to a token tree, NOT to an HTML string. `Markdown.svelte` renders
 * those tokens as real elements, so there is no `{@html}` anywhere and no
 * sanitiser to get wrong — a post cannot inject markup because markup is never
 * produced. That is the whole reason this exists instead of `marked` +
 * `DOMPurify`: the body is written by staff today, but "trusted input" is how
 * stored XSS gets in, and the front page is the worst place to find out.
 *
 * Supported, because it is what a news post actually needs:
 *   # / ## / ###   headings
 *   **bold**  *italic*  `code`  ||spoiler||
 *   [text](url)    links — internal paths and https only
 *   ![alt](url)    images — our own /uploads/ only
 *   - item         bullet lists
 *   > quote        blockquote
 *   ---            divider
 *   blank line     paragraph break
 *
 * Emoji need no support at all: they are ordinary characters and travel through
 * untouched, which is why the editor's picker just inserts them into the text.
 */

export type Inline =
  | { t: 'text'; v: string }
  | { t: 'bold'; v: Inline[] }
  | { t: 'italic'; v: Inline[] }
  | { t: 'code'; v: string }
  | { t: 'spoiler'; v: Inline[] }
  | { t: 'link'; href: string; v: Inline[] };

export type Block =
  | { t: 'p'; v: Inline[] }
  | { t: 'h'; level: 2 | 3 | 4; v: Inline[] }
  | { t: 'ul'; items: Inline[][] }
  | { t: 'quote'; v: Inline[] }
  | { t: 'img'; src: string; alt: string }
  | { t: 'hr' };

/**
 * Only links we are willing to render as an anchor. Anything else becomes
 * plain text, so a `javascript:` URL in a post is inert rather than filtered —
 * there is no filter to bypass.
 */
function safeHref(raw: string): string | null {
  const u = raw.trim();
  if (u.startsWith('/') && !u.startsWith('//')) return u;
  if (/^https:\/\/[^/\s]+/i.test(u)) return u;
  return null;
}

/**
 * Giphy's CDN. An allowlist, not a general "remote images are fine" — an
 * arbitrary <img> in user text reports every reader's IP to whoever owns the
 * host, so exactly two sources are permitted: our own uploads, and the GIF CDN
 * the picker draws from.
 */
const GIF_HOST = /^https:\/\/(?:[a-z0-9-]+\.)?giphy\.com\//i;

export function isGifUrl(raw: string): boolean {
  return GIF_HOST.test(raw.trim());
}

/** Images must be our own uploads or a Giphy CDN URL — nothing else. */
function safeSrc(raw: string): string | null {
  const u = raw.trim();
  if (u.startsWith('/uploads/')) return u;
  if (isGifUrl(u)) return u;
  return null;
}

const ESCAPES = /\\([\\`*_[\]()#>-])/g;

/** Inline parse: bold, italic, code, links. Left-to-right, no backtracking. */
function inline(src: string): Inline[] {
  const out: Inline[] = [];
  let buf = '';
  const flush = () => {
    if (buf) {
      out.push({ t: 'text', v: buf.replace(ESCAPES, '$1') });
      buf = '';
    }
  };

  for (let i = 0; i < src.length; i++) {
    const rest = src.slice(i);

    if (src[i] === '\\' && i + 1 < src.length) {
      buf += src[i] + src[i + 1];
      i++;
      continue;
    }
    // `code`
    let m = /^`([^`]+)`/.exec(rest);
    if (m) {
      flush();
      out.push({ t: 'code', v: m[1] });
      i += m[0].length - 1;
      continue;
    }
    // ||spoiler|| — Discord's syntax, and the one people already type
    m = /^\|\|([\s\S]+?)\|\|/.exec(rest);
    if (m) {
      flush();
      out.push({ t: 'spoiler', v: inline(m[1]) });
      i += m[0].length - 1;
      continue;
    }
    // **bold**
    m = /^\*\*([\s\S]+?)\*\*/.exec(rest);
    if (m) {
      flush();
      out.push({ t: 'bold', v: inline(m[1]) });
      i += m[0].length - 1;
      continue;
    }
    // *italic*
    m = /^\*([^*\n]+?)\*/.exec(rest);
    if (m) {
      flush();
      out.push({ t: 'italic', v: inline(m[1]) });
      i += m[0].length - 1;
      continue;
    }
    // [text](href)
    m = /^\[([^\]]*)\]\(([^)\s]+)\)/.exec(rest);
    if (m) {
      const href = safeHref(m[2]);
      flush();
      if (href) out.push({ t: 'link', href, v: inline(m[1]) });
      else out.push({ t: 'text', v: m[1] }); // unusable URL → just the words
      i += m[0].length - 1;
      continue;
    }
    buf += src[i];
  }
  flush();
  return out;
}

/** Block parse. Input is the raw post body; output is what Markdown.svelte draws. */
export function parseMarkdown(src: string): Block[] {
  const lines = (src ?? '').replace(/\r\n?/g, '\n').split('\n');
  const out: Block[] = [];
  let para: string[] = [];

  const flushPara = () => {
    if (para.length) {
      out.push({ t: 'p', v: inline(para.join(' ').trim()) });
      para = [];
    }
  };

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const trimmed = line.trim();

    if (!trimmed) {
      flushPara();
      continue;
    }
    // ![alt](src) on its own line — a figure, not an inline image
    let m = /^!\[([^\]]*)\]\(([^)\s]+)\)$/.exec(trimmed);
    if (m) {
      const src2 = safeSrc(m[2]);
      flushPara();
      if (src2) out.push({ t: 'img', src: src2, alt: m[1] });
      continue;
    }
    if (/^---+$/.test(trimmed)) {
      flushPara();
      out.push({ t: 'hr' });
      continue;
    }
    // Headings start at h2: the page's h1 is the post title, and a body that
    // can mint a second h1 breaks the document outline.
    m = /^(#{1,3})\s+(.*)$/.exec(trimmed);
    if (m) {
      flushPara();
      out.push({ t: 'h', level: (m[1].length + 1) as 2 | 3 | 4, v: inline(m[2]) });
      continue;
    }
    if (/^>\s?/.test(trimmed)) {
      flushPara();
      out.push({ t: 'quote', v: inline(trimmed.replace(/^>\s?/, '')) });
      continue;
    }
    if (/^[-*]\s+/.test(trimmed)) {
      flushPara();
      const items: Inline[][] = [];
      while (i < lines.length && /^[-*]\s+/.test(lines[i].trim())) {
        items.push(inline(lines[i].trim().replace(/^[-*]\s+/, '')));
        i++;
      }
      i--;
      out.push({ t: 'ul', items });
      continue;
    }
    para.push(trimmed);
  }
  flushPara();
  return out;
}

/** First ~N characters of plain text — the list page's excerpt. */
export function markdownExcerpt(src: string, max = 180): string {
  const text = parseMarkdown(src)
    .filter((b) => b.t === 'p' || b.t === 'quote')
    .map((b) => inlineText((b as { v: Inline[] }).v))
    .join(' ')
    .trim();
  return text.length > max ? text.slice(0, max).replace(/\s+\S*$/, '') + '…' : text;
}

function inlineText(nodes: Inline[]): string {
  return nodes
    .map((n) =>
      n.t === 'text'
        ? n.v
        : n.t === 'code'
          ? n.v
          : // An excerpt is shown without any way to hide it again, so a
            // spoiler's words must not travel into one.
            n.t === 'spoiler'
            ? '•••'
            : inlineText((n as { v: Inline[] }).v)
    )
    .join('');
}

/** One piece of a comment / review / chat line. */
export type TextPart =
  | { kind: 'text'; text: string }
  | { kind: 'spoiler'; text: string }
  | { kind: 'gif'; url: string };

/**
 * Parse plain text — the comment / review / chat path, which deliberately does
 * NOT run the Markdown parser (a comment that can mint headings and images is a
 * different feature with a different moderation story).
 *
 * Two things are recognised:
 *   ||spoiler||            same delimiter as a news post, so it is learnt once
 *   a bare Giphy URL       becomes the GIF, the way pasting a link works in
 *                          every chat app — no syntax for anyone to remember
 *
 * A spoiler wrapping a GIF link keeps working: the spoiler split happens first,
 * and the renderer parses each half independently.
 */
export function parseText(src: string): TextPart[] {
  const out: TextPart[] = [];

  const pushPlain = (chunk: string) => {
    // Inside a non-spoiler run, pull out bare GIF links.
    const re = /https:\/\/[^\s<>"']+/g;
    let last = 0;
    let m: RegExpExecArray | null;
    while ((m = re.exec(chunk)) !== null) {
      if (!isGifUrl(m[0])) continue;
      if (m.index > last) out.push({ kind: 'text', text: chunk.slice(last, m.index) });
      out.push({ kind: 'gif', url: m[0] });
      last = m.index + m[0].length;
    }
    if (last < chunk.length) out.push({ kind: 'text', text: chunk.slice(last) });
  };

  const spoilerRe = /\|\|([\s\S]+?)\|\|/g;
  let last = 0;
  let m: RegExpExecArray | null;
  while ((m = spoilerRe.exec(src)) !== null) {
    if (m.index > last) pushPlain(src.slice(last, m.index));
    out.push({ kind: 'spoiler', text: m[1] });
    last = m.index + m[0].length;
  }
  if (last < src.length) pushPlain(src.slice(last));
  return out;
}

/** Back-compat shim for callers that only care about spoilers. */
export function splitSpoilers(src: string): { text: string; spoiler: boolean }[] {
  return parseText(src)
    .filter((p) => p.kind !== 'gif')
    .map((p) => ({ text: p.kind === 'spoiler' ? p.text : (p as { text: string }).text, spoiler: p.kind === 'spoiler' }));
}
