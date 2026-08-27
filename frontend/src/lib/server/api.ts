import { env } from '$env/dynamic/private';
import type {
  Anime,
  Chapter,
  Episode,
  FollowNetwork,
  FollowUser,
  Manga,
  ReadlistEntry,
  Review,
  User,
  UserList,
  UserReview,
  UserStats,
  WatchlistEntry,
  CommunityReview,
  CommunityStats,
  CommunityTeamMember,
  ForumThread,
  ForumReply,
  ProfileBanner,
  HistoryDay,
  ActivityEvent,
  Announcement,
  AnimeRelations,
  ScheduleSlot
} from '$shared/types';

/**
 * Server-side API client for SSR load functions.
 * Inside docker the backend is reached via the service name (API_URL=http://backend:3000);
 * outside docker it falls back to localhost. Always pass SvelteKit's `event.fetch`.
 */
const BASE = (env.API_URL || 'http://localhost:3000').replace(/\/$/, '');

type Fetch = typeof globalThis.fetch;

export type Paginated<T> = {
  data: T[];
  pagination: { page: number; limit: number; total: number | string; totalPages: number };
};

async function get<T>(f: Fetch, path: string): Promise<T> {
  const res = await f(`${BASE}${path}`);
  if (!res.ok) throw new Error(`API ${path} -> ${res.status}`);
  return res.json() as Promise<T>;
}

/** pg `numeric` serializes as a string — normalize once at the boundary. */
const num = (v: unknown): number | undefined => (v == null ? undefined : Number(v));
const normAnime = (a: Anime): Anime => ({ ...a, score: num(a.score) });
const normManga = (m: Manga): Manga => ({ ...m, score: num(m.score) });

export async function listAnime(
  f: Fetch,
  params: { page?: number; limit?: number; sort?: string; dir?: string; genre?: string } = {}
): Promise<Paginated<Anime>> {
  const p = new URLSearchParams();
  if (params.page) p.set('page', String(params.page));
  p.set('limit', String(params.limit ?? 50));
  if (params.sort) p.set('sort', params.sort);
  if (params.dir) p.set('dir', params.dir);
  if (params.genre) p.set('genres', params.genre);
  const r = await get<Paginated<Anime>>(f, `/api/anime?${p}`);
  return { ...r, data: r.data.map(normAnime) };
}

/**
 * Fetch the entire catalog (API caps limit at 50, so page through).
 * Fine at the current scale; push filters down to the API once the
 * catalog outgrows a few hundred titles.
 */
/** Newest public custom lists with content — the /liste browse feed. */
export async function listPublicUserLists(f: Fetch): Promise<UserList[]> {
  try {
    return (await get<{ data: UserList[] }>(f, '/api/lists')).data;
  } catch {
    return []; // the seeded lists still render without the feed
  }
}

/** Hard ceiling on how many pages we will ever walk (50 per page). A bound is
 *  still wanted so a runaway catalog cannot turn one page view into thousands
 *  of backend calls — but it has to be far above the real catalog size. */
const MAX_PAGES = 100;
/** How long a fetched catalog stays warm. Walking ~40 pages per page view is
 *  what the old 6-page cap was really avoiding; caching removes the need to
 *  cap at all. adapter-node is a single long-lived process, so a module-level
 *  cache is shared by every request it serves. */
const CATALOG_TTL_MS = 5 * 60 * 1000;
const catalogCache = new Map<string, { at: number; data: Anime[] }>();

/**
 * The whole anime catalog.
 *
 * Was capped at 6 pages — 300 titles — which was invisible while the catalog
 * was small and silently truncated it once a bulk import pushed it past that.
 * The catalog page derives its genre chips, year and studio filters and its
 * result count from this list, so a truncated list did not just hide titles,
 * it hid whole genres and years from the filter rows.
 */
/**
 * Memoise a site-wide fetch for a few seconds, single-flighted.
 *
 * The member dashboard fans out to a dozen endpoints, none of which are
 * per-user: leaderboards, curated slots, the forum, the schedule, news. Served
 * uncached that was 13 API calls for every page view, which is what made
 * `/home` the page that exhausted the rate limiter first.
 *
 * Only ever wrap responses that are the same for everyone. Anything that
 * varies by viewer (watchlists, notifications, drafts) must not come through
 * here — the whole point is that one member's copy is served to the next.
 */
