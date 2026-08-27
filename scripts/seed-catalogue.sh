#!/bin/bash
# Fill a dev or staging database with a usable catalogue and FAKE members.
#
#   scripts/seed-catalogue.sh staging
#   scripts/seed-catalogue.sh dev
#
# What it copies from production: the catalogue only — titles, episodes, source
# links, the relation graph. All of it is public metadata that came from MAL and
# AniList in the first place.
#
# What it NEVER copies: anything about a person. Not users, not watchlists, not
# comments, chat, notifications, history, invites or password resets. Those rows
# are the one irreplaceable thing on the platform, they are covered by GDPR, and
# a contributor does not need a single one of them to work on the site. The
# members you get instead are invented, with a shared throwaway password.
#
# Also skipped: subtitles and releases. Not because they are personal, but
# because the Romanian subtitle track is the team's actual work product and it
# has no business sitting on a contributor's laptop.
#
# THIS RESETS THE WHOLE TARGET DATABASE, not just the catalogue tables.
# users.banner_anime_id and users.banner_manga_id are foreign keys onto anime
# and manga, so TRUNCATE ... CASCADE on the catalogue reaches `users`, and from
# there everything that references a user: chat, watchlists, comments, lists.
# That was a surprise the first time and is now the documented behaviour, since
# a clean slate is what you actually want from a seed. Say so out loud and
# require confirmation, rather than letting someone lose test state they cared
# about.
set -euo pipefail
cd "$(dirname "$0")/.."

TARGET=${1:-}
case "$TARGET" in
  staging) COMPOSE="docker compose -f docker-compose.staging.yml --env-file .env.staging"
           CONTAINER=anime-kage-postgres-staging; ENVFILE=.env.staging ;;
  dev)     COMPOSE="docker compose --env-file /dev/null -f docker-compose.dev.yml"
           CONTAINER=anime-kage-postgres-dev; ENVFILE="" ;;
  *) echo "usage: $0 {staging|dev}"; exit 1 ;;
esac

# Refuse to touch production, however this is called.
if [ "$CONTAINER" = "anime-kage-postgres" ]; then
  echo "refusing to seed production"; exit 1
fi

PROD_USER=$(grep -oP '^DB_USER=\K.*' .env)
PROD_DB=$(grep -oP '^DB_NAME=\K.*' .env)
if [ -n "$ENVFILE" ]; then
  DST_USER=$(grep -oP '^DB_USER=\K.*' "$ENVFILE"); DST_DB=$(grep -oP '^DB_NAME=\K.*' "$ENVFILE")
else
  DST_USER=dev; DST_DB=anime_kage_dev
fi

# Public catalogue only. Order matters: parents before children.
TABLES=(anime manga episodes chapters content_links anime_relations skip_marks emotes)

echo "seeding $TARGET  ($DST_DB) from the production catalogue"
echo "  tables: ${TABLES[*]}"
echo
echo "  This ERASES everything currently in $DST_DB — including any accounts,"
echo "  chat and lists you created there while testing."
if [ "${FORCE:-}" != "1" ] && [ -t 0 ]; then
    printf '  type the target name (%s) to continue: ' "$TARGET"
    read -r confirm
    [ "$confirm" = "$TARGET" ] || { echo "  aborted"; exit 1; }
fi

ARGS=(); for t in "${TABLES[@]}"; do ARGS+=(-t "public.$t"); done

docker exec "anime-kage-postgres" pg_dump -U "$PROD_USER" -d "$PROD_DB" \
    --data-only --no-owner --no-privileges --disable-triggers "${ARGS[@]}" \
  | docker exec -i "$CONTAINER" psql -U "$DST_USER" -d "$DST_DB" \
      -v ON_ERROR_STOP=1 -q \
      -c "TRUNCATE $(IFS=,; echo "${TABLES[*]}") RESTART IDENTITY CASCADE;" \
      -f -

echo "  catalogue loaded"

# Fabricated members. The hash is bcrypt for the password below and is the same
# for everyone — this database is disposable by definition.
docker exec -i "$CONTAINER" psql -U "$DST_USER" -d "$DST_DB" -v ON_ERROR_STOP=1 -q <<'SQL'
INSERT INTO users (username, email, password_hash, role)
VALUES
  ('admin',      'admin@example.test',      '$2a$10$5lIr5dyFWCR9kw2AGB4Nmexy8yYej5FWVv8V6uxG9bQa.Be4KnoQe', 'admin'),
  ('coordonator','coordonator@example.test','$2a$10$5lIr5dyFWCR9kw2AGB4Nmexy8yYej5FWVv8V6uxG9bQa.Be4KnoQe', 'coordinator'),
  ('traducator', 'traducator@example.test', '$2a$10$5lIr5dyFWCR9kw2AGB4Nmexy8yYej5FWVv8V6uxG9bQa.Be4KnoQe', 'translator'),
  ('moderator',  'moderator@example.test',  '$2a$10$5lIr5dyFWCR9kw2AGB4Nmexy8yYej5FWVv8V6uxG9bQa.Be4KnoQe', 'moderator'),
  ('membru',     'membru@example.test',     '$2a$10$5lIr5dyFWCR9kw2AGB4Nmexy8yYej5FWVv8V6uxG9bQa.Be4KnoQe', 'user')
ON CONFLICT (username) DO NOTHING;
SQL

echo
echo "done. sign in with any of:"
echo "  admin@example.test / coordonator@example.test / traducator@example.test"
echo "  moderator@example.test / membru@example.test"
echo "  password: seed-password"
