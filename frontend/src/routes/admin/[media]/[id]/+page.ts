import { error } from '@sveltejs/kit';
import type { PageLoad } from './$types';

export const ssr = false;

export const load: PageLoad = ({ params }) => {
  if (params.media !== 'anime' && params.media !== 'manga') throw error(404, 'Not found');
  const id = Number(params.id);
  if (!Number.isInteger(id) || id <= 0) throw error(404, 'Not found');
  return { media: params.media as 'anime' | 'manga', id };
};
