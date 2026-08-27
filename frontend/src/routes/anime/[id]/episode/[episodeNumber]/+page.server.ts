import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { getAnime, getEpisodes, animeRelations } from '$lib/server/api';
import type { RelatedAnime } from '$shared/types';

/** Where "next" (or "previous") goes when it leaves this season. */
export type SeasonJump = { anime: RelatedAnime; episodeNumber: number };

export const load: PageServerLoad = async ({ params, url, fetch }) => {
  const epNum = Number(params.episodeNumber);
  if (!Number.isFinite(epNum)) throw error(404, 'Episod negăsit');

  // Slug or numeric id (see the sibling +page.server.ts).
  const anime = await getAnime(fetch, params.id);
  if (!anime) throw error(404, 'Anime negăsit');
  if (anime.slug && params.id !== anime.slug) {
    throw redirect(301, `/anime/${anime.slug}/episode/${params.episodeNumber}${url.search}`);
  }

  const id = anime.id;
  const [episodes, relations] = await Promise.all([
    getEpisodes(fetch, id),
    animeRelations(fetch, id)
  ]);

  const sorted = episodes.slice().sort((a, b) => a.episodeNumber - b.episodeNumber);
  const idx = sorted.findIndex((e) => e.episodeNumber === epNum);
  if (idx === -1) throw error(404, 'Episod negăsit');

  const prev = idx > 0 ? sorted[idx - 1].episodeNumber : null;
  const next = idx < sorted.length - 1 ? sorted[idx + 1].episodeNumber : null;

  // The end of a season is not the end of the story. Crunchyroll rolls "next"
  // into the following season rather than greying it out, and that is the one
  // moment a viewer most needs to be told there IS a following season.
  //
  // Only a neighbour we actually host episodes for is offered — a link into a
  // season with an empty episode list is worse than no link.
  const chain = relations.chain;
  const here = chain.findIndex((c) => c.id === id);

  let nextSeason: SeasonJump | null = null;
  let prevSeason: SeasonJump | null = null;

  if (here !== -1) {
    if (next === null) {
      const after = chain.slice(here + 1).find((c) => c.episodeCount > 0);
      if (after) {
        // The real first episode number, not 1: a split-cour continuation is
        // often numbered from where the previous part stopped (…13, 14),
        // and guessing "1" would 404 on exactly those.
        const nums = (await getEpisodes(fetch, after.id)).map((e) => e.episodeNumber);
        if (nums.length) nextSeason = { anime: after, episodeNumber: Math.min(...nums) };
      }
    }
    if (prev === null) {
      // Backwards lands on the previous season's *last* episode, which is
      // where you'd actually resume from, not on its first.
      const before = chain
        .slice(0, here)
        .reverse()
        .find((c) => c.episodeCount > 0);
      if (before) {
        const nums = (await getEpisodes(fetch, before.id)).map((e) => e.episodeNumber);
        if (nums.length) prevSeason = { anime: before, episodeNumber: Math.max(...nums) };
      }
    }
  }

  return {
    anime,
    episodes: sorted,
    episode: sorted[idx],
    prev,
    next,
    nextSeason,
    prevSeason
  };
};
