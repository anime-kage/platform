import type { PageServerLoad } from './$types';
import { listAllManga, curatedSlots, curatedManga } from '$lib/server/api';
import type { Manga } from '$lib/types';

// SSR browse, mirroring /anime: filters come from the URL, data from the API.
// The manga endpoint has no sort param, so sorting happens here.
const sortManga = (list: Manga[], sort: string, dir: string) => {
  const s = list.slice();
  // comparators are written in each sort's NATURAL direction, then flipped —
  // one rule to reverse instead of six comparators to keep in step
  const flip = (n: number) => (dir === (sort === 'title' ? 'asc' : 'desc') ? n : -n);
  if (sort === 'title')
    return s.sort((a, b) => flip((a.titleEnglish ?? a.title).localeCompare(b.titleEnglish ?? b.title)));
  if (sort === 'year') return s.sort((a, b) => flip((b.year ?? 0) - (a.year ?? 0)));
  return s.sort((a, b) => flip((b.score ?? 0) - (a.score ?? 0)));
};

export const load: PageServerLoad = async ({ url, fetch }) => {
  const genre = url.searchParams.get('gen') ?? '';
  const year = url.searchParams.get('year') ?? '';
  const author = url.searchParams.get('author') ?? '';
  const sort = url.searchParams.get('sort') ?? 'score';
  // Each sort has a natural direction (best/newest first, but A→Z for title);
  // `dir` is only present in the URL when the user flipped it.
  const naturalDir = sort === 'title' ? 'asc' : 'desc';
  const dir = url.searchParams.get('dir') === (naturalDir === 'asc' ? 'desc' : 'asc')
    ? (naturalDir === 'asc' ? 'desc' : 'asc')
    : naturalDir;
  const view = url.searchParams.get('view') === 'list' ? 'list' : 'grid';
  const pageParam = Number(url.searchParams.get('page')) || 1;

  const all = (await listAllManga(fetch, 'score')).filter((m) => m.imageUrl);

  let list = sortManga(all, sort, dir);
  if (genre) list = list.filter((m) => m.genres?.includes(genre));
  if (year) list = list.filter((m) => m.year === Number(year));
  if (author) list = list.filter((m) => m.authors?.includes(author));

  // paginate after filtering — 60 per page (multiple of the 2–6 grid columns,
  // so rows stay full)
  const PAGE = 60;
  const count = list.length;
  const pages = Math.max(1, Math.ceil(count / PAGE));
  const page = Math.min(Math.max(1, pageParam), pages);
  list = list.slice((page - 1) * PAGE, page * PAGE);

  // Genres with frequency: the chip row shows only the most common ones,
  // the rest live behind the "+N" expander.
  const counts = new Map<string, number>();
  for (const m of all) for (const g of m.genres ?? []) counts.set(g, (counts.get(g) ?? 0) + 1);
  const genres = Array.from(counts.keys()).sort();
  const genresTop = Array.from(counts.entries())
    .sort((x, y) => y[1] - x[1] || x[0].localeCompare(y[0]))
    .slice(0, 10)
    .map(([g]) => g)
    .sort();

  // distinct years, newest first, for the year filter
  const years = Array.from(new Set(all.map((m) => m.year).filter((y): y is number => !!y))).sort(
    (a, b) => b - a
  );

  // distinct authors, A–Z, for the author filter
  const authors = Array.from(new Set(all.flatMap((m) => m.authors ?? []))).sort((a, b) =>
    a.localeCompare(b)
  );

  // Library recommendation; hidden while filtering so results stay
  // uncluttered. The coordinator's pick wins when it is shown at all.
  const showFeatured = !genre && !year && !author && page === 1;
  const featured = showFeatured
    ? (curatedManga(await curatedSlots(fetch), 'manga_featured') ??
      all.find((m) => m.synopsis && m.synopsis.length > 80) ??
      all[0] ??
      null)
    : null;

  return { dir, featured, list, count, page, pages, genres, genresTop, genre, year, author, years, authors, sort, view, total: all.length };
};
