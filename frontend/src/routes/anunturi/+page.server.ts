import type { PageServerLoad } from './$types';
import { announcements } from '$lib/server/api';

/** Every published post. The tag doubles as the filter, which is the one job
 *  it was carrying badly on the home strip — there it is decoration. */
export const load: PageServerLoad = async ({ fetch }) => {
  const posts = await announcements(fetch, 50);
  const tags = [...new Set(posts.map((p) => p.tag))].sort((a, b) => a.localeCompare(b, 'ro'));
  return { posts, tags };
};