const memoCache = new Map<string, { at: number; data: unknown }>();
const memoInflight = new Map<string, Promise<unknown>>();

export async function memo<T>(key: string, ttlMs: number, fn: () => Promise<T>): Promise<T> {
  const hit = memoCache.get(key);
  if (hit && Date.now() - hit.at < ttlMs) return hit.data as T;

  const running = memoInflight.get(key);
  if (running) return running as Promise<T>;

  const p = fn()
    .then((data) => {
      memoCache.set(key, { at: Date.now(), data });
      return data;
    })
    .catch((err) => {
      // Serving a slightly stale dashboard beats failing it.
      if (hit) return hit.data as T;
      throw err;
    })
    .finally(() => memoInflight.delete(key));

  memoInflight.set(key, p as Promise<unknown>);
  return p as Promise<T>;
}

/** In-flight walks, keyed like the cache. Without this, N concurrent page
 *  loads each start their own ~40-request walk — which is precisely how the
 *  first uncapped deploy tripped the backend's per-IP rate limit (the whole
 *  frontend container is one IP) and turned catalog pages into 500s. */
const catalogInflight = new Map<string, Promise<Anime[]>>();

/** One page, retried through a rate-limit rejection.
 *  The walk is ~40 calls from a single IP (the whole frontend container shares
 *  one), against a per-IP budget it also shares with every real visitor. A 429
 *  partway through should cost a pause, not the entire page. */
async function pageWithRetry(f: Fetch, page: number, sort: string, dir?: string) {
  for (let attempt = 0; ; attempt++) {
    try {
      return await listAnime(f, { page, limit: 50, sort, dir });
    } catch (err) {
      if (attempt >= 3 || !String(err).includes('429')) throw err;
      await new Promise((r) => setTimeout(r, 400 * 2 ** attempt));
    }
  }
}

async function walkCatalog(f: Fetch, sort: string, dir: string | undefined, key: string) {
  const first = await pageWithRetry(f, 1, sort, dir);
  const out = [...first.data];
  const total = Number(first.pagination.total ?? out.length);
  const pages = Math.min(Math.ceil(total / 50) || 1, MAX_PAGES);
  for (let page = 2; page <= pages; page++) {
    const next = await pageWithRetry(f, page, sort, dir);
    out.push(...next.data);
    if (!next.data.length) break;
  }
  catalogCache.set(key, { at: Date.now(), data: out });
  return out;
}

/** Sort orders the catalog page offers, applied in memory so every sort shares
 *  one cached fetch instead of each triggering its own 40-call walk. */
function sortCatalog(list: Anime[], sort: string, dir?: string): Anime[] {
  const desc = dir ? dir === 'desc' : sort !== 'title';
  const f = (n: number) => (desc ? -n : n);
  const by: Record<string, (a: Anime, b: Anime) => number> = {
    score: (a, b) => f((a.score ?? -1) - (b.score ?? -1)),
    year: (a, b) => f((a.year ?? -1) - (b.year ?? -1)),
    title: (a, b) => f(a.title.localeCompare(b.title, 'ro')),
    recent: (a, b) => f(Number(new Date(a.createdAt ?? 0)) - Number(new Date(b.createdAt ?? 0)))
  };
  return [...list].sort(by[sort] ?? by.score);
}

/**
 * The whole anime catalog.
 *
 * Was capped at 6 pages — 300 titles — which silently truncated the catalog
 * once a bulk import pushed it past that. The catalog page derives its genre
 * chips, year and studio filters and its result count from this list, so the
 * cap hid whole genres and years, not just titles.
 *
 * Removing the cap alone was not enough: it turned one page view into ~40
 * backend calls, and concurrent views multiplied that into rate-limit 429s.
 * Hence the cache, the single-flight guard, and serving stale data rather than
 * failing a page when a refresh cannot complete.
 */
