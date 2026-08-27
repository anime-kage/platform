// Cross-boundary types come from shared/types.ts (single source of truth,
// camelCase — matches the backend JSON). Add frontend-only helpers here.
export type { Anime, Manga, Episode, Chapter, ContentLink, User } from '$shared/types';
import type { Anime, Manga } from '$shared/types';

export const displayName = (
  a: Pick<Anime | Manga, 'title' | 'titleEnglish' | 'titleRomanian'>
): string => a.titleRomanian ?? a.titleEnglish ?? a.title;

export const studioOf = (a: Anime): string | null =>
  a.studios && a.studios.length ? a.studios[0] : null;

/**
 * Trailing credit lines MAL and Jikan append to a synopsis — "(Source: Anime
 * News Network)", "[Written by MAL Rewrite]". They are provenance for the
 * scraper, not part of the description, and they read as debris on a title
 * page.
 *
 * Stripped at display rather than at import so it also covers the ~35 titles
 * imported before this existed, with no backfill. The auto-translator is told
 * to drop them too (internal/translate: TranslateProse), but it only runs when
 * ANTHROPIC_API_KEY is configured and it is a model instruction, not a
 * guarantee — so this stays as the deterministic net under both languages.
 */
const SYNOPSIS_ATTRIBUTION = /\s*(?:\((?:Source|Sursa)\s*:[^)]*\)|\[(?:Written|Adapted)[^\]]*\])\s*$/gi;

const cleanSynopsis = (s: string | null | undefined): string | null => {
  if (!s) return null;
  let out = s.trim();
  // Loop: a synopsis can carry both a "(Source: …)" and a "[Written by …]".
  let prev;
  do {
    prev = out;
    out = out.replace(SYNOPSIS_ATTRIBUTION, '').trim();
  } while (out !== prev);
  return out || null;
};

/** The catalog synopsis we show: the Romanian one when it exists. */
export const displaySynopsis = (a: Pick<Anime | Manga, 'synopsis' | 'synopsisRomanian'>): string | null =>
  cleanSynopsis(a.synopsisRomanian) ?? cleanSynopsis(a.synopsis) ?? null;

/**
 * The URL segment for a title: its slug, or the numeric id when it has none.
 *
 * Both resolve — the API accepts either and the page redirects a numeric id to
 * its slug — so this is about which URL the user *sees*, not whether the link
 * works. Anything holding only an id can keep passing the id.
 */
export const titleRef = (t: Pick<Anime | Manga, 'id'> & { slug?: string }): string | number =>
  t.slug ?? t.id;

/** `/anime/91-days` (or `/anime/36` before the slug backfill has run). */
export const animeHref = (a: Pick<Anime, 'id'> & { slug?: string }) => `/anime/${titleRef(a)}`;

/** `/manga/vagabond` */
export const mangaHref = (m: Pick<Manga, 'id'> & { slug?: string }) => `/manga/${titleRef(m)}`;

/** `/anime/91-days/episode/3` */
export const episodeHref = (a: Pick<Anime, 'id'> & { slug?: string }, episodeNumber: number) =>
  `/anime/${titleRef(a)}/episode/${episodeNumber}`;

// MAL's genre vocabulary is small and fixed — a static map beats an LLM for
// this. DB values stay English (filters key on them); only labels translate.
//
// The rule, after "Felie de viață" made the catalog read like a bad dub:
//
//   Translate a genre only where Romanian already has the word people use for
//   it. Where the Romanian anime audience says the English or Japanese term,
//   keep it — translating those invents vocabulary nobody uses.
//
// This is not a stylistic preference, it is what the audience is trained on.
// Crunchyroll ships its interface in 21 languages and Romanian is not among
// them, so every Romanian watching there reads "Slice of Life", "Isekai" and
// "Shounen" in English. Netflix does localise, but only the broad film genres
// (Acțiune, Comedie, Dramă, Romantic) — it has no Romanian word for the
// anime-native ones either, because there isn't one.
//
// Anything unmapped falls through to English via genreRo, which is the correct
// default under this rule rather than a gap to be filled.

