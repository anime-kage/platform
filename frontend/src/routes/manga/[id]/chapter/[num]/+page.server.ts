import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { getChapter, getChapters, getManga } from '$lib/server/api';

export const load: PageServerLoad = async ({ params, url, fetch }) => {
  const manga = await getManga(fetch, params.id);
  if (!manga) throw error(404, 'Manga negăsit');
  if (manga.slug && params.id !== manga.slug) {
    throw redirect(301, `/manga/${manga.slug}/chapter/${params.num}${url.search}`);
  }

  const id = manga.id;
  const [chapters, chapter] = await Promise.all([
    getChapters(fetch, id),
    getChapter(fetch, id, params.num)
  ]);
  if (!manga || !chapter) throw error(404, 'Capitol negăsit');

  const sorted = chapters
    .slice()
    .sort((a, b) => parseFloat(a.chapterNumber) - parseFloat(b.chapterNumber));
  const idx = sorted.findIndex((c) => c.id === chapter.id);

  const numOf = (i: number) => String(parseFloat(sorted[i].chapterNumber));

  return {
    manga,
    chapters: sorted,
    chapter,
    prev: idx > 0 ? numOf(idx - 1) : null,
    next: idx >= 0 && idx < sorted.length - 1 ? numOf(idx + 1) : null
  };
};
