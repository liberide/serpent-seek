#!/usr/bin/env bash
# Build (if needed) and run SerpentSeek locally without Docker.
#
#   ./scripts/run-local.sh
#   PORT=18080 DATA_DIR=/tmp/ss ./scripts/run-local.sh
#
# The script fails early with a readable message instead of a bare
# "./serpentseek: No such file or directory".
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

DATA_DIR="${DATA_DIR:-./data}"
PORT="${PORT:-8080}"

need() {
	command -v "$1" >/dev/null 2>&1 || {
		echo "ERROR: '$1' is required but not found in PATH." >&2
		echo "       Install it or add it to PATH, then re-run this script." >&2
		exit 1
	}
}

# The Go binary embeds web/build, so it must exist and be up to date before
# `go build`.
needs_web_build=0
if [[ ! -f web/build/index.html ]]; then
	needs_web_build=1
elif [[ -n "$(find web/src web/static -newer web/build/index.html -print -quit 2>/dev/null)" ]]; then
	needs_web_build=1
fi
if [[ "${needs_web_build}" == "1" ]]; then
	echo "==> building the SPA (web/build is missing or stale)..."
	need npm
	(cd web && npm ci && npm run build)
fi

echo "==> building the Go binary..."
need go
CGO_ENABLED=0 go build -o serpentseek ./cmd/serpentseek

echo "==> starting SerpentSeek on http://localhost:${PORT} (DATA_DIR=${DATA_DIR})"
echo "    Look for 'SETUP_TOKEN:' below on first run."
exec env DATA_DIR="${DATA_DIR}" PORT="${PORT}" ./serpentseek
