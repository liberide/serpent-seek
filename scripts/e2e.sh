#!/usr/bin/env bash
# SerpentSeek end-to-end smoke test.
#
# By default it runs the locally built binary against a temporary SQLite file.
# Set USE_DOCKER=1 to run against `docker compose up --build` instead.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${PORT:-18080}"
BASE="http://127.0.0.1:${PORT}"
WORKDIR="$(mktemp -d)"
LOG="${WORKDIR}/serpentseek.log"
PID=""

cleanup() {
	if [[ -n "${PID}" ]]; then
		kill "${PID}" 2>/dev/null || true
		wait "${PID}" 2>/dev/null || true
	fi
	if [[ -n "${COMPOSE_PID:-}" ]]; then
		docker compose -f "${ROOT}/docker-compose.yml" down -v >/dev/null 2>&1 || true
	fi
	rm -rf "${WORKDIR}"
}
trap cleanup EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
step() { echo; echo "== $* =="; }

start_local() {
	step "building binary"
	(cd "${ROOT}" && CGO_ENABLED=0 go build -o "${WORKDIR}/serpentseek" ./cmd/serpentseek)
	SQLITE_PATH="${WORKDIR}/e2e.db" PORT="${PORT}" \
		"${WORKDIR}/serpentseek" >"${LOG}" 2>&1 &
	PID=$!
}

start_docker() {
	step "docker compose up --build"
	(cd "${ROOT}" && docker compose up -d --build)
	COMPOSE_PID=1
}

if [[ "${USE_DOCKER:-0}" == "1" ]]; then
	command -v docker >/dev/null || fail "docker is required for USE_DOCKER=1"
	start_docker
else
	start_local
fi

step "waiting for health"
for _ in $(seq 1 60); do
	if curl -fsS "${BASE}/healthz" >/dev/null 2>&1; then break; fi
	sleep 0.5
done
curl -fsS "${BASE}/healthz" | grep -q '"ok":true' || fail "healthz did not report ok"

step "setup token + first admin"
SETUP_TOKEN=""
if [[ -n "${SETUP_TOKEN_ENV:-}" ]]; then
	SETUP_TOKEN="${SETUP_TOKEN_ENV}"
else
	SETUP_TOKEN="$(grep -o 'SETUP_TOKEN: [0-9a-f]*' "${LOG}" | tail -1 | awk '{print $2}')"
fi
[[ -n "${SETUP_TOKEN}" ]] || fail "could not find SETUP_TOKEN in logs"

SETUP_RESP="$(curl -fsS -X POST "${BASE}/api/auth/setup" \
	-H 'Content-Type: application/json' \
	-d "{\"token\":\"${SETUP_TOKEN}\",\"name\":\"e2e-admin\"}")"
API_KEY="$(printf '%s' "${SETUP_RESP}" | sed -n 's/.*"api_key":"\([^"]*\)".*/\1/p')"
[[ -n "${API_KEY}" ]] || fail "setup did not return an api_key"

step "machine search returns 200 + array with headers"
HEADERS="$(curl -fsS -D - -o "${WORKDIR}/rows.json" -X POST "${BASE}/search" \
	-H "Authorization: Bearer ${API_KEY}" \
	-H 'Content-Type: application/json' \
	-d '{"query":"e2e smoke test","count":3}')"
printf '%s' "${HEADERS}" | grep -qi '^X-Serpent-Rid:' || fail "missing X-Serpent-Rid header"
printf '%s' "${HEADERS}" | grep -qi '^X-Serpent-Api:' || fail "missing X-Serpent-Api header"
python3 - "${WORKDIR}/rows.json" <<'PY' || fail "response is not a JSON array"
import json,sys
data=json.load(open(sys.argv[1]))
assert isinstance(data, list), type(data)
PY

step "history records the request"
REQ_ID="$(curl -fsS "${BASE}/api/requests?limit=1" -H "Authorization: Bearer ${API_KEY}" \
	| python3 -c 'import json,sys; print(json.load(sys.stdin)["items"][0]["id"])')"
[[ -n "${REQ_ID}" ]] || fail "history is empty"

step "SSE stream reaches request_done"
curl -fsS -N --max-time 20 "${BASE}/api/requests/${REQ_ID}/events" \
	-H "Authorization: Bearer ${API_KEY}" >"${WORKDIR}/sse.txt" || true
grep -q 'event: request_done' "${WORKDIR}/sse.txt" || \
	grep -q 'event: step_finished' "${WORKDIR}/sse.txt" || fail "no SSE events received"

step "playground async search"
UI_ID="$(curl -fsS -X POST "${BASE}/api/search/ui" \
	-H "Authorization: Bearer ${API_KEY}" -H 'Content-Type: application/json' \
	-d '{"query":"async","count":2}' | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
[[ -n "${UI_ID}" ]] || fail "async search did not return an id"

step "logs are persisted and can be cleared"
curl -fsS "${BASE}/api/logs?limit=1" -H "Authorization: Bearer ${API_KEY}" | grep -q '"items"' || fail "logs unavailable"
curl -fsS -X POST "${BASE}/api/maintenance/clear-logs" -H "Authorization: Bearer ${API_KEY}" | grep -q '"deleted"' || fail "clear-logs failed"

step "auth toggle off opens the admin API"
curl -fsS -X PUT "${BASE}/api/settings" -H "Authorization: Bearer ${API_KEY}" \
	-H 'Content-Type: application/json' -d '{"auth_enabled":"false"}' >/dev/null
curl -fsS "${BASE}/api/me" | grep -q '"admin":true' || fail "AUTH_ENABLED=false did not open the API"
curl -fsS -X PUT "${BASE}/api/settings" -H "Authorization: Bearer ${API_KEY}" \
	-H 'Content-Type: application/json' -d '{"auth_enabled":"true"}' >/dev/null

echo
echo "E2E OK"
