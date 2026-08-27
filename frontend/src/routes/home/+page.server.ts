import type { PageServerLoad } from './$types';
import {
  listAllAnime,
  listAllManga,
  leaderboardAnime,
  recentReleases,
  curatedSlots,
  curatedAnime,
  listPublicUserLists,
  communityForum,
  communityActivity,
  announcements,
  schedule as fetchSchedule,
  memo,
  type PublishedRelease
} from '$lib/server/api';
import { PLATFORM_LISTS, PLATFORM_LIST_SIZE, platformItems } from '$lib/data/platformLists';

/** How long the dashboard's site-wide strips stay warm. */
const HOME_TTL = 60_000;

/** How many cards the collections grid holds (2 × 2). */
const COLLECTION_SLOTS = 4;

export const load: PageServerLoad = async ({ fetch }) => {
  const [
    byScore,
    byScoreManga,
    latest,
    boardToday,
    boardMonth,
    boardAll,
    slots,
    memberLists,
    threads,
    activity,
    news,
    weekSlots
  ] = await Promise.all([
    // Every one of these is identical for every member, so they are memoised
    // site-wide rather than fetched per page view. HOME_TTL is short enough
    // that a new release or forum post shows up promptly and long enough that
    // a burst of visitors costs one round of calls, not one round each.
    listAllAnime(fetch, 'score').then((l) => l.filter((a) => a.imageUrl)),
    listAllManga(fetch, 'score').then((l) => l.filter((m) => m.imageUrl)),
    memo('home:releases', HOME_TTL, () => recentReleases(fetch, 12)),
    memo('home:board:today', HOME_TTL, () => leaderboardAnime(fetch, 'today', 8)),
    memo('home:board:month', HOME_TTL, () => leaderboardAnime(fetch, 'month', 8)),
    // 50, not 8: the sidebar shows the first eight and "Cele mai urmărite"
    // below needs the same ranking in full. One call, two consumers.
    memo('home:board:all', HOME_TTL, () => leaderboardAnime(fetch, 'all', PLATFORM_LIST_SIZE)),
    memo('home:curated', HOME_TTL, () => curatedSlots(fetch)),
    memo('home:lists', HOME_TTL, () => listPublicUserLists(fetch)),
    memo('home:forum', HOME_TTL, () => communityForum(fetch).catch(() => [])),
    memo('home:activity', HOME_TTL, () => communityActivity(fetch, 'site')),
    memo('home:news', HOME_TTL, () => announcements(fetch, 4)),
    memo('home:schedule', HOME_TTL, () => fetchSchedule(fetch, 7))
  ]);

  // "Ultimele lansări" lists episodes the team has actually published. There
  // are almost none yet, which left the shelf nearly empty on every member's
  // dashboard, so it is TOPPED UP with the newest airing series rather than
  // replaced by them: real releases always come first, and each new publish
  // pushes one filler card off the end. When there are SHELF_SIZE real
  // releases no filler is left and this stops having any effect.
  const SHELF_SIZE = 8;
  const already = new Set(latest.map((r) => `${r.medium}-${(r.anime ?? r.manga)?.id}`));
  // Recency, not airing status: "Sousou no Frieren 2nd Season" is year 2026 and
  // already `completed`, so an airing-only filter dropped exactly the kind of
  // just-finished season this shelf is meant to surface. `upcoming` is excluded
  // because nothing has been released for it yet.
  const filler = byScore
    .filter((a) => a.status !== 'upcoming' && !already.has(`anime-${a.id}`))
    .sort((a, b) => (b.year ?? 0) - (a.year ?? 0) || (b.score ?? 0) - (a.score ?? 0))
    .slice(0, Math.max(0, SHELF_SIZE - latest.length))
    .map((a): PublishedRelease => ({
      // Negative id so the {#each} key can never collide with a real release's.
      // Without an id every filler card keyed as "anime-undefined", which is a
      // duplicate key and takes the whole section down.
      id: -a.id,
      medium: 'anime' as const,
      anime: a,
      // No episode number: nothing has been published for these, so the card
      // links to the series and the ribbon shows the year/season instead of
      // claiming an episode that does not exist.
      upcoming: true as const
    }));
  const latestShelf = [...latest, ...filler];

  // Spotlight: the coordinator's pick, else the highest-scored title that has
  // a real synopsis to show.
  const spotlight =
    curatedAnime(slots, 'home_spotlight') ??
    byScore.find((a) => a.synopsis && a.synopsis.length > 80) ??
    byScore[0];

  // "Colecții" — members' own public lists first, then the platform's own
  // (Top 50 Anime, Top 50 Manga, cele mai urmărite, top pe gen) to fill the
  // grid. Both halves are real: the first comes from user_lists, the second is
  // computed from the catalog by the same code /liste uses, so a card here and
  // the page it opens can never disagree.
  const platformCollections = PLATFORM_LISTS.map((def) => {
    const items = platformItems(def, byScore, byScoreManga, boardAll);
    return {
      href: `/liste/top/${def.slug}`,
      title: def.title,
      curator: 'Anime-Kage',
      count: items.length,
      covers: items.slice(0, 3).map((a) => a.imageUrl as string)
    };
  }).filter((c) => c.covers.length >= 3);

  const memberCollections = memberLists
    .filter((l) => l.covers.length >= 3)
    .map((l) => ({
      href: `/liste/${l.id}`,
      title: l.title,
      curator: l.ownerName,
      count: l.itemCount,
      covers: l.covers.slice(0, 3)
    }));

  const collections = [...memberCollections, ...platformCollections].slice(0, COLLECTION_SLOTS);

  // Programul săptămânii — the team's own plan, from schedule_slots, capped at
  // what the 3-column grid holds. Nothing is derived or guessed: an empty
  // programme is a real answer and the strip says so.
  //
  // The day/date/time labels are formatted in the *browser* rather than here,
  // because `scheduledAt` is an instant and only the viewer knows their
  // timezone. This used to format a MAL weekday name server-side, which is
  // exactly why it could never say more than "23:30 JST".
  const schedule = weekSlots.slice(0, 6);

  return {
    spotlight,
    latest: latestShelf,
    collections,
    leaderboard: {
      today: boardToday.filter((a) => a.imageUrl),
      month: boardMonth.filter((a) => a.imageUrl),
      all: boardAll.filter((a) => a.imageUrl).slice(0, 8)
    },
    // the three community strips: real events, real threads, real announcements
    activity: activity.slice(0, 5),
    threads: threads.slice(0, 3),
    news,
    schedule
  };
};
