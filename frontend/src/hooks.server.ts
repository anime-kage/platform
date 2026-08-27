import { env } from '$env/dynamic/private';
import type { Handle } from '@sveltejs/kit';

/** Same resolution as $lib/server/api — the backend over the docker network. */
const BASE = (env.API_URL || 'http://localhost:3000').replace(/\/$/, '');

/**
 * Gives SSR a credential.
 *
 * The catalog, community and profile endpoints require a session (invite-only,
 * July 2026). That is a problem for server-side rendering: the JWT lives in
 * localStorage, which exists only in the browser, so every `+page.server.ts`
 * load was calling the backend anonymously — fine while those routes were open,
 * a guaranteed 401 once they were not.
 *
 * `api.setToken` therefore mirrors the token into an `ak_token` cookie, and this
 * hook wraps `event.fetch` so any request aimed at the backend picks up an
 * Authorization header from it. Wrapping the fetch — rather than threading a
 * token parameter through ~40 helpers and 27 loads — is the whole point: loads
 * keep using the `fetch` SvelteKit hands them and neither they nor
 * `$lib/server/api` need to know this happens.
 *
 * Only requests to BASE are touched. An outbound call to anywhere else (an
 * external image, AniList) must not carry our bearer token.
 */
export const handle: Handle = async ({ event, resolve }) => {
	const token = event.cookies.get('ak_token');
	// Every SSR fetch is attributed to this container by the API's per-IP rate
	// limiter unless we say who it is really for. One shared bucket for the
	// whole site is not a limit anyone designed; it is an outage waiting for
	// enough visitors.
	let clientIP = '';
	try {
		clientIP = event.getClientAddress();
	} catch {
		// adapter-node throws when it cannot determine an address; a missing
		// header is not worth failing a page render over.
	}

	if (token || clientIP) {
		const inner = event.fetch;

		event.fetch = (input, init) => {
			const url =
				typeof input === 'string'
					? input
					: input instanceof URL
						? input.href
						: input.url;

			if (url.startsWith(BASE)) {
				// Merge rather than replace: a caller that set its own headers
				// (getPublicUser passes an explicit Authorization) keeps them,
				// and an explicit one wins over the cookie.
				const headers = new Headers(
					init?.headers ?? (input instanceof Request ? input.headers : undefined)
				);
				if (token && !headers.has('Authorization')) {
					headers.set('Authorization', `Bearer ${token}`);
				}
				// The API only believes this header from an address in
				// TRUSTED_PROXIES, which this container is; it cannot be forged
				// from outside.
				if (clientIP && !headers.has('X-Forwarded-For')) {
					headers.set('X-Forwarded-For', clientIP);
				}
				return inner(input, { ...init, headers });
			}

			return inner(input, init);
		};
	}

	return resolve(event);
};