export async function listAllAnime(f: Fetch, sort = 'score', dir?: string): Promise<Anime[]> {
  // One cache entry for every sort. Each distinct sort used to trigger its own
  // ~40-call walk, so simply browsing the sort dropdown drained the per-IP rate
  // budget and turned catalog pages into 500s. Fetch once, order here.
  const key = '__catalog__';
  const hit = catalogCache.get(key);
  if (hit && Date.now() - hit.at < CATALOG_TTL_MS) return sortCatalog(hit.data, sort, dir);

  const running = catalogInflight.get(key);
  if (running) return sortCatalog(await running, sort, dir);

  const p = walkCatalog(f, 'score', 'desc', key).catch((err) => {
    // A stale catalog beats a 500. The next request retries; meanwhile the
    // page renders with data that is at most a few minutes old.
    if (hit) {
      console.warn(`catalog refresh failed (${key}), serving stale:`, err);
      return hit.data;
    }
    throw err;
  }).finally(() => catalogInflight.delete(key));

  catalogInflight.set(key, p);
  return sortCatalog(await p, sort, dir);
}

export type PublicUser = {
  user: Omit<User, 'email'>;
  stats: UserStats;
  network: FollowNetwork;
  /** the series they chose as a backdrop (PLAN 8.17); null for most members */
  banner: ProfileBanner | null;
};

/** Public profile by username; null when the account doesn't exist.
    Pass `token` (viewer's JWT) so `network.isFollowing` reflects the viewer. */
export async function getPublicUser(f: Fetch, username: string, token?: string): Promise<PublicUser | null> {
  const res = await f(`${BASE}/api/users/${encodeURIComponent(username)}`, {
    headers: token ? { Authorization: `Bearer ${token}` } : undefined
  });
  if (!res.ok) return null;
  return (await res.json()) as PublicUser;
}

/** Followers/following of a user; token personalizes per-row `isFollowing`. */
export async function getFollowList(
  f: Fetch,
  username: string,
  kind: 'followers' | 'following',
  token?: string
): Promise<FollowUser[] | null> {
  const res = await f(`${BASE}/api/users/${encodeURIComponent(username)}/${kind}`, {
    headers: token ? { Authorization: `Bearer ${token}` } : undefined
  });
  if (!res.ok) return null;
  return ((await res.json()) as { data: FollowUser[] }).data;
}

/** A user's public watchlist (Letterboxd-style: lists are visible to anyone). */
export async function getUserWatchlist(
  f: Fetch,
  username: string,
  status?: string
): Promise<WatchlistEntry[] | null> {
  const q = status ? `?status=${encodeURIComponent(status)}` : '';
  const res = await f(`${BASE}/api/users/${encodeURIComponent(username)}/watchlist${q}`);
  if (!res.ok) return null;
  const r = (await res.json()) as { data: WatchlistEntry[] };
  return r.data.map((e) => ({ ...e, anime: e.anime ? normAnime(e.anime) : e.anime }));
}

/** A user's public readlist. */
export async function getUserReadlist(
  f: Fetch,
  username: string,
  status?: string
): Promise<ReadlistEntry[] | null> {
  const q = status ? `?status=${encodeURIComponent(status)}` : '';
  const res = await f(`${BASE}/api/users/${encodeURIComponent(username)}/readlist${q}`);
  if (!res.ok) return null;
  const r = (await res.json()) as { data: ReadlistEntry[] };
  return r.data.map((e) => ({ ...e, manga: e.manga ? normManga(e.manga) : e.manga }));
}

/** A user's watch/read activity — public, like their lists and reviews. */
export async function getUserHistory(f: Fetch, username: string, days = 14): Promise<HistoryDay[]> {
  const res = await f(`${BASE}/api/users/${encodeURIComponent(username)}/history?days=${days}`);
  if (!res.ok) return [];
  return ((await res.json()) as { data: HistoryDay[] }).data;
}

/** All reviews written by a user (anime + manga entries with notes). */
export async function getUserReviews(f: Fetch, username: string): Promise<UserReview[] | null> {
  const res = await f(`${BASE}/api/users/${encodeURIComponent(username)}/reviews`);
  if (!res.ok) return null;
  return ((await res.json()) as { data: UserReview[] }).data;
}

export type RankedAnime = Anime & { points: number };
export type LeaderWindow = 'today' | 'month' | 'all';

