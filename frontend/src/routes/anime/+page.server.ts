import type { PageServerLoad } from './$types';
import { listAllAnime, searchAnime, curatedSlots, curatedAnime } from '$lib/server/api';
import { displayName } from '$lib/types';

// SSR browse: filters come from the URL, data from the backend API.
export const load: PageServerLoad = async ({ url, fetch }) => {
  const q = (url.searchParams.get('q') ?? '').trim();
  const genre = url.searchParams.get('gen') ?? '';
  const year = url.searchParams.get('year') ?? '';
  const season = url.searchParams.get('season') ?? '';
  const studio = url.searchParams.get('studio') ?? '';
  const sort = url.searchParams.get('sort') ?? 'score';
  // Each sort has a natural direction (best/newest first, but A→Z for title);
  // `dir` is only present in the URL when the user flipped it.
  const naturalDir = sort === 'title' ? 'asc' : 'desc';
  const dir = url.searchParams.get('dir') === (naturalDir === 'asc' ? 'desc' : 'asc')
    ? (naturalDir === 'asc' ? 'desc' : 'asc')
    : naturalDir;
  const view = url.searchParams.get('view') === 'list' ? 'list' : 'grid';
  const pageParam = Number(url.searchParams.get('page')) || 1;

  // Full catalog once (2 requests at current scale) — used for genre chips
  // and as the base list; text search goes through the API's search endpoint.
  const all = await listAllAnime(fetch, sort, dir);

  // A→Z has to order by the name on the card, not the one in the `title`
  // column. The API sorts by `title` (the romaji), while the card shows
  // displayName() = titleRomanian ?? titleEnglish ?? title — so "Sousou no
  // Frieren" filed under S but displayed as "Frieren: Beyond Journey's End"
  // looked misplaced under F. Re-sorted here rather than in SQL because the
  // list is already in memory and this keeps the ordering identical to the
  // search path below, which has always sorted on the displayed name.
  if (sort === 'title') {
    const flip = (n: number) => (dir === 'asc' ? n : -n);
    all.sort((a, b) => flip(displayName(a).localeCompare(displayName(b), 'ro')));
  }

  let list = all;
  if (q) {
    const r = await searchAnime(fetch, q, { genre: genre || undefined });
    list = r.data;
    // search endpoint has no sort param — apply the selected sort here.
    // Comparators stay in each sort's natural direction and `flip` reverses
    // them, so this path can't drift from the API's ordering.
    const flip = (n: number) => (dir === naturalDir ? n : -n);
    if (sort === 'title') {
      list = list
        .slice()
        .sort((a, b) => flip(displayName(a).localeCompare(displayName(b), 'ro')));
    } else if (sort === 'year') {
      list = list.slice().sort((a, b) => flip((b.year ?? 0) - (a.year ?? 0)));
    } else if (sort === 'score') {
      list = list.slice().sort((a, b) => flip((b.score ?? 0) - (a.score ?? 0)));
    }
  } else if (genre) {
    list = all.filter((a) => a.genres?.includes(genre));
  }
  // season/year/studio narrow whatever the base list is (we have the data — use it)
  if (year) list = list.filter((a) => a.year === Number(year));
  if (season) list = list.filter((a) => a.season?.toLowerCase() === season);
  if (studio) list = list.filter((a) => a.studios?.includes(studio));

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
  for (const a of all) for (const g of a.genres ?? []) counts.set(g, (counts.get(g) ?? 0) + 1);
  const genres = Array.from(counts.keys()).sort();
  const genresTop = Array.from(counts.entries())
    .sort((x, y) => y[1] - x[1] || x[0].localeCompare(y[0]))
    .slice(0, 10)
    .map(([g]) => g)
    .sort();

  // distinct years, newest first, for the year filter
  const years = Array.from(new Set(all.map((a) => a.year).filter((y): y is number => !!y))).sort(
    (a, b) => b - a
  );

  // distinct studios, A–Z, for the studio filter
  const studios = Array.from(new Set(all.flatMap((a) => a.studios ?? []))).sort((a, b) =>
    a.localeCompare(b)
  );

  // Catalog recommendation (matches the manga library banner); hidden
  // while searching or filtering so results stay uncluttered. The
  // coordinator's pick wins when the banner is shown at all.
  const showFeatured = !q && !genre && !year && !season && !studio && page === 1;
  const featured = showFeatured
    ? (curatedAnime(await curatedSlots(fetch), 'anime_featured') ??
      all.find((a) => a.imageUrl && a.synopsis && a.synopsis.length > 80) ??
      null)
    : null;

  return { dir, list, count, page, pages, genres, genresTop, q, genre, year, season, studio, years, studios, sort, view, total: all.length, featured };
};
