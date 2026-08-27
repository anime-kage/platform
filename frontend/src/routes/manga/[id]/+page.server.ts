import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { getChapters, getManga, listAllManga } from '$lib/server/api';

export const load: PageServerLoad = async ({ params, url, fetch }) => {
  // Slug ("vagabond") or numeric id — the API resolves both.
  const manga = await getManga(fetch, params.id);
  if (!manga) throw error(404, 'Manga negăsit');
  if (manga.slug && params.id !== manga.slug) {
    throw redirect(301, `/manga/${manga.slug}${url.search}`);
  }

  const id = manga.id;
  const chapters = await getChapters(fetch, id);

  const genres = new Set(manga.genres ?? []);
  const similar = (await listAllManga(fetch, 'score'))
    .filter((m) => m.id !== id && m.imageUrl && (m.genres ?? []).some((g) => genres.has(g)))
    .slice(0, 6);

  return { manga, chapters, similar };
};
