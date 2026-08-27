import type { PageServerLoad } from './$types';
import { landingCollage } from '$lib/server/api';

export const load: PageServerLoad = async ({ fetch }) => {
  // One public call. The "coordinator's picks win, top-scored covers fill the
  // rest" selection this used to do here now lives in the backend's /api/landing
  // handler, because /api/anime and /api/curated both require a session and this
  // page is what a stranger sees.
  //
  // The layout still has exactly three positions and the API tops the list up to
  // fill them; an empty catalog returns an empty list and the collage simply
  // does not draw.
  const collage = await landingCollage(fetch);
  return { collage };
};