/**
 * Leaderboard for a time window. "today"/"month" rank by episodes watched in
 * that window (watch_history); "all" ranks by total trackers. `points` is the
 * window-appropriate count to display.
 */
export async function leaderboardAnime(
  f: Fetch,
  window: LeaderWindow,
  limit = 8
): Promise<RankedAnime[]> {
  const r = await get<{ data: RankedAnime[] }>(
    f,
    `/api/anime/most-watched?window=${window}&limit=${limit}`
  );
  return r.data.map((a) => ({ ...normAnime(a), points: a.points ?? 0 }));
}

export type PublishedRelease = {
  id: number;
  medium: 'anime' | 'manga';
  episodeNumber?: number;
  chapterNumber?: string;
  /** Absent on `upcoming` entries — nothing has been published for those yet. */
  publishedAt?: string;
  anime?: Anime;
  manga?: Manga;
  /** Filler card: an airing series with no release yet, used to top the home
   *  shelf up to a full row. Links to the series and shows no episode ribbon,
   *  because claiming an episode number that does not exist is worse than an
   *  empty shelf slot. */
  upcoming?: true;
};

/** Latest published RO-subtitle releases (anime + manga) — the homepage feed. */
export async function recentReleases(f: Fetch, limit = 12): Promise<PublishedRelease[]> {
  const r = await get<{ data: PublishedRelease[] }>(f, `/api/recent-releases?limit=${limit}`);
  return r.data
    .filter((x) => (x.anime ?? x.manga)?.imageUrl)
    .map((x) => ({
      ...x,
      anime: x.anime ? normAnime(x.anime) : undefined,
      manga: x.manga ? normManga(x.manga) : undefined
    }));
}

export async function searchAnime(
  f: Fetch,
  q: string,
  params: { page?: number; limit?: number; genre?: string } = {}
): Promise<Paginated<Anime>> {
  const p = new URLSearchParams({ q });
  if (params.page) p.set('page', String(params.page));
  p.set('limit', String(params.limit ?? 50));
  if (params.genre) p.set('genres', params.genre);
  const r = await get<Paginated<Anime>>(f, `/api/anime/search?${p}`);
  return { ...r, data: r.data.map(normAnime) };
}

/** By numeric id or by slug — the API resolves either. */
export async function getAnime(f: Fetch, idOrSlug: number | string): Promise<Anime | null> {
  try {
    const r = await get<{ data: Anime }>(f, `/api/anime/${encodeURIComponent(String(idOrSlug))}`);
    return normAnime(r.data);
  } catch {
    return null;
  }
}

/** One review (watchlist/readlist entry with notes) by its entry id. */
export async function getReview(
  f: Fetch,
  kind: 'anime' | 'manga',
  titleId: number,
  entryId: number
): Promise<Review | null> {
  try {
    const r = await get<{ data: Review[] }>(f, `/api/${kind}/${titleId}/reviews`);
    return r.data.find((rev) => rev.entryId === entryId) ?? null;
  } catch {
    return null;
  }
}

export async function getEpisodes(f: Fetch, animeId: number): Promise<Episode[]> {
  try {
    const r = await get<{ data: Episode[] }>(f, `/api/anime/${animeId}/episodes`);
    return r.data;
  } catch {
    return [];
  }
}

export async function listManga(
  f: Fetch,
  params: { page?: number; limit?: number; sort?: string; dir?: string; genre?: string } = {}
): Promise<Paginated<Manga>> {
  const p = new URLSearchParams();
  if (params.page) p.set('page', String(params.page));
  p.set('limit', String(params.limit ?? 50));
  if (params.sort) p.set('sort', params.sort);
  if (params.dir) p.set('dir', params.dir);
  if (params.genre) p.set('genres', params.genre);
  const r = await get<Paginated<Manga>>(f, `/api/manga?${p}`);
  return { ...r, data: r.data.map(normManga) };
}

/**
 * Manga search. The backend has had `/api/manga/search` all along; nothing on
 * the SSR side ever called it, which is why site search was anime-only.
 */
