#!/usr/bin/env bash
# Local development entry point.
#
#   ./scripts/dev.sh db     start only PostgreSQL (port 5433)
#   ./scripts/dev.sh api    run the Go API on :8081 against that database
#   ./scripts/dev.sh web    run the Next.js app on :3100
#   ./scripts/dev.sh all    db, then api and web in this terminal
#
# The ports match docker-compose.yml and the NEXT_PUBLIC_API_BASE_URL default, so
# an existing compose stack and this script do not fight over anything.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dsn="${DATABASE_URL:-postgres://nepse:nepse_local_dev@localhost:5433/nepse?sslmode=disable}"
secret="${JWT_SECRET:-dev-secret-minimum-32-bytes-secure-nepse-platform}"

cmd="${1:-all}"

start_db() {
  docker compose -f "$root/docker-compose.yml" up -d postgres
}

start_api() {
  cd "$root/services/api"
  DATABASE_URL="$dsn" JWT_SECRET="$secret" HTTP_ADDR=":8081" \
    ALLOWED_ORIGINS="http://localhost:3100" COOKIE_SECURE=false \
    go run ./cmd/server
}

start_web() {
  cd "$root/apps/web"
  [[ -d node_modules ]] || npm ci
  NEXT_PUBLIC_API_BASE_URL="${NEXT_PUBLIC_API_BASE_URL:-http://localhost:8081}" npm run dev
}

case "$cmd" in
  db) start_db ;;
  api) start_api ;;
  web) start_web ;;
  all)
    start_db
    ( start_api ) &
    api_pid=$!
    # If the web tier exits, stop the API too rather than leaving it orphaned.
    trap 'kill $api_pid 2>/dev/null || true' EXIT
    start_web
    ;;
  *)
    echo "usage: $0 {db|api|web|all}" >&2
    exit 2
    ;;
esac
