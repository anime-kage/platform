import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { getPublicUser, getFollowList } from '$lib/server/api';
import { MEMBERS } from '$lib/data/community';
import type { FollowUser } from '$shared/types';

export const load: PageServerLoad = async ({ params, fetch }) => {
  const handle = params.username;

  const real = await getPublicUser(fetch, handle);
  if (real) {
    const rows = (await getFollowList(fetch, handle, 'following')) ?? [];
    return { kind: 'real' as const, handle, name: real.user.username, rows };
  }

  const member = MEMBERS.find((m) => m.id === handle.toLowerCase());
  if (!member) throw error(404, 'Utilizatorul nu a fost găsit');
  return { kind: 'seed' as const, handle, name: member.name, rows: [] as FollowUser[] };
};
