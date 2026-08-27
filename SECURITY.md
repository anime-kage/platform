# Security

## Reporting a vulnerability

**Please do not open a public issue.** A public report tells everyone how to
attack a live site with real accounts on it, before there is a fix.

Report privately instead, whichever is easier:

- GitHub → the **Security** tab → *Report a vulnerability* (private to maintainers)
- Discord — a direct message to an admin on the Anime-Kage server

Useful things to include, as far as you have them: what you did, what happened,
what you expected, and whether you needed an account. A rough description is
worth far more than nothing — do not wait until you have a polished write-up.

You will get an acknowledgement within a few days. This is a volunteer project,
so please be patient with the fix itself; anything touching accounts or member
data is treated as urgent.

## Scope

The platform holds real member accounts: password hashes, email addresses,
watch history and private lists. Anything that could expose those, let someone
act as another member, or take the site down is in scope.

Also in scope, and easy to overlook:

- credentials in the repository or in build output
- a way to reach the origin server without going through Cloudflare
- privilege escalation between roles (`user`, `translator`, `moderator`, `admin`)
- stored XSS anywhere user-written text is rendered — announcements, comments,
  reviews, chat, list descriptions

Out of scope: reports about third-party video hosts we link to, and findings
from automated scanners without a demonstrated impact.

## Please do not

- run scans or load tests against the live site
- access, modify or download another member's data
- leave proof-of-concept content on the public site

If you need to demonstrate something destructive, describe it and we will
reproduce it on staging.

## For contributors

Most vulnerabilities in a project like this arrive through ordinary changes,
not attacks. The three that matter most here:

**Never commit a secret.** `.env` is gitignored and so is everything shaped like
it. A secret pushed once is public forever — deleting it in the next commit does
not help, because the object stays in history. If it happens, say so immediately:
the fix is rotating the credential, not removing the file.

**Never render user text as HTML.** `announcements.body` is parsed into a token
tree and rendered as real elements, so no HTML string is ever produced and there
is no sanitiser to bypass. There is no `{@html}` in this codebase. Keep it that
way — see `frontend/src/lib/markdown.ts` and its tests.

**Never trust a client-supplied URL.** Links must be an internal path or
`https://`; images must be our own uploads or the allowlisted GIF CDN. Both
allowlists are tested; if a test in `markdown.test.ts` fails, that is the point
of the test.
