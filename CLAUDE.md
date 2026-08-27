# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Anime-Kage is a Romanian-language anime and manga discovery and tracking
platform. The frontend, the Go API, the proxy config and the monitoring stack
all live in this repository.

## Development Commands

### Start everything
```bash
docker-compose -f anime-kage/docker-compose.dev.yml up
# Frontend: http://localhost:5173
# Backend:  http://localhost:3000
# DB:       PostgreSQL on localhost:5432
```

### Frontend (anime-kage/frontend/)
```bash
npm run dev          # Start Vite dev server
npm run build        # Production build
npm run check        # Type check with svelte-check
npm run check:watch  # Type check in watch mode
npm run check:browser # Parse every built file at the TV syntax floor (see vite.config.ts)
npm test             # Vitest, both projects, ~3s
npm run test:watch   # Re-run on save while writing a test
npm run test:unit    # Pure functions only — no DOM, well under a second
npm run test:ui      # Component tests only
```
**Node 22.** `.npmrc` sets `engine-strict=true` and the prod image is
`node:22-alpine`, so `npm install` refuses to run on other majors (Node 23
included). `nvm use 22` first.

### Backend — Go (anime-kage/backend-go/) ← the only backend
```bash
go run./cmd/api         # Run the API (needs DATABASE_URL + JWT_SECRET set, or a.env)
go run./cmd/migrate     # Apply SQL migrations (cmd/migrate/migrations/*.sql)
go run./cmd/populate    # Bulk-import anime from Jikan (-ids 5114,9253 for specific)
go run./cmd/autoupdate  # Sync: episodes | refresh | banners | relations | all (cron)
                         # `banners` pulls series art from AniList (MAL has none)
                         # `relations` pulls the season chain / franchise graph, also AniList
go run./cmd/autoupdate seasonal   # MANUAL ONLY — deliberately NOT in `all`
go run./cmd/autoupdate backfill   # MANUAL ONLY — episode titles/air dates/filler for the
                                   # WHOLE catalog, not just airing series (see below)
go run./cmd/autoupdate slugs      # fill URL slugs; in `all` too (no network, idempotent)
go build./...           # Compile everything
go vet./...             # Static checks
go test./...            # Unit tests (integration suite needs TEST_DATABASE_URL — see backend-go/README.md)
```

## Environments

Three, from one repository. The code is identical in all of them; only the
`.env` and the compose file differ. That is the point: what you approve on
staging is byte-for-byte what ships.

| | compose file | project | database | notes |
|---|---|---|---|---|
| dev | `docker-compose.dev.yml` | `anime-kage-dev` | `anime_kage_dev` | your machine, throwaway |
| staging | `docker-compose.staging.yml` | `anime-kage-staging` | `anime_kage_staging` | `staging.SITE_DOMAIN`, seeded, no SMTP, no Discord bot |
| production | `docker-compose.prod.yml` | `anime-kage` | `anime_kage` | real members |

**Every compose file pins `name:`, and removing it will take the site down.**
Compose derives the project name from the *directory*, all three files live in
one directory, so without an explicit name they are the same project. Running
the dev stack replaced the production containers and — because `--build` tags
images after the project — overwrote the production frontend image with the Vite
dev one. The container kept its name while serving `vite dev` on 5173 behind an
nginx that proxies 3000, so the site returned 502 with everything apparently
"running".

Staging service names are suffixed `-staging` for a related reason: Compose
registers a **service name** as a network alias on every network it joins, and
staging shares production's network so nginx can reach it. A plain `postgres`
there resolved round-robin between staging's database and production's; the only
thing that prevented a cross-connection was the passwords differing.

### Seed data

`scripts/seed-catalogue.sh {dev|staging}` fills a disposable database with the
real catalogue and **invented** members. It copies titles, episodes, source links
and the relation graph — public metadata that came from MAL and AniList — and
never copies users, watchlists, comments, chat, notifications, history, invites
or password resets. It also skips subtitles and releases: not personal, but the
team's actual work product.

It **erases the whole target database**, not just the catalogue: `users`
references `anime` and `manga` through the profile-banner columns, so
`TRUNCATE ... CASCADE` reaches `users` and everything beyond. That is what you
want from a seed, but it is worth knowing before running it.

Seeded accounts are `admin@example.test` and four others by role, password
`seed-password`.

### Deploying

