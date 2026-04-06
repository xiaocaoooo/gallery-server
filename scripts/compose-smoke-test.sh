#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_PORT="${APP_PORT:-18080}"
POSTGRES_PORT="${POSTGRES_PORT:-15432}"
VALKEY_PORT="${VALKEY_PORT:-16379}"
QDRANT_HTTP_PORT="${QDRANT_HTTP_PORT:-16333}"
QDRANT_GRPC_PORT="${QDRANT_GRPC_PORT:-16334}"
SEAWEED_MASTER_PORT="${SEAWEED_MASTER_PORT:-19333}"
SEAWEED_VOLUME_PORT="${SEAWEED_VOLUME_PORT:-18081}"
SEAWEED_FILER_PORT="${SEAWEED_FILER_PORT:-18888}"
IMAGINARY_PORT="${IMAGINARY_PORT:-19000}"
APP_URL="${APP_URL:-http://127.0.0.1:${APP_PORT}}"
READ_TOKEN="${SMOKE_READ_TOKEN:-read-secret}"
WRITE_TOKEN="${SMOKE_WRITE_TOKEN:-write-secret}"
ENV_BACKUP=""
RESPONSE_FILE="$(mktemp)"

cleanup() {
  set +e
  rm -f "$RESPONSE_FILE"
  docker compose down -v --remove-orphans >/dev/null 2>&1
  if [[ -n "$ENV_BACKUP" && -f "$ENV_BACKUP" ]]; then
    mv "$ENV_BACKUP" "$ROOT_DIR/.env"
  else
    rm -f "$ROOT_DIR/.env"
  fi
}
trap cleanup EXIT

wait_for_http() {
  local url="$1"
  local expected_status="$2"
  local attempts="${3:-90}"

  for ((i=1; i<=attempts; i++)); do
    local status
    status="$(curl --noproxy '*' -sS -o /dev/null -w '%{http_code}' "$url" 2>/dev/null || true)"
    if [[ "$status" == "$expected_status" ]]; then
      return 0
    fi
    sleep 2
  done

  echo "Timed out waiting for $url to return $expected_status" >&2
  return 1
}

assert_status() {
  local expected="$1"
  local actual="$2"
  local description="$3"
  if [[ "$actual" != "$expected" ]]; then
    echo "$description failed: expected status $expected, got $actual" >&2
    echo "Response body:" >&2
    cat "$RESPONSE_FILE" >&2
    return 1
  fi
}

cd "$ROOT_DIR"

if [[ -f "$ROOT_DIR/.env" ]]; then
  ENV_BACKUP="$(mktemp)"
  cp "$ROOT_DIR/.env" "$ENV_BACKUP"
fi

: > "$ROOT_DIR/.env"
{
  printf 'READ_TOKEN=%s\n' "$READ_TOKEN"
  printf 'WRITE_TOKEN=%s\n' "$WRITE_TOKEN"
  printf 'APP_PORT=%s\n' "$APP_PORT"
  printf 'POSTGRES_PORT=%s\n' "$POSTGRES_PORT"
  printf 'VALKEY_PORT=%s\n' "$VALKEY_PORT"
  printf 'QDRANT_HTTP_PORT=%s\n' "$QDRANT_HTTP_PORT"
  printf 'QDRANT_GRPC_PORT=%s\n' "$QDRANT_GRPC_PORT"
  printf 'SEAWEED_MASTER_PORT=%s\n' "$SEAWEED_MASTER_PORT"
  printf 'SEAWEED_VOLUME_PORT=%s\n' "$SEAWEED_VOLUME_PORT"
  printf 'SEAWEED_FILER_PORT=%s\n' "$SEAWEED_FILER_PORT"
  printf 'IMAGINARY_PORT=%s\n' "$IMAGINARY_PORT"
} >> "$ROOT_DIR/.env"

docker compose down -v --remove-orphans >/dev/null 2>&1 || true

docker compose build app

docker compose up -d
wait_for_http "$APP_URL/healthz" 200 120

status="$(curl --noproxy '*' -sS -o "$RESPONSE_FILE" -w '%{http_code}' "$APP_URL/v1/tags")"
assert_status 401 "$status" "Unauthorized tag list"

status="$(curl --noproxy '*' -sS -o "$RESPONSE_FILE" -w '%{http_code}' \
  -H "Content-Type: application/json" \
  -H "X-Write-Token: $WRITE_TOKEN" \
  -d '{"name":"Cat"}' \
  "$APP_URL/v1/tags")"
assert_status 201 "$status" "Create tag"

python - "$RESPONSE_FILE" <<'PY'
import json
import pathlib
import sys

payload = json.loads(pathlib.Path(sys.argv[1]).read_text())
if payload.get("name") != "Cat":
    raise SystemExit(f"expected created tag name Cat, got {payload!r}")
PY

status="$(curl --noproxy '*' -sS -o "$RESPONSE_FILE" -w '%{http_code}' \
  -H "Content-Type: application/json" \
  -H "X-Write-Token: $WRITE_TOKEN" \
  -d '{"name":"cat"}' \
  "$APP_URL/v1/tags")"
assert_status 409 "$status" "Case-insensitive duplicate tag creation"

status="$(curl --noproxy '*' -sS -o "$RESPONSE_FILE" -w '%{http_code}' \
  -H "X-Read-Token: $READ_TOKEN" \
  "$APP_URL/v1/tags")"
assert_status 200 "$status" "Authorized tag list"

python - "$RESPONSE_FILE" <<'PY'
import json
import pathlib
import sys

payload = json.loads(pathlib.Path(sys.argv[1]).read_text())
items = payload.get("items", [])
if not any(item.get("name") == "Cat" for item in items):
    raise SystemExit(f"expected Cat tag in response, got {payload!r}")
PY

status="$(curl --noproxy '*' -sS -o "$RESPONSE_FILE" -w '%{http_code}' \
  "$APP_URL/v1/tags?token=$READ_TOKEN")"
assert_status 200 "$status" "Query-token tag list"

printf 'Compose smoke test passed.\n'
