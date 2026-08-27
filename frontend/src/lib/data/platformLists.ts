// Platform-curated lists ("de la Anime-Kage") — computed live from the catalog,
// not stored. Rendered on /liste (Populare) and opened at /liste/top/[slug].
import type { Anime, Manga } from '$shared/types';

export type PlatformListDef = {
  slug: string;
  kind: 'anime' | 'manga';
  title: string;
  desc: string;
  source: 'score' | 'watched';
  genre?: string;
};

export const PLATFORM_LIST_SIZE = 50;

export const PLATFORM_LISTS: PlatformListDef[] = [
  {
    slug: 'top-anime',
    kind: 'anime',
    title: 'Top 50 Anime',
    desc: 'Cele mai bine cotate anime după scorul MAL.',
    source: 'score'
  },
  {
    slug: 'top-manga',
    kind: 'manga',
    title: 'Top 50 Manga',
    desc: 'Cele mai bine cotate manga după scorul MAL.',
    source: 'score'
  },
  {
    slug: 'cele-mai-urmarite',
    kind: 'anime',
    title: 'Cele mai urmărite',
    desc: 'Anime-urile cu cei mai mulți urmăritori pe Anime-Kage.',
    source: 'watched'
  },
  {
    slug: 'top-actiune',
    kind: 'anime',
    title: 'Top Acțiune',
    desc: 'Cele mai bune anime de acțiune.',
    source: 'score',
    genre: 'Action'
  },
  {
    slug: 'top-fantezie',
    kind: 'anime',
    title: 'Top Fantezie',
    desc: 'Cele mai bune anime fantasy.',
    source: 'score',
    genre: 'Fantasy'
  },
  {
    slug: 'top-romance',
    kind: 'anime',
    title: 'Top Romantic',
    desc: 'Cele mai bune anime de dragoste.',
    source: 'score',
    genre: 'Romance'
  }
];

export const platformBySlug = (slug: string) => PLATFORM_LISTS.find((l) => l.slug === slug);

/**
 * Resolve a platform list's titles. `animeByScore`/`mangaByScore` are the
 * score-sorted catalogs; `animeByWatchers` is the leaderboard (all-time).
 */
export function platformItems(
  def: PlatformListDef,
  animeByScore: Anime[],
  mangaByScore: Manga[],
  animeByWatchers: Anime[]
): (Anime | Manga)[] {
  if (def.source === 'watched') return animeByWatchers.slice(0, PLATFORM_LIST_SIZE);
  const base: (Anime | Manga)[] = def.kind === 'manga' ? mangaByScore : animeByScore;
  const filtered = def.genre ? base.filter((x) => x.genres?.includes(def.genre!)) : base;
  return filtered.slice(0, PLATFORM_LIST_SIZE);
}
