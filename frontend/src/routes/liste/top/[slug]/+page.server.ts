import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { listAllAnime, listAllManga, leaderboardAnime } from '$lib/server/api';
import { platformBySlug, platformItems, PLATFORM_LIST_SIZE } from '$lib/data/platformLists';

const PER = 20;

export const load: PageServerLoad = async ({ fetch, url, params }) => {
  const def = platformBySlug(params.slug);
  if (!def) throw error(404, 'Lista nu există');

  const needAnime = def.kind === 'anime' && def.source === 'score';
  const needManga = def.kind === 'manga';
  const needWatched = def.source === 'watched';

  const [anime, manga, watched] = await Promise.all([
    needAnime ? listAllAnime(fetch, 'score') : Promise.resolve([]),
    needManga ? listAllManga(fetch, 'score') : Promise.resolve([]),
    needWatched ? leaderboardAnime(fetch, 'all', PLATFORM_LIST_SIZE) : Promise.resolve([])
  ]);

  const all = platformItems(
    def,
    anime.filter((a) => a.imageUrl),
    manga.filter((m) => m.imageUrl),
    watched.filter((a) => a.imageUrl)
  );

  const pageParam = Number(url.searchParams.get('page')) || 1;
  const pages = Math.max(1, Math.ceil(all.length / PER));
  const page = Math.min(Math.max(1, pageParam), pages);
  const items = all
    .slice((page - 1) * PER, page * PER)
    .map((a, i) => ({ a, rank: (page - 1) * PER + i + 1 }));

  return {
    def,
    total: all.length,
    page,
    pages,
    items,
    covers: all.slice(0, 5).map((a) => a.imageUrl as string)
  };
};