export async function searchManga(
  f: Fetch,
  q: string,
  params: { page?: number; limit?: number; genre?: string } = {}
): Promise<Paginated<Manga>> {
  const p = new URLSearchParams({ q });
  if (params.page) p.set('page', String(params.page));
  p.set('limit', String(params.limit ?? 50));
  if (params.genre) p.set('genres', params.genre);
  const r = await get<Paginated<Manga>>(f, `/api/manga/search?${p}`);
  return { ...r, data: r.data.map(normManga) };
}

/** By numeric id or by slug — the API resolves either. */
export async function getManga(f: Fetch, idOrSlug: number | string): Promise<Manga | null> {
  try {
    const r = await get<{ data: Manga }>(f, `/api/manga/${encodeURIComponent(String(idOrSlug))}`);
    return normManga(r.data);
  } catch {
    return null;
  }
}

export async function getChapters(f: Fetch, mangaId: number): Promise<Chapter[]> {
  try {
    const r = await get<{ data: Chapter[] }>(f, `/api/manga/${mangaId}/chapters`);
    return r.data;
  } catch {
    return [];
  }
}

export async function getChapter(
  f: Fetch,
  mangaId: number,
  num: string
): Promise<Chapter | null> {
  try {
    const r = await get<{ data: Chapter }>(f, `/api/manga/${mangaId}/chapters/${num}`);
    return r.data;
  } catch {
    return null;
  }
}

export async function listAllManga(f: Fetch, sort = 'score', dir?: string): Promise<Manga[]> {
  const first = await listManga(f, { page: 1, limit: 50, sort, dir });
  const out = [...first.data];
  const pages = Math.ceil(Number(first.pagination.total ?? out.length) / 50);
  for (let page = 2; page <= Math.min(pages, 6); page++) {
    const next = await listManga(f, { page, limit: 50, sort, dir });
    out.push(...next.data);
    if (!next.data.length) break;
  }
  return out;
}

// ── Community (public read data for SSR) ────────────────────────────────────
export async function communityReviews(f: Fetch): Promise<CommunityReview[]> {
  const r = await get<{ data: CommunityReview[] }>(f, '/api/community/reviews');
  return r.data;
}
export async function communityStats(f: Fetch): Promise<CommunityStats> {
  return get<CommunityStats>(f, '/api/community/stats');
}
export async function communityTeam(f: Fetch): Promise<CommunityTeamMember[]> {
  const r = await get<{ data: CommunityTeamMember[] }>(f, '/api/community/team');
  return r.data;
}
export async function communityForum(f: Fetch): Promise<ForumThread[]> {
  const r = await get<{ data: ForumThread[] }>(f, '/api/community/forum');
  return r.data;
}

/**
 * The activity feed. `site` is the whole community (what /home shows), `friends`
 * is mutual follows only (the /comunitate tab).
 *
 * Never throws: /home renders five independent strips and one of them failing
 * must cost that strip, not the page.
 */
export async function communityActivity(
  f: Fetch,
  scope: 'site' | 'friends' = 'site'
): Promise<ActivityEvent[]> {
  try {
    return (await get<{ data: ActivityEvent[] }>(f, `/api/community/activity?scope=${scope}`)).data;
  } catch {
    return [];
  }
}

/**
 * A series' neighbours: the season chain and the franchise grid.
 *
 * Never throws — relations are an enhancement to a detail page, and a title
 * with none is the common case anyway, so a failure degrades to "no strips"
 * rather than taking the page down.
 */
export async function animeRelations(f: Fetch, id: number): Promise<AnimeRelations> {
  try {
    return (await get<{ data: AnimeRelations }>(f, `/api/anime/${id}/relations`)).data;
  } catch {
    return { chain: [], related: [] };
  }
}

/**
 * The team-decided programme for the next `days` days ("Programul săptămânii").
 * Never throws — an empty week and a failed call render the same empty state.
 */
export async function schedule(f: Fetch, days = 7): Promise<ScheduleSlot[]> {
  try {
    return (await get<{ data: ScheduleSlot[] }>(f, `/api/schedule?days=${days}`)).data;
  } catch {
    return [];
  }
}

/** One post by slug or id; null when it doesn't exist (or is a draft). */
export async function announcement(f: Fetch, idOrSlug: string): Promise<Announcement | null> {
  try {
    return (await get<{ data: Announcement }>(f, `/api/announcements/${encodeURIComponent(idOrSlug)}`)).data;
  } catch {
    return null;
  }
}

