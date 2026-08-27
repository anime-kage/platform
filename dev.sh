#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"

# Anime-Kage development environment: postgres + Go backend + SvelteKit frontend.
# The Go backend (backend-go/) runs via `go run` inside a golang container with
# the source bind-mounted — after code changes: $COMPOSE restart backend

echo "🚀 Starting Anime-Kage development environment..."

if ! docker info > /dev/null 2>&1; then
    echo "❌ Docker is not running. Please start Docker first."
    exit 1
fi

# Prefer the compose plugin, fall back to the standalone binary
if docker compose version > /dev/null 2>&1; then
    COMPOSE="docker compose"
elif command -v docker-compose > /dev/null 2>&1; then
    COMPOSE="docker-compose"
else
    echo "❌ Docker Compose not found. Please install it."
    exit 1
fi

# --env-file /dev/null disables compose's automatic ./.env load.
#
# That autoload is why dev could silently inherit PRODUCTION settings: the
# deploy .env lives in this same directory, so ${INVITE_ONLY:-false} in the dev
# compose file resolved to prod's INVITE_ONLY=true and registration came back
# "Ai nevoie de un cod de invitație" on a database with no users in it. Any
# other ${VAR} would have bled the same way.
#
# Shell environment still wins over this, so rehearsing the real launch flow
# still works:  INVITE_ONLY=true ./dev.sh
COMPOSE="$COMPOSE --env-file /dev/null"

# First run: apply schema migrations (backend-go/cmd/migrate).
# Detected by asking postgres whether `users` exists.
$COMPOSE -f docker-compose.dev.yml up -d --wait postgres
if ! docker exec anime-kage-postgres-dev psql -U dev -d anime_kage_dev -tAc \
        "SELECT 1 FROM information_schema.tables WHERE table_name = 'users'" 2>/dev/null | grep -q 1; then
    echo "🗄️  Empty database — running schema init (one-time)..."
    $COMPOSE -f docker-compose.dev.yml --profile setup run --rm db-init
fi

echo ""
echo "🌐 Frontend:    http://localhost:5173  (SvelteKit, hot reload)"
echo "🔧 Backend API: http://localhost:3000  (Go — restart after code changes:"
echo "                $COMPOSE -f docker-compose.dev.yml restart backend)"
echo "🗄️  Database:   localhost:5432 (dev / dev_password / anime_kage_dev)"
echo ""
echo "Press Ctrl+C to stop all services"
echo ""

$COMPOSE -f docker-compose.dev.yml up --build backend frontend
