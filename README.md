# Anime-Kage

The platform behind [anime-kage.ro](https://anime-kage.ro) — a Romanian-language
anime and manga catalogue, watchlist and community, built by a fansub team.

Anime-Kage **hosts no video**. What the team owns is the Romanian subtitle track,
the catalogue, and the community around it; playback is resolved from third-party
sources at watch time. That constraint shapes most of the architecture.

## Stack

| Part | Choice |
|------|--------|
| Frontend | SvelteKit 5 (runes), TypeScript, plain CSS with design tokens — no CSS framework |
| Backend | Go — chi router, pgx + scany, hand-written SQL, JWT (HS256) + bcrypt |
| Database | PostgreSQL 15 |
| Proxy | nginx, behind Cloudflare |
| Monitoring | Prometheus + Grafana, alerts to Discord |

## Running it locally

You need Docker and **Node 22** (`.npmrc` sets `engine-strict=true`, and the
production image is `node:22-alpine`, so other majors are refused).

```bash
git clone <this repo> && cd platform
./dev.sh
```

That brings up Postgres, the Go API and the SvelteKit dev server:

- Frontend — http://localhost:5173
- API — http://localhost:3000
- Database — localhost:5432 (`dev` / `dev_password` / `anime_kage_dev`)

The dev database starts **empty**. See `docs/` for loading a seed catalogue.

## Layout

```
frontend/      SvelteKit app
backend-go/    Go API, migrations, and the cron binaries (populate, autoupdate…)
shared/        TypeScript types shared across the boundary
nginx/         production reverse proxy (templated from SITE_DOMAIN)
monitoring/    Prometheus config, Grafana dashboards, alert rules
```

## Tests

```bash
cd frontend  && npm test        # Vitest — unit + component
cd backend-go && go test ./...  # Go unit tests
```

## Contributing

Read `CONTRIBUTING.md` first — it covers the branch → PR → review loop from
scratch, including for people who have not used git before.

`CLAUDE.md` documents the architecture and the decisions behind it. It is
written for AI coding assistants but is the best orientation for humans too.

## Security

Never commit a `.env`. If you believe you have found a vulnerability, see
`SECURITY.md` — please do not open a public issue.

## Licence

See `LICENSE`.
