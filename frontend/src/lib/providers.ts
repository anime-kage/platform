/**
 * Human labels for `content_links.provider` (PLAN 3.1).
 *
 * The column holds a lowercase slug — the same key the resolver registry uses
 * for `kind='extract'` rows — so an embed source can be promoted to extract
 * later without being renamed. This file is the only place a slug turns into
 * something a member reads.
 *
 * Slugs are the service, not the domain a particular link happens to sit on:
 * file hosts rotate domains constantly (DoodStream alone appears as dood.li,
 * doods.pro, d0000d.com and playmogo.com in one import), and a member picking
 * a source cares which player they're getting, not which mirror serves it.
 * Anything unmapped is title-cased from the slug, so a provider an import
 * script knows about before this file does still renders as a name.
 */
const LABELS: Record<string, string> = {
  abstream: 'Abstream',
  // Manga chapter hosts. These are document viewers rather than video hosts,
  // but they arrive through the same content_links table and the same reader
  // fallback, so they need names here too.
  calameo: 'Calaméo',
  docdroid: 'DocDroid',
  drive: 'Google Drive',
  flipbook: 'FlipBook',
  flipsnack: 'FlipSnack',
  mangadex: 'MangaDex',
  onedrive: 'OneDrive',
  scribd: 'Scribd',
  abyss: 'Abyss',
  dood: 'DoodStream',
  doodstream: 'DoodStream',
  filemoon: 'Filemoon',
  hexload: 'Hexload',
  hexupload: 'Hexupload',
  luluvdo: 'Luluvdo',
  mega: 'MEGA',
  mp4upload: 'Mp4Upload',
  sibnet: 'Sibnet',
  streamtape: 'Streamtape',
  streamwish: 'StreamWish',
  uqload: 'Uqload',
  veev: 'Veev',
  vidara: 'Vidara',
  vidhide: 'VidHide',
  vidmoly: 'Vidmoly',
  voe: 'VOE',
  vtbe: 'VTBE'
};

/** Slug → label, title-casing anything this file has not been taught. */
export function providerLabel(slug: string): string {
  const key = slug.trim().toLowerCase();
  if (!key) return '';
  return LABELS[key] ?? key.charAt(0).toUpperCase() + key.slice(1);
}

/** The host's own name, for rows stored before `provider` was being written. */
function hostLabel(url: string): string {
  try {
    const host = new URL(url).hostname.replace(/^www\./, '').replace(/\.$/, '');
    const [base] = host.split('.');
    return base ? providerLabel(base) : '';
  } catch {
    return '';
  }
}

type NamedSource = { provider?: string; hostingUrl?: string };

/**
 * What to call a source in the UI. `provider` when the row carries one, the
 * host name when it does not, and a positional fallback last so a source
 * button is never blank.
 */
export function sourceName(link: NamedSource, index = 0): string {
  return (
    (link.provider ? providerLabel(link.provider) : '') ||
    (link.hostingUrl ? hostLabel(link.hostingUrl) : '') ||
    `Sursa ${index + 1}`
  );
}
