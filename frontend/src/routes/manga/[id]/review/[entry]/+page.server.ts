import { error, redirect } from '@sveltejs/kit';
import { getManga, getReview } from '$lib/server/api';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ params, url, fetch }) => {
  const entryId = Number(params.entry);
  if (!Number.isInteger(entryId)) throw error(404, 'Recenzie inexistentă');

  // Slug or numeric id; the review itself is always keyed on the numeric id.
  const manga = await getManga(fetch, params.id);
  if (!manga) throw error(404, 'Recenzie inexistentă');
  if (manga.slug && params.id !== manga.slug) {
    throw redirect(301, `/manga/${manga.slug}/review/${params.entry}${url.search}`);
  }

  const review = await getReview(fetch, 'manga', manga.id, entryId);
  if (!review) throw error(404, 'Recenzie inexistentă');

  return { manga, review };
};
