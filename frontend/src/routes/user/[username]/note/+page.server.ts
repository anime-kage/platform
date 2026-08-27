import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { getPublicUser, getUserWatchlist, getUserReadlist } from '$lib/server/api';

/** One rated title. `stars` is the 1–5 the user actually picked: the column
 *  stores score = stars × 2, so everything here halves it once, here. */
export type RatedTitle = {
  key: string;
  kind: 'anime' | 'manga';
  id: number;
  title: string;
  year?: number;
  imageUrl?: string;
  stars: number;
  ratedAt: string;
};

export const load: PageServerLoad = async ({ params, fetch }) => {
  const handle = params.username;
  const user = await getPublicUser(fetch, handle);
  if (!user) throw error(404, 'Utilizatorul nu a fost găsit');

  const [wl, rl] = await Promise.all([
    getUserWatchlist(fetch, handle),
    getUserReadlist(fetch, handle)
  ]);

  const rated: RatedTitle[] = [];
  for (const e of wl ?? []) {
    if (!e.score || !e.anime) continue;
    rated.push({
      key: `a${e.anime.id}`,
      kind: 'anime',
      id: e.anime.id,
      title: e.anime.titleRomanian || e.anime.title,
      year: e.anime.year,
      imageUrl: e.anime.imageUrl,
      stars: e.score / 2,
      ratedAt: String(e.updatedAt)
    });
  }
  for (const e of rl ?? []) {
    if (!e.score || !e.manga) continue;
    rated.push({
      key: `m${e.manga.id}`,
      kind: 'manga',
      id: e.manga.id,
      title: e.manga.titleRomanian || e.manga.title,
      year: e.manga.year,
      imageUrl: e.manga.imageUrl,
      stars: e.score / 2,
      ratedAt: String(e.updatedAt)
    });
  }

  return { handle, name: user.user.username, rated };
};
