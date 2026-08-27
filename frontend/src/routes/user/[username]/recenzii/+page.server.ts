import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { getPublicUser, getUserReviews, listAnime } from '$lib/server/api';
import { MEMBERS, REVIEW_SEEDS, seedHash } from '$lib/data/community';
import type { Anime } from '$lib/types';
import type { UserReview } from '$shared/types';

export type SeedReview = { rating: number; date: string; likes: number; text: string; anime: Anime };

export const load: PageServerLoad = async ({ params, fetch }) => {
  const handle = params.username;

  const real = await getPublicUser(fetch, handle);
  if (real) {
    const reviews = (await getUserReviews(fetch, handle)) ?? [];
    return {
      kind: 'real' as const,
      handle,
      name: real.user.username,
      reviews: [] as SeedReview[],
      userReviews: reviews
    };
  }

  const member = MEMBERS.find((m) => m.id === handle.toLowerCase());
  if (!member) throw error(404, 'Utilizatorul nu a fost găsit');

  const catalog = (await listAnime(fetch, { limit: 50, sort: 'score' })).data.filter((a) => a.imageUrl);
  const pick = (key: string) => catalog[seedHash(member.id + key) % Math.max(1, catalog.length)];

  // Demo profile: their seeded review first, a few others fill the page
  const reviews: SeedReview[] = REVIEW_SEEDS.filter((r) => r.memberId === member.id)
    .concat(REVIEW_SEEDS.filter((r) => r.memberId !== member.id))
    .slice(0, 5)
    .map((r, i) => ({ rating: r.rating, date: r.date, likes: r.likes, text: r.text, anime: pick(`rev${i}`) }))
    .filter((r) => r.anime);

  return { kind: 'seed' as const, handle, name: member.name, reviews, userReviews: [] as UserReview[] };
};
