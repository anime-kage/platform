import type { PageServerLoad } from './$types';
import { error } from '@sveltejs/kit';
import { getPublicUser, getUserWatchlist, getUserReadlist } from '$lib/server/api';
import { MEMBERS } from '$lib/data/community';
import type { ReadlistEntry, WatchlistEntry } from '$shared/types';

export const load: PageServerLoad = async ({ params, fetch }) => {
  const handle = params.username;

  const real = await getPublicUser(fetch, handle);
  const member = MEMBERS.find((m) => m.id === handle.toLowerCase());

  if (real) {
    const [planAnime, planManga] = await Promise.all([
      getUserWatchlist(fetch, handle, 'plan-to-watch').then((es) => (es ?? []).filter((e) => e.anime)),
      getUserReadlist(fetch, handle, 'plan-to-read').then((es) => (es ?? []).filter((e) => e.manga))
    ]);
    return { handle, name: member?.name ?? real.user.username, planAnime, planManga };
  }

  if (!member) throw error(404, 'Utilizatorul nu a fost găsit');
  return { handle, name: member.name, planAnime: [] as WatchlistEntry[], planManga: [] as ReadlistEntry[] };
};