Migrations are **baked into the image**. `--profile setup run --rm db-migrate`
without `--build` reports success while applying nothing, because the container
holds the migration files from the last build. Always pass `--build`.

nginx reads its config from a bind-mounted template, and `docker compose up -d`
does **not** recreate a container when only a bind-mounted file changed. Use
`docker restart anime-kage-nginx`.

## Publishing this repository

This repo is published from a private one that also holds planning documents and
unrelated projects. `export-platform.sh` in the private repo builds it
mechanically from tracked files and refuses to run if it finds anything that
looks like a credential.

Anything this repo must say differently belongs in the private repo, not in the
export step — an export that edits files is an export whose edits get lost.

Contributor-facing docs: `CONTRIBUTING.md` (the branch → PR → review loop, written
for someone new to git) and `SECURITY.md` (private vulnerability disclosure).

## Architecture

### Stack
- **Frontend**: SvelteKit 5 (runes), TypeScript, **plain CSS** — scoped `<style>` blocks + design tokens (`tokens.css`). **No Tailwind, no CSS framework — never add one.**
  - Gotcha: `.btn` / `.fill` / `.ghost` are **not global** — each page redefines them in its own scoped `<style>` (Svelte scoping means a page that only uses the class names gets unstyled browser buttons). Copy the block from `routes/admin/moderation/+page.svelte`.
- **Backend (product API)**: **Go** (`backend-go/`) — chi router, pgx + scany, hand-written SQL, JWT (HS256) + bcrypt. One backend language, by decision.
- **Future services** (resolver/extractor, health checker, subtitle worker — planned): also Go, as sibling `cmd/` binaries in the same module. See `the planning notes` §8.
- **Shared types**: `anime-kage/shared/types.ts` — import from here for cross-boundary types

### Request Flow
```
Browser → SvelteKit (port 5173)
        → lib/api.ts (ApiClient class)
        → Go backend (port 3000)
        → handler → repo (SQL via pgx) / jikan client
        → PostgreSQL
```

