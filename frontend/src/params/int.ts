import type { ParamMatcher } from '@sveltejs/kit';

// numeric route params — lets /liste/[id=int] (real lists) win over
// /liste/[slug] (seeded community lists)
export const match: ParamMatcher = (param) => /^\d+$/.test(param);
