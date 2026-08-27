import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { searchAnime, searchManga } from '$lib/server/api';

/**
 * Combined search. The header used to send every query to `/anime?q=`, so a
 * member searching for a manga title got an empty anime catalog and no hint
 * that the manga existed.
 *
 * Both sides are fetched in parallel and either may be empty; the page decides
 * what to show. A failure on one side must not blank the other — a broken
 * manga search is not a reason to hide the anime results.
 */
export const load: PageServerLoad = async ({ url, fetch }) => {
  const q = (url.searchParams.get('q') ?? '').trim();
  if (!q) throw redirect(302, '/anime');

  const [anime, manga] = await Promise.all([
    searchAnime(fetch, q, { limit: 50 }).catch(() => ({ data: [], pagination: undefined })),
    searchManga(fetch, q, { limit: 50 }).catch(() => ({ data: [], pagination: undefined }))
  ]);

  return { q, anime: anime.data, manga: manga.data };
};