// ── Genres Romanian genuinely has a word for ──────────────────────────────
const GENRE_RO_TRANSLATED: Record<string, string> = {
  Action: 'Acțiune',
  Adventure: 'Aventură',
  Comedy: 'Comedie',
  Drama: 'Dramă',
  Fantasy: 'Fantezie',
  Mystery: 'Mister',
  Romance: 'Romantic',
  'Sci-Fi': 'SF',
  Sports: 'Sport',
  Supernatural: 'Supranatural',
  Suspense: 'Suspans',
  'Avant Garde': 'Avangardă',
  'Award Winning': 'Premiat',
  Gourmet: 'Culinar',
  Music: 'Muzică',
  Psychological: 'Psihologic',
  Historical: 'Istoric',
  Military: 'Militar',
  School: 'Școală',
  Kids: 'Pentru copii',
  Parody: 'Parodie',
  Space: 'Spațiu',
  Vampire: 'Vampiri',
  'Martial Arts': 'Arte marțiale',
  'Super Power': 'Superputeri',
  Demons: 'Demoni',
  Game: 'Jocuri',
  // "Poliție" is the institution; the *genre* in Romanian is "polițist",
  // as in "film polițist" — the same word Romanian uses for a police drama.
  Police: 'Polițist',

  // MAL themes that arrive through the same field and do have plain Romanian
  Detective: 'Detectivi',
  Survival: 'Supraviețuire',
  'Time Travel': 'Călătorii în timp',
  Reincarnation: 'Reîncarnare',
  Mythology: 'Mitologie',
  Medical: 'Medical',
  Racing: 'Curse',
  Educational: 'Educativ',
  'Team Sports': 'Sporturi de echipă',
  'Combat Sports': 'Sporturi de contact',
  'Video Game': 'Jocuri video',
  'Strategy Game': 'Jocuri de strategie',
  'Organized Crime': 'Crimă organizată',
  'Visual Arts': 'Arte vizuale',
  'Performing Arts': 'Artele spectacolului',
  'Adult Cast': 'Personaje adulte',
  Anthropomorphic: 'Antropomorf',
  Delinquents: 'Delincvenți',
  Pets: 'Animale de companie',
  Childcare: 'Îngrijirea copiilor',
  Mythological: 'Mitologic',
  Erotica: 'Erotic',
  Workplace: 'Viață profesională',
  'Otaku Culture': 'Cultură otaku',
  'Romantic Subtext': 'Subtext romantic',
  'Love Polygon': 'Triunghi amoros'
};

// ── Terms the Romanian anime audience uses untranslated ───────────────────
// Listed explicitly rather than left to the fallback, so it is on the record
// that each one was considered and deliberately kept.
const GENRE_RO_KEPT = [
  // "Felie de viață" is a literal calque nobody says out loud. This is the
  // term the audience actually reads, on Crunchyroll and everywhere else.
  'Slice of Life',
  // Romanian has "groază", but Netflix uses it for horror *film*; the anime
  // audience says horror, and MAL's tag is what they are searching for.
  'Horror',
  'Thriller',
  // Japanese demographic and genre vocabulary — untranslatable by design.
  'Ecchi',
  'Isekai',
  'Mecha',
  'Josei',
  'Seinen',
  'Shoujo',
  'Shounen',
  'Harem',
  'Reverse Harem',
  'Samurai',
  'Iyashikei',
  'Mahou Shoujo',
  'CGDCT',
  'Gore',
  // MAL renamed Yaoi/Yuri to these; the English is what the tag now reads.
  'Boys Love',
  'Girls Love'
] as const;

const GENRE_RO: Record<string, string> = {
  ...GENRE_RO_TRANSLATED,
  ...Object.fromEntries(GENRE_RO_KEPT.map((g) => [g, g]))
};

export const genreRo = (g: string): string => GENRE_RO[g] ?? g;

const SEASON_RO: Record<string, string> = {
  winter: 'Iarnă',
  spring: 'Primăvară',
  summer: 'Vară',
  fall: 'Toamnă'
};

export const seasonRo = (s: string): string => SEASON_RO[s.toLowerCase()] ?? s;

/** One combined token for the meta line — "Primăvară 2012", never a year and
    a season stranded at opposite ends of the row. */
export const seasonYearLabel = (a: Pick<Anime, 'season' | 'year'>): string | null => {
  if (a.season && a.year) return `${seasonRo(a.season)} ${a.year}`;
  if (a.year) return String(a.year);
  return null;
};