/** Published announcements, newest first — the "Știri & anunțuri" strip. */
export async function announcements(f: Fetch, limit = 4): Promise<Announcement[]> {
  try {
    return (await get<{ data: Announcement[] }>(f, `/api/announcements?limit=${limit}`)).data;
  } catch {
    return [];
  }
}

export async function communityThread(
  f: Fetch,
  id: number
): Promise<{ thread: ForumThread; replies: ForumReply[] } | null> {
  try {
    const r = await get<{ data: { thread: ForumThread; replies: ForumReply[] } }>(f, `/api/community/forum/${id}`);
    return r.data;
  } catch {
    return null;
  }
}

// ── Curated placements ────────────────────────────────────────────

export type CuratedPick = {
  position: number;
  anime?: Anime;
  manga?: Manga;
  /** Set for slots that feature a member's list rather than a title. */
  list?: UserList;
  imageUrl?: string;
};
export type CuratedSlots = Record<string, CuratedPick[]>;

/**
 * All curated slots in one call. Never throws: an editor's picks are a nicety
 * and the front page must still render if this fails, so callers get an empty
 * map and fall back to their automatic choice.
 *
 * A per-placement `imageUrl` override is folded into the title's own
 * `imageUrl` here, once, rather than in each of the four templates. The
 * override exists because these blocks are wide and cinematic while a cover
 * is portrait; it is scoped to the placement and never written back to the
 * catalog, so every card and list still shows the real poster.
 */
export async function curatedSlots(f: Fetch): Promise<CuratedSlots> {
  try {
    const r = await get<{ data: CuratedSlots }>(f, '/api/curated');
    const out: CuratedSlots = {};
    for (const [slot, picks] of Object.entries(r.data ?? {})) {
      out[slot] = (picks ?? []).map((p) => ({
        ...p,
        anime: p.anime
          ? { ...normAnime(p.anime), imageUrl: p.imageUrl ?? p.anime.imageUrl }
          : undefined,
        manga: p.manga
          ? { ...normManga(p.manga), imageUrl: p.imageUrl ?? p.manga.imageUrl }
          : undefined
      }));
    }
    return out;
  } catch {
    return {};
  }
}

/** The anime in a single-pick slot, or null when nothing is curated. */
export function curatedAnime(slots: CuratedSlots, slot: string): Anime | null {
  return slots[slot]?.[0]?.anime ?? null;
}

/** The manga in a single-pick slot, or null when nothing is curated. */
export function curatedManga(slots: CuratedSlots, slot: string): Manga | null {
  return slots[slot]?.[0]?.manga ?? null;
}

/**
 * The signed-in member as the *server* sees them, or null.
 *
 * Relies on hooks.server.ts having turned the `ak_token` cookie into an
 * Authorization header, so this answers the one question SSR previously could
 * not: is whoever asked for this page logged in? Used by the root layout to turn
 * guests away before a page renders instead of after it hydrates.
 */
export async function serverMe(f: Fetch): Promise<User | null> {
  try {
    const r = await get<{ user: User }>(f, '/api/auth/me');
    return r.user ?? null;
  } catch {
    // Any non-200 — no cookie, expired token, backend down — is "not signed in".
    return null;
  }
}

/** Just enough of a title to draw a collage card: a key, a cover, and the
 *  fields displayName() falls through. */
export type LandingCollageItem = Pick<
  Anime,
  'id' | 'title' | 'titleEnglish' | 'titleRomanian' | 'imageUrl'
>;

/**
 * The landing page's collage, chosen by the API.
 *
 * This is the only catalog read that works without a session. The landing page
 * used to build the collage itself from `/api/anime` + `/api/curated`, which is
 * precisely why those two had to stay open to anyone; the selection now lives in
 * the backend's `landing` handler so both could be closed.
 */
export async function landingCollage(f: Fetch): Promise<LandingCollageItem[]> {
  try {
    const r = await get<{ data: { collage: LandingCollageItem[] } }>(f, '/api/landing');
    return r.data?.collage ?? [];
  } catch {
    // The front door renders without art rather than 500ing at a stranger.
    return [];
  }
}
