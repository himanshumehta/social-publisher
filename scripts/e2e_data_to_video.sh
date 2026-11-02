#!/usr/bin/env bash
# End-to-end check of the data-to-video -> social-publisher path, in dry-run.
#
#   scripts/e2e_data_to_video.sh <data-to-video checkout> <rendered output dir>
#
# Needs Postgres, Redis and S3-compatible storage already running, configured
# through the usual DB_*, REDIS_*, S3_* variables (see .env.example), and a
# data-to-video checkout where `make setup && make demo` has run. It builds the
# binaries, migrates, creates a throwaway admin and API-key owner, starts the
# server and worker, sends the video with publish.py --send, approves the post
# as the admin, waits for the worker to publish it (dry run: "would publish"
# log lines, no platform API), then sends the same video again and checks that
# the replay returns the same post. Server and worker are stopped on exit.
set -euo pipefail

D2V=${1:?usage: $0 <data-to-video dir> <output dir>}
OUT=${2:?usage: $0 <data-to-video dir> <output dir>}
D2V=$(cd "$D2V" && pwd)
case "$OUT" in /*) ;; *) OUT="$D2V/$OUT" ;; esac

if [ "${PUBLISH_DRY_RUN:-true}" != "true" ]; then
  echo "refusing to run with PUBLISH_DRY_RUN=${PUBLISH_DRY_RUN}: this script must never post for real" >&2
  exit 1
fi
export PUBLISH_DRY_RUN=true APP_ENV=development
PORT=${APP_PORT:-8081}
export APP_PORT=$PORT APP_HOST=127.0.0.1
BASE="http://127.0.0.1:$PORT"
WORK=$(mktemp -d "${TMPDIR:-/tmp}/sp-e2e.XXXXXX")
PIDS=()
cleanup() {
  for p in "${PIDS[@]:-}"; do [ -n "$p" ] && kill "$p" 2>/dev/null || true; done
  wait 2>/dev/null || true
  echo "logs kept in $WORK"
}
trap cleanup EXIT

json() { python3 -c "import json,sys; d=json.load(sys.stdin); print($1)"; }

cd "$(dirname "$0")/.."
echo "== build"
go build -o "$WORK/bin/" ./cmd/migrate ./cmd/server ./cmd/worker ./cmd/admin

echo "== migrate"
"$WORK/bin/migrate" up 2>&1 | tail -1

echo "== accounts"
RUN=$(openssl rand -hex 4)
ADMIN_EMAIL="e2e-admin-$RUN@example.com"
OWNER_EMAIL="e2e-d2v-$RUN@example.com"
ADMIN_PW=$(openssl rand -hex 16)
echo "$ADMIN_PW" | "$WORK/bin/admin" create-user -email "$ADMIN_EMAIL" -role admin 2>&1 | tail -1
OWNER_ID=$(openssl rand -hex 16 | "$WORK/bin/admin" create-user -email "$OWNER_EMAIL" -role creator 2>&1 \
  | sed -n 's/^created user \([0-9a-f-]*\) .*/\1/p')
API_KEY=$("$WORK/bin/admin" create-api-key -email "$OWNER_EMAIL" -name data-to-video \
  -scopes posts:create,posts:read -ttl 1h 2>/dev/null)
echo "api key owner $OWNER_ID (key not shown)"

echo "== start server and worker"
"$WORK/bin/server" >"$WORK/server.log" 2>&1 & PIDS+=($!)
"$WORK/bin/worker" >"$WORK/worker.log" 2>&1 & PIDS+=($!)
for _ in $(seq 1 50); do curl -fsS "$BASE/health" >/dev/null 2>&1 && break; sleep 0.2; done
curl -fsS "$BASE/health/ready"; echo

send() {
  (cd "$D2V" && SOCIAL_PUBLISHER_URL=$BASE SOCIAL_PUBLISHER_TOKEN=$API_KEY \
    env -u GEMINI_API_KEY -u GOOGLE_API_KEY -u OPENAI_API_KEY -u ANTHROPIC_API_KEY \
    .venv/bin/python scripts/publish.py "$OUT" --send)
}

echo "== publish.py --send (first)"
FIRST=$(send); echo "$FIRST"
POST_ID=$(echo "$FIRST" | sed -n 's/.*post_id=\([0-9a-f-]*\).*/\1/p')
[ -n "$POST_ID" ] || { echo "no post_id in publish.py output" >&2; exit 1; }

status() {
  curl -fsS -H "Authorization: Bearer $API_KEY" "$BASE/api/v1/external/posts/$POST_ID" | json 'd["status"]'
}
echo "status before approval: $(status)"

echo "== admin approves"
TOKEN=$(curl -fsS -H 'Content-Type: application/json' \
  -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PW\"}" "$BASE/api/v1/auth/login" | json 'd["access_token"]')
curl -fsS -X POST -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/admin/posts/$POST_ID/approve"; echo

echo "== wait for the worker"
for _ in $(seq 1 60); do
  S=$(status); [ "$S" = published ] && break; sleep 1
done
echo "status after approval: $S"
[ "$S" = published ] || { echo "post did not reach published" >&2; tail -20 "$WORK/worker.log" >&2; exit 1; }
grep -o 'would publish [a-z]* '"$POST_ID"'[^"]*' "$WORK/worker.log" | sort -u

echo "== publish.py --send (replay)"
SECOND=$(send); echo "$SECOND"
echo "$SECOND" | grep -q "post_id=$POST_ID .*replayed" || { echo "replay did not return the same post" >&2; exit 1; }
TOTAL=$(curl -fsS -H "Authorization: Bearer $TOKEN" "$BASE/api/v1/admin/posts?author_id=$OWNER_ID" | json 'd["total"]')
echo "posts owned by the API key: $TOTAL"
[ "$TOTAL" = 1 ] || { echo "expected exactly one post" >&2; exit 1; }
echo "E2E OK"
