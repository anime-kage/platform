import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

// Genre pages moved into the catalog's ?gen= filter.
export const load: PageServerLoad = ({ params }) => {
  throw redirect(301, `/anime?gen=${encodeURIComponent(params.genre)}`);
};
