import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { getAnime, getEpisodes, listAllAnime, animeRelations } from '$lib/server/api';

/** Episodes per page in the "Episoade" tab. One Piece has 1167 of them, which
 *  rendered as one list was ~750 KB of DOM on top of a 213 KB payload. */
const EP_PER_PAGE = 100;

export const load: PageServerLoad = async ({ params, url, fetch }) => {
  // `params.id` is a slug ("91-days") or a numeric id — the API resolves both,
  // so old links and shared numeric URLs keep working.
  const anime = await getAnime(fetch, params.id);
  if (!anime) throw error(404, 'Anime negăsit');

  // One canonical URL per series: a numeric id redirects to the slug. This is
  // what makes every existing `/anime/${a.id}` link on the site land on a pretty
  // URL without all 58 of them having to be rewritten.
  if (anime.slug && params.id !== anime.slug) {
    throw redirect(301, `/anime/${anime.slug}${url.search}`);
  }

  const id = anime.id;
  const [allEpisodes, relations] = await Promise.all([
    getEpisodes(fetch, id),
    animeRelations(fetch, id)
  ]);

  // Sliced here rather than in the browser: the backend call is server-to-
  // server so fetching all of them is cheap, but shipping all of them to the
  // page is not — only the requested page crosses the wire.
  const epPages = Math.max(1, Math.ceil(allEpisodes.length / EP_PER_PAGE));
  const epPage = Math.min(Math.max(1, Number(url.searchParams.get('ep')) || 1), epPages);
  const episodes = allEpisodes.slice((epPage - 1) * EP_PER_PAGE, epPage * EP_PER_PAGE);

  // A compact index of the whole run, for the parts that need every episode
  // whichever page is shown: the comment-scope picker and the "watch episode
  // 1" button. Ids and numbers only — the per-episode `links` arrays are what
  // make the full list heavy.
  const episodeIndex = allEpisodes.map((e) => ({ id: e.id, episodeNumber: e.episodeNumber }));

  // "Similare" — share at least one genre, ranked by score.
  const genres = new Set(anime.genres ?? []);
  const similar = (await listAllAnime(fetch, 'score'))
    .filter((a) => a.id !== id && a.imageUrl && (a.genres ?? []).some((g) => genres.has(g)))
    .slice(0, 6);

  return {
    anime,
    episodes,
    episodeIndex,
    epPage,
    epPages,
    epTotal: allEpisodes.length,
    similar,
    relations
  };
};
