import { redirect } from '@sveltejs/kit';
import type { LayoutServerLoad } from './$types';
import { serverMe } from '$lib/server/api';

/**
 * Guests get the landing page and the auth flow, nothing else.
 *
 * The companion check in +layout.svelte does the same thing in the browser; this
 * one is the real gate, because it runs before a byte of the page is rendered.
 * Until the token was mirrored into a cookie the server had no way to know who
 * was asking, so the redirect could only happen after hydration — which meant
 * `curl` still got the full HTML. It no longer does.
 *
 * Exact paths, not prefixes: a new public route has to be added here on purpose
 * rather than inheriting access from a permitted parent.
 */
const GUEST_PATHS = new Set([
  '/',
  '/login',
  '/register',
  '/parola-uitata',
  '/reseteaza-parola'
]);

export const load: LayoutServerLoad = async ({ url, cookies, fetch }) => {
  if (GUEST_PATHS.has(url.pathname)) return {};

  // No cookie is the common case for a stranger — answer without troubling the
  // backend.
  if (!cookies.get('ak_token')) throw redirect(303, '/');

  // A cookie that no longer verifies is worse than none: every load below this
  // one would 401 and the member would land on an error page instead of the
  // front door. Resolve it here, and drop the dead cookie on the way out.
  const user = await serverMe(fetch);
  if (!user) {
    cookies.delete('ak_token', { path: '/' });
    throw redirect(303, '/');
  }

  return {};
};
