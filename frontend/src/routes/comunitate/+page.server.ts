import type { PageServerLoad } from './$types';
import { communityReviews, communityStats, communityTeam, communityForum } from '$lib/server/api';

// Public read data for the first paint (Reviews tab + sidebar stats + Team +
// Forum list). Members and friends-only Activity are viewer-specific, so the
// component fetches those client-side with the auth token. Each source falls
// back to empty so one slow/broken endpoint doesn't blank the whole page.
export const load: PageServerLoad = async ({ fetch }) => {
  const [reviews, stats, team, threads] = await Promise.all([
    communityReviews(fetch).catch(() => []),
    communityStats(fetch).catch(() => ({ members: 0, subtitles: 0, titles: 0, online: 0 })),
    communityTeam(fetch).catch(() => []),
    communityForum(fetch).catch(() => [])
  ]);
  return { reviews, stats, team, threads };
};
