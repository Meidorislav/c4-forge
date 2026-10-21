#!/usr/bin/env bash
# Smoke test of the container image: starts the compose stack from an already
# built c4forge:local image and checks that the app comes up healthy and
# serves the API and the UI.
#
# Runs as a separate compose project, so it never touches the data of the
# local development stack; everything it creates is removed on exit.
set -euo pipefail

cd "$(dirname "$0")/.."

export COMPOSE_PROJECT_NAME=c4forge-smoke
export C4FORGE_HTTP_PORT="${C4FORGE_SMOKE_PORT:-18080}"
base="http://127.0.0.1:${C4FORGE_HTTP_PORT}"

cleanup() { docker compose down --volumes --remove-orphans >/dev/null 2>&1 || true; }
trap cleanup EXIT

fail() {
  echo "smoke test failed: $*" >&2
  docker compose ps >&2 || true
  docker compose logs --no-color app >&2 || true
  exit 1
}

echo "starting the stack on ${base}"
docker compose up -d --no-build --wait --wait-timeout 120 || fail "the stack did not become healthy"

curl -fsS "${base}/readyz" | grep -q '"status":"ready"' || fail "/readyz is not ready"
curl -fsS "${base}/" | grep -q '<div id="root">' || fail "the UI is not served"
curl -fsSI "${base}/" | grep -qi '^content-security-policy:' || fail "the Content-Security-Policy header is missing"

app_container="$(docker compose ps -q app)"
uid="$(docker top "${app_container}" -o pid,uid | awk 'NR > 1 { print $2; exit }')"
case "${uid}" in
  "" | *[!0-9]* | 0) fail "the app must run as a non-root numeric uid, got '${uid}'" ;;
esac

echo "smoke test passed (app uid ${uid})"
