# anime-kage backend (Go)

The product API: catalogue, accounts, watchlists, comments, releases and the
admin surface. chi for routing, pgx with hand-written SQL for data, HS256 JWTs
and bcrypt for auth.

Several entry points share this module under `cmd/` — the API itself plus the
catalogue-sync and maintenance commands that run on a schedule.

## Run

```bash
cp .env.example .env   # fill in DATABASE_URL + JWT_SECRET (both required, no fallbacks)
go run ./cmd/api
```

Or via docker compose from `anime-kage/`: `docker compose -f docker-compose.dev.yml up`.

## Layout

```
cmd/api/            the server (config, db pool, graceful shutdown)
cmd/migrate/        SQL migrations — migrations/*.sql applied in order
cmd/populate/       bulk Jikan import (-ids 5114,9253 or curated default list)
cmd/autoupdate/     Jikan sync for cron: episodes | refresh | seasonal | all
internal/config/    env loading — fails hard on missing secrets
internal/handler/   HTTP layer: parsing, validation, response shapes, router.go
internal/repo/      data layer: hand-written SQL via pgx, scanned with scany
internal/model/     JSON DTOs mirroring ../shared/types.ts field-for-field
internal/auth/      bcrypt + JWT (HS256, claims {userId,username,email,role})
internal/middleware/ auth context, role gates, per-IP rate limit, security headers
internal/jikan/     Jikan v4 client (rate-limited, retrying)
```

Conventions:
- `shared/types.ts` stays the wire contract; change it and `internal/model` together.
- Handlers never touch SQL; repo functions never touch HTTP.
- `repo.ErrNotFound` / `repo.ErrExists` are the only sentinel errors handlers branch on.
- The watchlist/readlist upsert is a **partial merge** (`coalesce` on conflict):
  a progress-only update must never wipe notes or score — reviews live in notes.
  This is load-bearing; there are parity tests for it.

## Tests

```bash
go test ./...          # unit tests only — integration suite skips itself without a DB
go test -short ./...   # also skips the sleepy jikan 429-retry test
```

The integration suite (`internal/handler/api_test.go`) exercises the real router
against a real Postgres. It's gated behind `TEST_DATABASE_URL` and **wipes that
database on every run** (drops + recreates the schema), so it refuses any DSN
whose database name doesn't contain "test":

```bash
docker exec anime-kage-postgres-dev createdb -U dev anime_kage_test   # once
TEST_DATABASE_URL='postgresql://dev:dev_password@localhost:5432/anime_kage_test' go test ./...
```

The schema it applies is the snapshot at `internal/handler/testdata/schema.sql`.
After adding a migration, regenerate it (command documented at the top of
`api_test.go`):

```bash
docker exec anime-kage-postgres-dev pg_dump -U dev --schema-only --no-owner --no-privileges anime_kage_dev \
  | sed '/^\\/d' > internal/handler/testdata/schema.sql
```

## Migrations

`cmd/migrate/migrations/` is the schema source of truth. To change the schema,
add a new numbered file (`0002_add_x.sql`) and run:

```bash
go run ./cmd/migrate     # applies pending migrations, tracked in schema_migrations
```

`0001_baseline.sql` is the full schema at the Go cutover; a database that
predates the tool (tables exist, no ledger) is adopted without re-running it.
After a migration, regenerate the test snapshot (`internal/handler/testdata/
schema.sql`) — command at the top of `api_test.go` and in the Tests section.

## Keeping the catalog fresh

```bash
go run ./cmd/populate                  # import the curated popular list
go run ./cmd/populate -ids 52991       # import specific MAL ids
go run ./cmd/autoupdate                # add newly-aired episodes (default)
go run ./cmd/autoupdate refresh        # re-sync airing/upcoming metadata
                                       #   (status flips, score, broadcast day)
go run ./cmd/autoupdate seasonal       # import the current season's new titles
go run ./cmd/autoupdate all            # everything — good nightly cron target
```

All need only `DATABASE_URL`. The Jikan client rate-limits itself (3 req/s);
per-title Jikan errors (504s happen) are logged and skipped, not fatal.