### Backend Structure (`backend-go/`)
- `cmd/api/main.go` — entry: config, db pool, graceful shutdown
- `internal/config/` — env loading; **fails hard on missing `JWT_SECRET`/`DATABASE_URL`** (no fallbacks, by design)
- `internal/handler/` — HTTP layer: one file per domain + `router.go` (chi wiring, CORS, rate limits, role gates)
- `internal/repo/` — data layer: hand-written SQL scanned into models (`titles.go` has the shared anime/manga query builder — one implementation, not two)
- `internal/model/` — JSON shapes mirroring `shared/types.ts` field-for-field
- `internal/auth/` — bcrypt, JWT sign/verify (claims `{userId,username,email,role}`, HS256 — tokens interchange with old TS ones), register/login validation (password: length only — min 8 runes, max 72 bytes, which is bcrypt's own ceiling; no case/symbol rules by decision)
- `internal/middleware/` — RequireAuth/OptionalAuth/RequireRole, per-IP rate limiting, security headers
- `internal/jikan/` — Jikan v4 client (334ms rate limit, 429 retry, transform to internal shape)
- `cmd/migrate/migrations/` — **schema source of truth**: numbered SQL files applied
  in order, tracked in `schema_migrations`. New schema change = new numbered file;
  never edit an applied one. Regenerate the test snapshot
  (`internal/handler/testdata/schema.sql`) after migrating — command in `api_test.go`.
- `cmd/populate`, `cmd/autoupdate` — Jikan import/sync CLIs (autoupdate is cron-able;
  `refresh` populates `broadcast_day` for the calendar)

### Frontend Structure (`frontend/src/`)
- `lib/api.ts` — Centralized `ApiClient` class; all backend calls go through here
- `lib/stores/auth.ts` — Svelte store for user auth state; token in localStorage
- `lib/stores/toast.ts` — Toast notification store (success/error/info/warning)
- `lib/components/` — Shared UI: AnimeCard, MangaCard, CommentSection, SearchAutocomplete, StarRating, Header, Toast, skeletons
- `routes/` — SvelteKit file-based routing

### Frontend Routes
| Route | Purpose |
|-------|---------|
| `/` | Landing page for guests (collage, features) — members are sent to `/home` |
| `/home` | Member dashboard: spotlight, continuă vizionarea, ultimele lansări, colecții + clasament, activitate/forum/anunțuri, programul săptămânii. **Every strip reads real data and renders an empty state when there is none** — no seed arrays |
| `/anime` | Anime list with search, filters, pagination |
| `/anime/[id]` | Anime detail. **`[id]` is a slug or a numeric id** — the API resolves either, and a numeric id 301-redirects to the slug so every existing `/anime/${a.id}` link lands on the canonical URL without being rewritten. Use `titleRef(a)` / `animeHref(a)` from `lib/types.ts` in new links |
| `/anime/[id]/episode/[episodeNumber]` | Video player (iframe), multi-source, prev/next nav (rolls into the next/previous season at a season edge), episode description with inline edit, filler/recap marks. Same slug handling as above |
| `/anime/search`, `/anime/trending`, `/anime/airing` | Discovery pages |
| `/anime/season/[year]/[season]` | Seasonal anime |
| `/anime/genre/[genre]` | Genre-filtered anime |
| `/manga` | Manga list with search, pagination |
| `/manga/[id]` | Manga detail: hero, chapters, readlist, comments, reader. Slug-or-id like `/anime/[id]` |
| `/login`, `/register` | Auth forms |
| `/parola-uitata`, `/reseteaza-parola` | Password reset: request a link, then redeem `?token=` |
| `/profile` | User profile: stats, edit bio/genres, logout |
| `/lista` | Combined watchlist+readlist with status tabs |
| `/calendar` | The team's programme, next 14 days, from `schedule_slots`. No longer derived from MAL broadcast days (and no longer falls back to a simulated `index % 7` assignment — both are gone) |
| `/comunitate` | Community hub: Recenzii (all users), Activitate (friends-only), Membri, Forum, Echipă — all real data |
| `/comunitate/forum/[id]` | Forum thread detail: body, replies, reply composer, mod pin/lock/delete |
| `/notificari` | Notification inbox (Toate / Necitite tabs) |
| `/admin` | Role-gated episode/chapter/link management |
| `/admin/curated` | "Vitrină" — pick what the landing collage, home spotlight and catalog banners show |
| `/anunturi` | All news posts, filterable by tag |
| `/anunturi/[slug]` | One post: cover, Markdown body, comment thread, "citește și" |
| `/admin/anunturi` | "Anunțuri" — the post editor: Markdown toolbar (bold/italic/headings/list/quote/link), image upload into the body, cover image, emoji picker, live preview, drafts (admin/moderator) |
| `/admin/program` | "Program" — place episodes on the weekly programme: pick a series, episode number, date+time (admin/coordinator) |

### Key Database Tables
| Table | Purpose |
|-------|---------|
| `users` | Auth + profiles (role: user/translator/moderator/admin) |
| `anime` / `manga` | Content metadata (synced from Jikan via `malId`). `slug` is the URL segment ("91-days"), unique via a partial index, filled by `cmd/autoupdate slugs` and **never regenerated** — a slug is a URL, and rewriting it on a title edit would break shared links |
| `episodes` / `chapters` | Per-episode/chapter records. `episodes.synopsis` is a team-written description (editable inline on the episode page, same gate as the series description); `is_filler` / `is_recap` are MAL's marks, overridable by hand since MAL is often unreachable |
| `contentLinks` | Hosting URLs for video/manga players (embedded via iframe) |
| `watchlist` / `readlist` | User tracking with status enum + score |
| `user_lists` + `user_list_items` | Member-curated collections (mixed anime/manga, per-item notes) |
| `comments` + `commentVotes` | Threaded discussion with like/dislike |
| `notifications` | Per-user in-app inbox (follow / reply / release events; `read_at` NULL = unread) |
| `forum_threads` + `forum_replies` | Persisted community forum (categories, pin/lock, denormalised reply_count/last_activity_at) |
| `announcements` | Team-written news posts. `body` is a **Markdown subset**, `cover_url` a local upload, `slug` the shareable URL (minted once, never regenerated), `tag` doubles as the filter on `/anunturi`, `is_published` is the draft flag. Comments live on the shared `comments` table via `announcement_id` |
| `schedule_slots` | The weekly programme, decided by admins/coordinators (`/admin/program`). One row = "episode N of series X goes live at this instant". `scheduled_at` is a **timestamptz**, so it renders in each viewer's own timezone; the MAL `broadcast_day`/`broadcast_time` it replaced could only be shown as "23:30 JST". `UNIQUE (anime_id, episode_number)` makes a reschedule an upsert, not a duplicate |
| `anime_relations` | Season chain + franchise graph from AniList. **Stores MAL ids only** — the local row is resolved by joining `anime.mal_id` at read time, so an edge to an unimported series costs nothing and resolves itself if that series ever arrives. Never creates an anime row |
| `chat_messages` | Live chat. A reply stores a snapshot of what it answered (`reply_to_user`, `reply_to_excerpt`) so the quote still reads correctly if the original is deleted, **plus `reply_to_id`** so the client can scroll to it. `ON DELETE SET NULL`: deleting the target keeps the reply and its quote, it just stops being clickable. Matching on the excerpt instead was rejected — two identical messages from one person are normal in a chat, and jumping to the wrong one is worse than not jumping. |
| `invites` | Discord-minted single-use registration codes (gated by `INVITE_ONLY`) |
| `curated_picks` | Editorially chosen titles per placement slot (landing collage, home spotlight, catalog banners) |
| `anime.banner_url` / `manga.banner_url` | Wide AniList banner art; `''` = asked, none exists; NULL = never asked |
| `users.banner_anime_id` / `banner_manga_id` | The series a member picked as their profile backdrop |
| `password_resets` | Reset tokens — **SHA-256 hash only**, single-use, 1h TTL |
| `animeTranslations` / `mangaTranslations` | Romanian translations (partially integrated) |

### API Endpoints Overview
- **Auth**: `POST /api/auth/register`, `POST /api/auth/login`, `GET /api/auth/me`, `POST /api/auth/logout`, `POST /api/auth/forgot-password` (always 200 — never reveals whether an address has an account), `POST /api/auth/reset-password` (single-use token; does **not** sign the user in)
- **Anime**: CRUD + search + trending + airing + seasonal + import from Jikan
- **Manga**: CRUD + search + trending + publishing + import from Jikan
- **Schedule**: `GET /api/schedule?days=7` (window the home page draws) / `?upcoming=1` (whole plan ahead, admin editor) — any member. `POST /`, `PUT /:id`, `DELETE /:id` — **admin/coordinator**. `scheduledAt` must be RFC3339 **with an offset**; a bare `2026-08-20T18:00` is rejected rather than silently read in the server's timezone. POST upserts on `(animeId, episodeNumber)`, so rescheduling moves the slot instead of duplicating it
- **Relations**: `GET /api/anime/:id/relations` → `{chain, related}`. `chain` is the PREQUEL→SEQUEL run in watch order (empty for a standalone title, and the anime itself is in it); `related` is everything that doesn't linearise — ALTERNATIVE/SIDE_STORY/SPIN_OFF/PARENT/SUMMARY. **Only titles in the catalog appear** (INNER JOIN on `mal_id`), so cards always link somewhere real
- **Episodes**: `GET/POST/PUT/DELETE /api/anime/:id/episodes[/:num]`, `POST /api/episodes/:id/links`. `PUT` accepts `synopsis` / `isFiller` / `isRecap` and is gated on **editRole** (admin/coordinator/translator — widened from contentRole so whoever can rewrite a series description can rewrite an episode's). A blank `synopsis` clears it; every other field is leave-alone-on-omit. `POST /api/anime/:id/episodes/sync` pulls titles/air dates/filler from MAL, falling back to AniList
- **Chapters**: `GET/POST/PUT/DELETE /api/manga/:id/chapters[/:num]`, `POST /api/chapters/:id/links`
- **Users**: profile CRUD, watchlist CRUD, readlist CRUD (`/api/users/me/...`). Public per-username: `GET /api/users/:username` (+ `/history`, `/watchlist`, `/readlist`, `/reviews`, `/followers`, `/following`) — lists, ratings, reviews and history are all public, Letterboxd-style
- **List import**: `POST /api/users/me/import/anilist` (`{username}` — public AniList list, one GraphQL call), `POST /api/users/me/import/mal` (multipart `file` — MAL's XML export; **Jikan's `/users/*` 504s and MAL v2 needs a client id**, so the export is the only reliable route and the only one that works for private lists). Matched on `mal_id`; skipped titles are counted and sampled
- **Profile banner**: `GET /api/users/me/banner/options` (titles from the member's own lists that have art), `PUT /api/users/me/banner` (`{mediaType,id}`; id 0 clears). Banner art comes from **AniList `bannerImage`** — MAL/Jikan have no banners — backfilled by `cmd/autoupdate banners`
- **Comments**: per-anime/manga comments, replies, voting, reporting
- **Notifications**: `GET /api/notifications` (list + unread), `GET /api/notifications/unread-count` (badge poll), `POST /api/notifications/read-all`, `POST /api/notifications/:id/read` — written best-effort from follow/reply/release-lifecycle handlers via `h.notify`
- **Curated**: `GET /api/curated` (public — every slot resolved to full titles, plus the slot registry), `PUT /api/curated/:slot` (admin/coordinator/moderator — replaces the slot; empty list = back to automatic), `POST /api/curated/image` (per-placement artwork → `/uploads/curated/`; **overrides the cover for that placement only, never writes `anime.image_url`**). Slot rules (max items, allowed media) live server-side in `handler/curated.go`, never in the UI
- **Community** (`/api/community`, all real data — no seed): `members`, `reviews` (all users), `activity?scope=friends|site` (auth — `friends` = mutual follows, the /comunitate tab; `site` = the whole community, what `/home` shows), `stats` (members + published subtitles), `team` (roles). Forum: `GET/POST /forum`, `GET/DELETE /forum/:id`, `POST /forum/:id/replies`, `POST /forum/:id/{pin,lock}` (mod)
- **Announcements** (`/api/announcements`): `GET /{idOrSlug}` (one post; drafts 404 for non-editors), `GET|POST /{id}/comments`, `POST /image` (cover/body upload → `/uploads/announcements/`). `coverUrl` must be a path we minted — a pasted remote URL is refused. `GET /` (any member; `?drafts=1` adds unpublished rows for admin/moderator and is silently ignored for everyone else), `POST /`, `PUT /:id`, `DELETE /:id` (admin/moderator). `url` must be an internal path or `https://` — it is rendered as an anchor on every member's home page, so `javascript:`/`http:`/protocol-relative are refused

### Environment Variables
**Backend** (env or `.env` in `backend-go/`):
- `DATABASE_URL` — PostgreSQL connection string (**required**)
- `JWT_SECRET` — Token signing secret (**required — the server refuses to start without it**)
- `JWT_EXPIRES_IN` — Token lifetime (default: `7d`; Go durations like `168h` also work)
- `PORT` — API port (default: `3000`)
- `CORS_ORIGIN` — comma-separated allowed origins (default: localhost:5173/4173)
- `UPLOADS_DIR` — avatar storage dir (default: `./uploads`)
- `TRUSTED_PROXIES` — CIDRs whose `X-Forwarded-For` is believed, comma-separated
  (default: none = attribute every request to its `RemoteAddr`). **Required behind a
  reverse proxy** or per-IP rate limiting sees one client. `chimw.RealIP` was removed
  because it trusted `X-Real-IP` from anyone, which the proxy in front does not
  sanitize. See `httpx.ClientIP`.
- `INVITE_ONLY` — `true` closes registration behind a Discord code (default: `false`)
- `PUBLIC_URL` — the site's own origin, used to build links in outgoing mail.
  Deliberately not derived from the request Host (default: `http://localhost:5173`)
- `SMTP_HOST` / `SMTP_PORT` / `SMTP_USER` / `SMTP_PASSWORD` / `MAIL_FROM` —
  outgoing mail. **With `SMTP_HOST` unset, mail is logged instead of sent** —
  that's how password reset is testable in dev; the API warns at startup
- `PASSWORD_RESET_TTL` — reset link lifetime (default: `1h`)

**Discord bot** (`cmd/discordbot`, env or the same `.env`):
- `DISCORD_TOKEN` (**required**), `DISCORD_GUILD_ID` (scopes the command to one
  server — instant registration vs up to an hour globally),
  `DISCORD_INVITE_CHANNEL_ID`, `DISCORD_INVITE_COMMAND` (default `invitație`),
  `INVITE_TTL` (default `7d`), `INVITE_QUOTA_WINDOW` (default `24h`)
- Anti-spam honeypot (August 2026): `DISCORD_PROTECTION_CHANNEL_ID` (**blank = the
  whole feature is off**), `DISCORD_MUTED_ROLE_ID` (added on top of a native
  timeout, never instead of one — see below), `DISCORD_PROTECTION_PURGE_WINDOW`
  (default `5m`, hard max 14 days — Discord's bulk-delete limit),
  `DISCORD_MOD_LOG_CHANNEL_ID`. The bot **refuses to start** if the honeypot is
  pointed at the invite channel

**Frontend** (Vite):
- `VITE_API_URL` — Backend base URL (default: `http://localhost:3000`)

### The Discord bot's two channel behaviours (August 2026)
Both live in `cmd/discordbot` and both react to `MessageCreate`, which is why the
bot's intents went from `IntentsNone` to `IntentsGuilds | IntentsGuildMessages`.
**`MESSAGE_CONTENT` is still off** — neither feature reads what a message says,
only that one arrived and who sent it — so nothing privileged needs approving.

- **Sticky invite board** (`sticky.go`) — the StickyBot trick: when anyone posts
  in the invite channel, delete our board and post it again underneath, so it is
  never more than one message from the bottom. Debounced (`stickyDelay`, 4s) and
  coalesced through a one-slot channel, so a burst of fifty messages costs one
  repost. Nothing is pinned any more: a sticky board is strictly more visible
  than a pin, and re-pinning would post a "pinned a message" line every time.
  `ensureMessage` **edits the board's text in place when the copy changes**,
  which matters because a bot's message cannot be edited from the Discord client
  by anyone — not even the server owner. Change the string, redeploy, done.
- **Honeypot** (`protection.go`) — one channel, named in
  `DISCORD_PROTECTION_CHANNEL_ID`, whose only message says not to post in it.
  The channel is deliberately not named here: the technique is well known and
  worth documenting, but which channel is the trap is not, since a spammer who
  knows simply avoids it. A phished account spamming every channel does
  not read signs, so posting there is a confession: mute first (a 28-day native
  timeout **and** the muted role, not one or the other — the timeout cannot be
  misconfigured but expires, the role never expires but is only as good as the
  channel overrides behind it, and **role toggles can only grant permissions, so
  a Muted role does nothing until each category denies Send Messages to it**),
  then delete everything that account said **anywhere** in
  the last `PURGE_WINDOW` by walking every readable channel and thread. Staff are
  exempt by permission (`staffPermissions`), not by a list somebody maintains,
  and an account that trips it repeatedly only starts one purge (`claimPunish`).
  It mutes and never bans, so a false positive is undone by removing a role.

### Jikan API Integration
`backend-go/internal/jikan/` wraps Jikan API v4 with:
- Rate limiting: 3 req/sec (334ms delay)
- Retry logic with exponential backoff for 429 responses
- Data transformation from Jikan format → internal schema

### Authentication Flow
1. POST `/api/auth/register` or `/api/auth/login` → returns JWT
2. Frontend stores token in localStorage via auth store
3. `ApiClient` injects `Authorization: Bearer <token>` on all requests
4. Backend middleware verifies JWT; `optionalAuth` routes work for both guests and users

## Frontend tests

Vitest, two projects (`vite.config.ts` → `test.projects`), 147 tests in ~3s.

| Project | Files | Environment | Covers |
|---------|-------|-------------|--------|
| `unit` | `src/**/*.test.ts` | node | Pure functions — no DOM, no component compile |
| `component` | `src/**/*.svelte.test.ts` | happy-dom | Components mounted via `@testing-library/svelte` |

The naming split is what selects the project, so **a component test must be
named `Foo.svelte.test.ts`** — call it `Foo.test.ts` and it runs in the `unit`
project with no DOM and fails on the first `render()`.

The component project sets `resolve.conditions: ['browser']`. Without it,
`import Foo from './Foo.svelte'` resolves to Svelte's SSR build and every
`render()` throws `lifecycle_function_unavailable: mount(...) is not available
on the server`.

What is covered today, and why these files first:
- **`markdown.ts` + `Markdown.svelte`** — the XSS boundary. The parser tests
  assert markup is never *produced*; the component tests assert the renderer
  never *reintroduces* it. Both halves are needed: either one alone can pass
  while the pair is broken. Also pins the link scheme allowlist, the
  `/uploads/` + Giphy image allowlist, and `rel="noopener noreferrer nofollow"`
  on external anchors.
- **`RichText.svelte`** — the comment/review/chat path, including `Spoiler`
  through it: reveal-once, no button when `interactive={false}` (a button
  cannot nest in an `<a>`), and `referrerpolicy="no-referrer"` on GIFs.
- **`providers.ts`** — provider slug → label, host fallback, and the
  never-render-a-blank-button rule.
- **`reltime.ts`** — every Romanian bucket, on a frozen clock.
- **`media.ts`, `avatar.ts`, `params/int.ts`** — small, and each one is relied
  on somewhere a wrong answer is invisible until a page 404s or a tile changes
  colour.
- **`PagePicker.svelte`** — open/close, link-vs-button modes, `aria-current`,
  Escape and click-outside, and the >40-page jump field.

Two behaviours are documented by tests rather than fixed, because both are
deliberate and easy to "fix" by accident:
- A markdown href ends at the first `)`, so wikipedia-style URLs cannot be
  linked (`markdown.test.ts` → "ends an href at the first closing paren").
- `PagePicker`'s jump field has `min`/`max`, so an out-of-range page never
  reaches `go()` — constraint validation blocks the submit. The clamp inside
  `go()` is a backstop, tested by dispatching `submit` directly.

**`npm run check` currently reports 7 pre-existing type errors** (curated list
picks, home shelf, community stats) — unrelated to tests, but they will fail a
CI step that runs `check`, so fix them before wiring one up.


## Rich text (news posts)

`announcements.body` is a **Markdown subset**, parsed by `frontend/src/lib/markdown.ts`
into a **token tree** and rendered as real elements by `Markdown.svelte`. There is
no `{@html}` anywhere and no sanitiser, because no HTML string is ever produced —
a post cannot inject markup, since markup is never parsed. `<script>alert(1)</script>`
in a body renders as those literal characters.

Two allowlists do the rest: links must be an internal path or `https://` (anything
else renders as plain text, not as a stripped anchor), and images must be under
`/uploads/` so a post cannot beacon every reader's IP to a third party.

Supported: `#`/`##`/`###` (rendered h2–h4 — the page's h1 is the title), `**bold**`,
`*italic*`, `` `code` ``, `||spoiler||`, `[text](url)`, `![alt](/uploads/…)`,
`- lists`, `> quotes`, `---`. Emoji need no support: they are ordinary characters
and travel through untouched, which is why the editor's picker just inserts them.

**Spoilers work in comments and reviews too**, via `splitSpoilers()` +
`SpoilerText.svelte` — plain text with only `||…||` honoured, deliberately NOT
the full Markdown parser (a comment that can mint headings and images is a
different feature with a different moderation story). `markdownExcerpt()` masks
spoilers as `•••`, because an excerpt is shown with no way to hide it again.
Inside an `<a>` pass `interactive={false}`: a reveal button cannot nest in a link.

Image uploads: `posterMaxUpload` is 4 MB (covers, curated art), `postMaxUpload`
is 24 MB (news posts — full-width screenshots, not 240px covers). The shared
handler is `uploadImage`, which accepts the file as either `poster` or `image`.

Prefer extending this over adding `marked`+`DOMPurify`: the input is staff-written
today, but "trusted input" is exactly how stored XSS reaches a front page.

## Page titles

`<title>` uses ` · ` as its only separator. It used to be ` — ` (em dash) on the
episode/chapter/review pages, which in a browser tab reads as a run of hyphens —
"Ep. 3 — 91Days" looked like "3--- 91 days". Eleven files were switched; keep new
titles on `·`.

## The catalog is curated by hand (August 2026)

**Nothing imports series automatically any more.** `autoupdate all` — what the
nightly cron runs — used to include `importSeasonal`, which pulled in every title
of the current season each night. That is where the catalog's
sequels-without-prequels came from (Mushoku Tensei III, Grand Blue Season 3,
Youjo Senki II, Bleach Kashin-tan…): nobody chose them, they were merely airing.

A series now enters the catalog when it is being translated, via
`/admin/catalog` or `cmd/populate`. `seasonal` still exists as an explicit
manual command; adding it back to `all` would undo this decision.

The knock-on effect: **MAL's `broadcast_day`/`broadcast_time` are no longer the
schedule.** They say when a series airs in Japan, which has nothing to do with
when our subtitle lands. The weekly programme is `schedule_slots`, decided by
admins/coordinators in `/admin/program` — see that table below.

## Episode metadata: two sources, MAL first (August 2026)

Titles, air dates and filler/recap marks come from **Jikan/MAL first, AniList as
a fallback**, because only MAL has all three — AniList has no `filler`/`recap`
flags and no per-episode air dates, just titles via `streamingEpisodes`.

The fallback is not optional politeness: **MAL fails per-entry, not just
globally.** `/anime/32998/episodes` (91 Days) and `/anime/877/episodes` (NANA)
return 504 on every attempt while `/anime/5114/episodes` (FMA) answers 200 every
time. Retrying is not a fix for those; AniList is. 91 Days' twelve episode titles
came from AniList for exactly this reason.

When a series falls back to AniList, `FillEpisodeMeta` is passed **nil** for
filler/recap rather than `false` — writing AniList's absent flags as `false`
would silently un-mark every filler episode MAL had already identified.

`AiringAnime` (status airing/upcoming) drives the nightly `episodes` step, which
is right for finding newly aired episodes but means **a completed series added by
hand is never reached**. Two ways to fill one in:
- `POST /api/anime/:id/episodes/sync` — the "↻ Sincronizează de pe MAL" button in
  `/admin/anime/{id}`, for one series
- `cmd/autoupdate backfill` — the whole catalog, manual only (walks every series
  and paginates, far too expensive nightly)

Neither ever overwrites a real title an editor typed. Both DO replace
*placeholder* titles — `"Episodul 5"`, `"Episode 5"`, `"Ep. 5"` — because those
are the absence of a title written out as text, not editorial content. The old
sync inserted `fmt.Sprintf("Episode %d")` when MAL had no title, which is where
they came from; migration 0037 nulled the ones already stored.

## The product direction (read this before designing anything)

**Anime-Kage hosts no anime video.** Video is resolved from third-party sources at play
time; what we own is the Romanian subtitle track, the skip timestamps, the catalog, and
the community. Manga is the exception — those pages can be hosted by us.

The consequence that matters when writing code: an `<iframe>` embed can never carry our
own subtitles or a skip-intro button (same-origin policy). Those features require
resolving a source to an HLS manifest and playing it in **our own player**. The current
iframe player is a fallback, not the target. See `the planning notes` §2.

## Known Bugs / active issues

Fixed by the Go rewrite (July 2026): JWT_SECRET fallback, missing role gates on
anime/manga import/update, no rate limiting, password min 6, avatar MIME trust,
comment avatarUrl-from-JWT bug, `?genres=` 500, ignored `sort` on filtered lists.

Still open:
- *(none from this list)*

Fixed 26 August 2026 — **`JWT_SECRET` rotated**, and the leak that made it urgent
closed. `/api/chat/stream` and `/api/releases/*` accept `?token=` because
EventSource and `<video>`/`<track>` cannot send an Authorization header, and both
nginx (combined format logs `$request`) and chi's Logger printed the whole URI —
so every such request wrote a live session JWT to disk. The claims carry no scope
and logout is client-side only, so that string was the entire account for its full
7-day life. Now: `mw.Logger` replaces `chimw.Logger` and masks it, nginx has a
`redacted` log_format doing the same, and the secret rotation invalidated
everything already written to the old logs.

Fixed July 2026: player iframes are sandboxed and content-link URLs are validated
server-side (https + public host + optional `CONTENT_HOSTS` allowlist).

Fixed August 2026: `jikan.AnimeByID`/`MangaByID` refuse an empty payload
(`ErrEmptyPayload`). A 200 with `data: {}` used to decode to a zero-valued
record, and `transformAnime` turned it into a *plausible* row — `mal_id` 0, blank
title, status "completed" via `mapOr`'s default — which the sync then wrote over
a live series. It destroyed one anime row (recoverable only as an episode titled
"My Cousin Girlfriend"), and the resulting `mal_id = 0` row collided with every
later occurrence, which is the `anime_mal_id_unique` violation the nightly
autoupdate logged for weeks. The damaged row was deleted (backup:
`anime-kage/logs/deleted-anime-24-backup-2026-08-17.sql`); `internal/jikan/jikan_test.go`
covers the guard. MAL still serves an empty record for mal_id 62811, so the
nightly log now shows a clean "empty record" skip for it instead.

## Current State

Core features work end-to-end: browsing, search with filters, watchlist/readlist with
full status tracking, public profiles with follows and public watchlists, reviews,
threaded comments with voting, history page, admin panel, toasts. Dark editorial theme,
Romanian UI, SSR on most routes (`adapter-node`). Backend is Go (`backend-go/`),
parity-tested against the old TS API.

**Nearly production-ready.** Prod images exist for both services (backend multi-binary
alpine, frontend adapter-node — `VITE_API_URL` is a build arg). The whole prod stack was
rehearsed from an empty database in July 2026 and the artifacts are known-good: the
nginx config is driven by `SITE_DOMAIN`, uploads/staging are on named volumes, and
`.env.prod.example` documents every variable. Remaining ops are on the box, not in the
repo — secrets, DNS, SMTP, backups, cron.
