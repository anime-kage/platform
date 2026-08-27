import { error, redirect } from '@sveltejs/kit';
import { getAnime, getReview } from '$lib/server/api';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ params, url, fetch }) => {
  const entryId = Number(params.entry);
  if (!Number.isInteger(entryId)) throw error(404, 'Recenzie inexistentă');

  // Slug or numeric id; the review itself is always keyed on the numeric id.
  const anime = await getAnime(fetch, params.id);
  if (!anime) throw error(404, 'Recenzie inexistentă');
  if (anime.slug && params.id !== anime.slug) {
    throw redirect(301, `/anime/${anime.slug}/review/${params.entry}${url.search}`);
  }

  const review = await getReview(fetch, 'anime', anime.id, entryId);
  if (!review) throw error(404, 'Recenzie inexistentă');

  return { anime, review };
};
