#!/usr/bin/env bash
# Development mode: backend on :8080 + Vite dev server on :5173 (hot reload).
#
#   ./scripts/dev-local.sh
#
# Vite proxies /api, /search, /healthz and /version to the backend, so open
# http://localhost:5173 while working on the SPA.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

DATA_DIR="${DATA_DIR:-./data}"
PORT="${PORT:-8080}"

command -v go >/dev/null 2>&1 || { echo "ERROR: go is required" >&2; exit 1; }
command -v npm >/dev/null 2>&1 || { echo "ERROR: npm is required" >&2; exit 1; }

# A build is needed once so go:embed finds web/build; rebuild when stale.
if [[ ! -f web/build/index.html ]] || [[ -n "$(find web/src web/static -newer web/build/index.html -print -quit 2>/dev/null)" ]]; then
	(cd web && npm install && npm run build)
fi

echo "==> backend on :${PORT} (logs below)"
DATA_DIR="${DATA_DIR}" PORT="${PORT}" CGO_ENABLED=0 go run ./cmd/serpentseek &
BACKEND_PID=$!
trap 'kill "${BACKEND_PID}" 2>/dev/null || true' EXIT

sleep 2
echo "==> vite dev on http://localhost:5173"
(cd web && npm run dev)
