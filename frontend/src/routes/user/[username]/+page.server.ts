import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import {
  getPublicUser,
  getAnime,
  getManga,
  getUserReviews,
  getUserWatchlist,
  getUserReadlist,
  getUserHistory,
  listAnime,
  listPublicUserLists
} from '$lib/server/api';
import { MEMBERS, REVIEW_SEEDS, seedHash, seedActivity, type Member } from '$lib/data/community';
import type { Anime, Manga } from '$lib/types';
import type { UserReview, UserStats, HistoryDay, WatchlistEntry, ReadlistEntry } from '$shared/types';

export type ProfileFavorite = { type: 'anime' | 'manga'; item: Anime | Manga };
export type WatchlistPeek = { kind: 'anime' | 'manga'; item: Anime | Manga; addedAt: Date | string };
export type ProfileReview = { rating: number; date: string; likes: number; text: string; anime: Anime };

/** Demo showcase for a community member: favorites, reviews, lists, activity. */
async function seedShowcase(f: typeof globalThis.fetch, member: Member) {
  const catalog = (await listAnime(f, { limit: 50, sort: 'score' })).data.filter((a) => a.imageUrl);
  const pick = (key: string) => catalog[seedHash(member.id + key) % Math.max(1, catalog.length)];

  const favorites: ProfileFavorite[] = [];
  for (let i = 0; favorites.length < Math.min(4, catalog.length) && i < 20; i++) {
    const a = pick(`fav${i}`);
    if (a && !favorites.some((fav) => fav.item.id === a.id)) favorites.push({ type: 'anime', item: a });
  }

  const reviews: ProfileReview[] = REVIEW_SEEDS.filter((r) => r.memberId === member.id)
    .concat(REVIEW_SEEDS.filter((r) => r.memberId !== member.id).slice(0, 2))
    .slice(0, 3)
    .map((r, i) => ({ rating: r.rating, date: r.date, likes: r.likes, text: r.text, anime: pick(`rev${i}`) }))
    .filter((r) => r.anime);

  // Deliberately no lists here. This showcase used to invent genre lists per
  // member, so every profile displayed lists nobody had made. Real lists are
  // loaded from the API against the real account id, below.
  return { favorites, reviews, lists: [], activity: seedActivity(member.id) };
}

/**
 * Public profile. Community members (Maria, Andrei, …) are REAL accounts —
 * follows against them persist — but their showcase (favorites, reviews,
 * lists, activity) stays demo-seeded until they have real content.
 */
export const load: PageServerLoad = async ({ params, fetch }) => {
  const handle = params.username;
  const member = MEMBERS.find((m) => m.id === handle.toLowerCase());

  const real = await getPublicUser(fetch, handle);
  if (real) {
    const [favorites, userReviews, planAnime, planManga, allAnime, allManga, history] = await Promise.all([
      Promise.all(
        (real.user.favorites ?? []).slice(0, 5).map(async (r): Promise<ProfileFavorite | null> => {
          const item = r.type === 'anime' ? await getAnime(fetch, r.id) : await getManga(fetch, r.id);
          return item ? { type: r.type, item } : null;
        })
      ).then((xs) => xs.filter((x): x is ProfileFavorite => !!x)),
      getUserReviews(fetch, handle).then((rs) => rs ?? []),
      getUserWatchlist(fetch, handle, 'plan-to-watch').then((es) => es ?? []),
      getUserReadlist(fetch, handle, 'plan-to-read').then((es) => es ?? []),
      getUserWatchlist(fetch, handle).then((es) => es ?? []),
      getUserReadlist(fetch, handle).then((es) => es ?? []),
      getUserHistory(fetch, handle, 14)
    ]);
    const ratedCount =
      allAnime.filter((e) => e.score).length + allManga.filter((e) => e.score).length;

    // public watchlist peek: newest plan-to-watch/plan-to-read titles
    const watchlistPeek: WatchlistPeek[] = [
      ...planAnime
        .filter((e) => e.anime)
        .map((e) => ({ kind: 'anime' as const, item: e.anime as Anime, addedAt: e.updatedAt })),
      ...planManga
        .filter((e) => e.manga)
        .map((e) => ({ kind: 'manga' as const, item: e.manga as Manga, addedAt: e.updatedAt }))
    ]
      .sort((a, b) => new Date(b.addedAt).getTime() - new Date(a.addedAt).getTime())
      .slice(0, 5);
    const watchlistCount = planAnime.length + planManga.length;

    // demo dressing for member accounts that haven't picked their own yet
    const showcase =
      member && (!favorites.length || !userReviews.length) ? await seedShowcase(fetch, member) : null;

    // This member's real public lists. The public feed carries userId, so their
    // own are one filter away; nobody's profile shows a list they did not make.
    const ownLists = (await listPublicUserLists(fetch))
      .filter((l) => l.userId === real.user.id)
      .map((l) => ({
        slug: String(l.id),
        title: l.title,
        desc: l.description ?? '',
        count: l.itemCount,
        covers: l.covers.slice(0, 3)
      }));

    return {
      kind: 'real' as const,
      handle,
      profile: {
        name: member?.name ?? real.user.username,
        bio: real.user.bio ?? '',
        role: real.user.role,
        avatarUrl: real.user.avatarUrl ?? null,
        hue: real.user.avatarUrl ? null : (member?.hue ?? null),
        followers: real.network.followers,
        following: real.network.following,
        memberSince: real.user.createdAt ?? null,
        // their chosen series backdrop; null for most members
        banner: real.banner ?? null
      },
      stats: real.stats as UserStats,
      favorites: favorites.length ? favorites : (showcase?.favorites ?? []),
      reviews: userReviews.length ? [] : (showcase?.reviews ?? []),
      userReviews: userReviews.slice(0, 3),
      userReviewCount: userReviews.length,
      ratedCount,
      watchlistPeek,
      watchlistCount,
      // real 14-day activity, and the full lists behind the taste bars — the
      // same material /profile shows, so another member's page isn't a shell
      history,
      trackedAnime: allAnime,
      trackedManga: allManga,
      lists: ownLists,
      activity: showcase?.activity ?? null
    };
  }

  // Seeded community member without a real account (fallback)
  if (!member) throw error(404, 'Utilizatorul nu a fost găsit');

  const showcase = await seedShowcase(fetch, member);
  return {
    kind: 'seed' as const,
    handle,
    userReviews: [] as UserReview[],
    userReviewCount: 0,
    ratedCount: 0,
    watchlistPeek: [] as WatchlistPeek[],
    watchlistCount: 0,
    // seeded profiles have no real lists behind them; `activity` below is the
    // demo bar chart they show instead
    history: [] as HistoryDay[],
    trackedAnime: [] as WatchlistEntry[],
    trackedManga: [] as ReadlistEntry[],
    profile: {
      name: member.name,
      bio: member.bio,
      role: 'user' as const,
      avatarUrl: null,
      hue: member.hue,
      followers: member.followers,
      following: member.following,
      memberSince: null,
      // seeded showcase profiles have no real account to carry a backdrop
      banner: null
    },
    stats: null,
    favorites: showcase.favorites,
    reviews: showcase.reviews,
    lists: showcase.lists,
    activity: showcase.activity
  };
};
