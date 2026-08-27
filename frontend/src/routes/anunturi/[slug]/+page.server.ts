import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { announcement, announcements } from '$lib/server/api';

export const load: PageServerLoad = async ({ params, fetch }) => {
  // slug or numeric id — same contract as /anime/[id]
  const post = await announcement(fetch, params.slug);
  if (!post) throw error(404, 'Anunțul nu există');

  // "citește și" — the newest few other posts, so the page is not a dead end
  const others = (await announcements(fetch, 6)).filter((p) => p.id !== post.id).slice(0, 3);
  return { post, others };
};
