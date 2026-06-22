#!/usr/bin/env bash
# One-command local debug launcher for APAGE.
#
# Brings up infra (Postgres/Redis/MinIO) via docker compose, builds the Go
# binaries, then runs api + worker + web in this one terminal. Every line is
# prefixed with a colored service tag and scrolls together. Ctrl+C stops
# everything (Go services shut down gracefully; the web dev server is killed).
# apage-cli is also built but NOT auto-started — customers run it manually
# (`apage-cli mcp`) to expose the MCP server their agent calls.
#
# Usage:
#   ./scripts/dev.sh             # infra + all app services + frontend
#   ./scripts/dev.sh --no-web    # backend only (skip Next.js)
#   STOP_INFRA=1 ./scripts/dev.sh  # also `docker compose stop` on exit
#
# Env overrides (defaults match docker-compose host ports): DATABASE_URL,
# REDIS_URL, S3_ENDPOINT, SESSION_SECRET, APAGE_API_URL, WEB_PORT.

set -uo pipefail
cd "$(dirname "$0")/.."

WITH_WEB=1
[[ "${1:-}" == "--no-web" ]] && WITH_WEB=0

# --- colors / labels -------------------------------------------------------
NC=$'\033[0m'
C_DEV=$'\033[1;34m'; C_API=$'\033[36m'
C_WRK=$'\033[33m';  C_WEB=$'\033[32m'; C_ERR=$'\033[31m'

log()  { printf '%b%-7s%b│ %s\n' "$C_DEV" "dev" "$NC" "$*"; }
err()  { printf '%b%-7s%b│ %s\n' "$C_ERR" "dev" "$NC" "$*" >&2; }

# stream <label> <color> <cmd...> : run cmd, prefixing each output line.
stream() {
  local name="$1" color="$2"; shift 2
  (
    "$@" 2>&1 | while IFS= read -r line; do
      printf '%b%-7s%b│ %s\n' "$color" "$name" "$NC" "$line"
    done
  ) &
}

# --- cleanup: kill the whole process group on exit -------------------------
cleanup() {
  trap - INT TERM EXIT
  printf '\n'; log "stopping services…"
  # Kill tracked log-pipe subshells, then make sure the service processes die
  # (covers grandchildren and non-interactive launches like `make dev`).
  kill $(jobs -p) 2>/dev/null
  pkill -f 'bin/apage-api'     2>/dev/null
  pkill -f 'bin/apage-worker'  2>/dev/null
  [[ "$WITH_WEB" == "1" ]] && pkill -f 'next dev' 2>/dev/null
  # Process-group kill (works when launched as a PG leader, e.g. ./scripts/dev.sh).
  kill -- -$$ 2>/dev/null
  if [[ "${STOP_INFRA:-0}" == "1" ]]; then
    log "stopping infra (docker compose stop)…"
    docker compose stop >/dev/null 2>&1
  else
    log "infra (postgres/redis/minio) left running — STOP_INFRA=1 to stop it too."
  fi
  log "bye."
  exit 0
}
trap cleanup INT TERM

# --- env (defaults match docker-compose host mappings) ---------------------
: "${DATABASE_URL:=postgres://apage:apage@localhost:5433/apage?sslmode=disable}"
: "${REDIS_URL:=redis://localhost:6379/0}"
: "${S3_ENDPOINT:=http://localhost:9100}"
: "${S3_BUCKET:=apage}"
: "${S3_ACCESS_KEY:=minioadmin}"
: "${S3_SECRET_KEY:=minioadmin}"
: "${APP_BASE_DOMAIN:=preview.localhost}"
: "${SESSION_SECRET:=dev-session-secret}"
: "${APAGE_API_URL:=http://localhost:8080}"
: "${WEB_PORT:=3000}"
export DATABASE_URL REDIS_URL S3_ENDPOINT S3_BUCKET S3_ACCESS_KEY S3_SECRET_KEY \
       APP_BASE_DOMAIN SESSION_SECRET

# --- 1. infra --------------------------------------------------------------
log "starting infra (postgres :5433, redis :6379, minio :9100)…"
if ! docker compose up -d --wait postgres redis minio; then
  err "infra failed to become healthy. Check 'docker compose ps'."
  err "Tip: stale volume? docker volume rm apage_pgdata && retry. Port taken? lsof -nP -iTCP:5433 -sTCP:LISTEN"
  exit 1
fi

# --- 2. build --------------------------------------------------------------
# apage-cli is built for convenience but not auto-started below — customers run
# it manually (`./bin/apage-cli mcp`) to expose the MCP server their agent calls.
log "building Go binaries…"
mkdir -p bin
if ! go build -o bin/apage-api ./cmd/api \
   && go build -o bin/apage-worker ./cmd/worker \
   && go build -o bin/apage-cli ./cmd/apage-cli; then
  err "go build failed."
  exit 1
fi

# Clear any stale instances holding the ports.
pkill -f 'bin/apage-(api|worker)' 2>/dev/null || true
sleep 0.3

# --- 3. app services -------------------------------------------------------
log "starting services — Ctrl+C to stop all."
log "  api  http://localhost:8080   web http://localhost:${WEB_PORT}"
stream "API"    "$C_API" ./bin/apage-api
stream "WORKER" "$C_WRK" ./bin/apage-worker

if [[ "$WITH_WEB" == "1" ]]; then
  if [[ ! -d web/node_modules ]]; then
    log "installing web deps (first run)…"
    (cd web && npm install >/dev/null 2>&1) || err "npm install failed; run it manually in web/"
  fi
  stream "WEB" "$C_WEB" env APAGE_API_URL="$APAGE_API_URL" npm --prefix web run dev -- -p "$WEB_PORT"
fi

# Block until Ctrl+C. The `sleep & wait $!` idiom keeps the trap responsive to
# signals even when launched non-interactively (a bare `wait` defers them).
while :; do
  sleep 2 & wait $! 2>/dev/null
done
