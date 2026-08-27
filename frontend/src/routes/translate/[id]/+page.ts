import type { PageLoad } from './$types';

// Auth lives in localStorage — this page is client-only.
export const ssr = false;

export const load: PageLoad = ({ params }) => ({ releaseId: Number(params.id) });
