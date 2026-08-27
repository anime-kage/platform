import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { communityThread } from '$lib/server/api';

export const load: PageServerLoad = async ({ fetch, params }) => {
  const id = Number(params.id);
  if (!Number.isInteger(id) || id < 1) throw error(404, 'Subiect inexistent');
  const data = await communityThread(fetch, id);
  if (!data) throw error(404, 'Subiectul nu există');
  return { thread: data.thread, replies: data.replies };
};
